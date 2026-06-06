package qpool

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	disruptor "github.com/smarty/go-disruptor"
)

const unassignedDisruptorWorker = int64(-1)

type disruptorWorkKind uint8

const (
	disruptorWorkNone disruptorWorkKind = iota
	disruptorWorkJob
	disruptorWorkFast
)

type jobDisruptorQueue struct {
	disruptor     disruptor.Disruptor
	ring          []jobDisruptorSlot
	mask          int64
	pool          *Q[any]
	listenWG      atomicWaitGroup
	closed        atomic.Bool
	activeWorkers atomic.Int64
}

type jobDisruptorSlot struct {
	worker atomic.Int64
	kind   disruptorWorkKind
	job    Job
	fast   fastDisruptorWork
}

type fastDisruptorWork struct {
	ctx    context.Context
	fn     func(context.Context) (interface{}, error)
	result *resultSlot
}

type jobDisruptorHandler struct {
	queue       *jobDisruptorQueue
	workerIndex int64
}

func newJobDisruptorQueue(
	pool *Q[any],
	queueCapacity int,
	maxWorkers int,
) (*jobDisruptorQueue, error) {
	if maxWorkers < 1 {
		maxWorkers = 1
	}

	capacity := nextPowerOfTwo(max(1, queueCapacity+maxWorkers))
	queue := &jobDisruptorQueue{
		ring: make([]jobDisruptorSlot, capacity),
		mask: int64(capacity - 1),
		pool: pool,
	}

	for i := range queue.ring {
		queue.ring[i].worker.Store(unassignedDisruptorWorker)
	}

	handlers := make([]disruptor.Handler, maxWorkers)
	for i := range handlers {
		handlers[i] = &jobDisruptorHandler{
			queue:       queue,
			workerIndex: int64(i),
		}
	}

	instance, err := disruptor.New(
		disruptor.Options.BufferCapacity(uint32(capacity)),
		disruptor.Options.WriterCount(2),
		disruptor.Options.NewHandlerGroup(handlers...),
	)
	if err != nil {
		return nil, fmt.Errorf("qpool: initialize disruptor job queue: %w", err)
	}

	queue.disruptor = instance
	queue.listenWG.Add(1)

	go func() {
		defer queue.listenWG.Done()
		instance.Listen()
	}()

	return queue, nil
}

func (queue *jobDisruptorQueue) setActiveWorkers(workers int64) {
	if workers < 1 {
		workers = 1
	}

	queue.activeWorkers.Store(workers)
}

func (queue *jobDisruptorQueue) publishJob(ctx context.Context, job Job) error {
	return queue.publish(ctx, disruptorWorkJob, job, fastDisruptorWork{})
}

func (queue *jobDisruptorQueue) publishFast(
	ctx context.Context,
	work fastDisruptorWork,
) error {
	return queue.publish(ctx, disruptorWorkFast, Job{}, work)
}

func (queue *jobDisruptorQueue) publish(
	ctx context.Context,
	kind disruptorWorkKind,
	job Job,
	fast fastDisruptorWork,
) error {
	if queue == nil || queue.disruptor == nil {
		return fmt.Errorf("qpool: job queue unavailable")
	}

	for spin := 0; ; spin++ {
		if queue.closed.Load() {
			return fmt.Errorf("qpool: pool closed")
		}

		if err := queue.pool.ctx.Err(); err != nil {
			return err
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		upper := queue.disruptor.TryReserve(1)
		switch upper {
		case disruptor.ErrCapacityUnavailable:
			backoffDisruptorReservation(spin)
			continue
		case disruptor.ErrReservationSize:
			return fmt.Errorf("qpool: invalid disruptor reservation")
		default:
			slot := &queue.ring[upper&queue.mask]
			slot.worker.Store(unassignedDisruptorWorker)
			slot.kind = kind
			slot.job = job
			slot.fast = fast

			if kind == disruptorWorkJob {
				queue.pool.metrics.incJobQueued()
			}

			queue.disruptor.Commit(upper, upper)

			return nil
		}
	}
}

func (queue *jobDisruptorQueue) Close() {
	if queue == nil || queue.disruptor == nil {
		return
	}

	if queue.closed.Swap(true) {
		return
	}

	_ = queue.disruptor.Close()
	queue.listenWG.Wait()
}

func (handler *jobDisruptorHandler) Handle(lowerSequence, upperSequence int64) {
	for sequence := lowerSequence; sequence <= upperSequence; sequence++ {
		slot := &handler.queue.ring[sequence&handler.queue.mask]
		assignedWorker := handler.assignedWorker(slot, sequence)

		if assignedWorker != handler.workerIndex {
			continue
		}

		switch slot.kind {
		case disruptorWorkJob:
			handler.handleJob(slot)

		case disruptorWorkFast:
			handler.handleFast(slot)
		}

		slot.kind = disruptorWorkNone
		slot.job = Job{}
		slot.fast = fastDisruptorWork{}
	}
}

func (handler *jobDisruptorHandler) handleJob(slot *jobDisruptorSlot) {
	handler.queue.pool.metrics.decJobQueued()
	handler.queue.pool.metrics.incBusyWorker()

	func() {
		defer handler.queue.pool.metrics.decBusyWorker()

		processJob(handler.queue.pool, handler.queue.pool.ctx, slot.job)
	}()
}

func (handler *jobDisruptorHandler) handleFast(slot *jobDisruptorSlot) {
	work := slot.fast

	if work.result == nil {
		return
	}

	result := work.result

	if err := work.ctx.Err(); err != nil {
		finishFast(result, nil, err)

		return
	}

	if err := handler.queue.pool.ctx.Err(); err != nil {
		finishFast(result, nil, fmt.Errorf("qpool: pool closed: %w", err))

		return
	}

	value, err := invokeFastFnOnce(work.ctx, work.fn)
	finishFast(result, value, err)
}

func (handler *jobDisruptorHandler) assignedWorker(
	slot *jobDisruptorSlot,
	sequence int64,
) int64 {
	for {
		assigned := slot.worker.Load()
		if assigned != unassignedDisruptorWorker {
			return assigned
		}

		activeWorkers := max(handler.queue.activeWorkers.Load(), 1)

		worker := sequence % activeWorkers
		if slot.worker.CompareAndSwap(unassignedDisruptorWorker, worker) {
			return worker
		}
	}
}

func backoffDisruptorReservation(spin int) {
	if spin < 64 {
		runtime.Gosched()

		return
	}

	time.Sleep(100 * time.Microsecond)
}

func nextPowerOfTwo(value int) int {
	if value <= 1 {
		return 1
	}

	power := 1
	for power < value {
		power <<= 1
	}

	return power
}
