package qpool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/phuslu/log"
)

type workerToken struct {
	id     uint64
	cancel context.CancelFunc
}

type workerRegistry struct {
	mu        sync.Mutex
	tokens    []*workerToken
	positions map[uint64]int
}

func newWorkerRegistry() *workerRegistry {
	return &workerRegistry{
		positions: make(map[uint64]int),
	}
}

func (registry *workerRegistry) push(token *workerToken) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.positions[token.id] = len(registry.tokens)
	registry.tokens = append(registry.tokens, token)
}

func (registry *workerRegistry) popLast() *workerToken {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	lastIndex := len(registry.tokens) - 1

	if lastIndex < 0 {
		return nil
	}

	token := registry.tokens[lastIndex]
	registry.tokens = registry.tokens[:lastIndex]
	delete(registry.positions, token.id)

	return token
}

func (registry *workerRegistry) remove(id uint64) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	index, ok := registry.positions[id]

	if !ok {
		return
	}

	lastIndex := len(registry.tokens) - 1
	lastToken := registry.tokens[lastIndex]
	registry.tokens[index] = lastToken
	registry.tokens = registry.tokens[:lastIndex]
	delete(registry.positions, id)

	if index == lastIndex {
		return
	}

	registry.positions[lastToken.id] = index
}

func (pool *Q) startWorker() {
	if !pool.metrics.tryIncWorkerIfBelow(pool.maxWorkers) {
		return
	}

	workerCtx, cancel := context.WithCancel(pool.ctx)

	id := pool.nextWorker.Add(1)
	token := &workerToken{id: id, cancel: cancel}

	pool.registry.push(token)
	pool.wg.Add(1)

	go func() {
		defer pool.wg.Done()

		pool.runWorker(workerCtx, token)
	}()

	pool.publishTelemetry(Event{
		Component: "qpool",
		Op:        "worker-start",
		Message:   fmt.Sprintf("worker started; workers=%d", pool.metrics.workerCount.Load()),
		Time:      time.Now(),
		Level:     log.DebugLevel,
		Fields: []Field{
			{Key: "workers", Value: pool.metrics.workerCount.Load()},
		},
	})
}

func (pool *Q) runWorker(workerCtx context.Context, token *workerToken) {
	defer pool.registry.remove(token.id)
	defer pool.metrics.decWorkerCount()

	for {
		select {
		case <-workerCtx.Done():
			pool.publishTelemetry(Event{
				Component: "qpool",
				Op:        "worker-exit",
				Message:   "worker exiting due to cancellation",
				Time:      time.Now(),
				Level:     log.DebugLevel,
				Fields: []Field{
					{Key: "worker", Value: token.id},
				},
			})

			return

		case job, ok := <-pool.jobCh:
			if !ok {
				return
			}

			pool.metrics.decJobQueued()
			pool.metrics.incBusyWorker()

			func() {
				defer pool.metrics.decBusyWorker()

				processJob(pool, workerCtx, job)
			}()
		}
	}
}

func (pool *Q) scaleDownWorkers(count int) {
	for range count {
		token := pool.registry.popLast()

		if token == nil {
			return
		}

		token.cancel()
	}
}

/*
Close cancels workers and drains queued jobs with shutdown errors.
*/
func (pool *Q) Close() {
	if pool == nil {
		return
	}

	pool.publishTelemetry(Event{
		Component: "qpool",
		Op:        "close",
		Message:   "closing Q pool",
		Time:      time.Now(),
		Level:     log.DebugLevel,
	})

	pool.stopping.Store(true)

	if pool.cancel != nil {
		pool.cancel()
	}

	// Wait until schedules already inside the enqueue select release RLock.
	pool.shutdownMu.Lock()
	pool.shutdownMu.Unlock()
	pool.wg.Wait()
	pool.shutdownMu.Lock()

	pool.closeJobOnce.Do(func() {
		close(pool.jobCh)
	})

	for job := range pool.jobCh {
		pool.space.StoreError(job.ID, fmt.Errorf("qpool: pool shut down"), job.TTL)
	}

	pool.shutdownMu.Unlock()
	pool.space.Close()
	pool.publishTelemetry(Event{
		Component: "qpool",
		Op:        "closed",
		Message:   "Q pool closed",
		Time:      time.Now(),
		Level:     log.DebugLevel,
	})
}
