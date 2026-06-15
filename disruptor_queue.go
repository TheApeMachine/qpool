package qpool

import (
	"context"
	"fmt"
	"math/rand"
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
)

type jobDisruptorQueue struct {
	disruptor     disruptor.Disruptor
	ring          RingBuffer[jobDisruptorSlot]
	pool          *Q[any]
	wg            *WaitGroup
	closed        atomic.Bool
	activeWorkers atomic.Int64
}

type jobDisruptorSlot struct {
	worker atomic.Int64
	kind   disruptorWorkKind
	job    Job
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

	capacity := max(1, queueCapacity+maxWorkers)
	queue := &jobDisruptorQueue{
		ring: NewRingBuffer[jobDisruptorSlot](capacity),
		pool: pool,
		wg:   &WaitGroup{},
	}

	for sequence := int64(0); sequence < int64(queue.ring.Capacity()); sequence++ {
		queue.ring.Slot(sequence).worker.Store(unassignedDisruptorWorker)
	}

	handlers := make([]disruptor.Handler, maxWorkers)

	for index := range handlers {
		handlers[index] = &jobDisruptorHandler{
			queue:       queue,
			workerIndex: int64(index),
		}
	}

	instance, err := disruptor.New(
		disruptor.Options.BufferCapacity(uint32(queue.ring.Capacity())),
		disruptor.Options.WriterCount(2),
		disruptor.Options.WaitStrategy(&DisruptorIdleWaitStrategy{}),
		disruptor.Options.NewHandlerGroup(handlers...),
	)

	if err != nil {
		return nil, fmt.Errorf("qpool: initialize disruptor job queue: %w", err)
	}

	queue.disruptor = instance
	queue.wg.Add(1)

	go func() {
		defer queue.wg.Done()
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
	return queue.publish(ctx, disruptorWorkJob, job)
}

func (queue *jobDisruptorQueue) publish(
	ctx context.Context,
	kind disruptorWorkKind,
	job Job,
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
			queue.backoffReservation(spin)
			continue
		case disruptor.ErrReservationSize:
			return fmt.Errorf("qpool: invalid disruptor reservation")
		default:
			slot := queue.ring.Slot(upper)
			slot.worker.Store(unassignedDisruptorWorker)
			slot.kind = kind
			slot.job = job

			if kind == disruptorWorkJob {
				queue.pool.metrics.incJobQueued()
			}

			queue.disruptor.Commit(upper, upper)

			return nil
		}
	}
}

func (queue *jobDisruptorQueue) backoffReservation(spin int) {
	if spin < 64 {
		runtime.Gosched()

		return
	}

	time.Sleep(time.Duration(50+rand.Intn(101)) * time.Microsecond)
}

func (queue *jobDisruptorQueue) Close() {
	if queue == nil || queue.disruptor == nil {
		return
	}

	if queue.closed.Swap(true) {
		return
	}

	_ = queue.disruptor.Close()
	queue.wg.Wait()
}

func (handler *jobDisruptorHandler) Handle(lowerSequence, upperSequence int64) {
	for sequence := lowerSequence; sequence <= upperSequence; sequence++ {
		slot := handler.queue.ring.Slot(sequence)
		assignedWorker := handler.assignedWorker(slot, sequence)

		if assignedWorker != handler.workerIndex {
			continue
		}

		if slot.kind == disruptorWorkJob {
			handler.handleJob(slot)
		}

		slot.kind = disruptorWorkNone
		slot.job = Job{}
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
