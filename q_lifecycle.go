package qpool

import (
	"fmt"
	"sync"
	"time"

	"github.com/phuslu/log"
)

type workerToken struct {
	id     uint64
	cancel func()
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

func (pool *Q[any]) startWorker() {
	if !pool.metrics.tryIncWorkerIfBelow(pool.maxWorkers) {
		return
	}

	id := pool.nextWorker.Add(1)
	token := &workerToken{id: id, cancel: func() {}}

	pool.registry.push(token)
	pool.jobQueue.setActiveWorkers(pool.metrics.workerCount.Load())

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

func (pool *Q[any]) scaleDownWorkers(count int) {
	for range count {
		token := pool.registry.popLast()

		if token == nil {
			return
		}

		token.cancel()
		pool.metrics.decWorkerCount()
		pool.jobQueue.setActiveWorkers(pool.metrics.workerCount.Load())

		pool.publishTelemetry(Event{
			Component: "qpool",
			Op:        "worker-exit",
			Message:   "worker deactivated",
			Time:      time.Now(),
			Level:     log.DebugLevel,
			Fields: []Field{
				{Key: "worker", Value: token.id},
			},
		})
	}
}

/*
Close cancels the pool and drains committed disruptor jobs with shutdown errors.
*/
func (pool *Q[any]) Close() {
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

	pool.waitForScheduleLockDrain()

	if pool.jobQueue != nil {
		pool.jobQueue.Close()
	}

	if pool.fastQueue != nil {
		pool.fastQueue.Close()
	}

	pool.deactivateWorkers()
	pool.wg.Wait()
	pool.space.Close()
	pool.publishTelemetry(Event{
		Component: "qpool",
		Op:        "closed",
		Message:   "Q pool closed",
		Time:      time.Now(),
		Level:     log.DebugLevel,
	})
}

func (pool *Q[any]) waitForScheduleLockDrain() {
	// Taking the write lock blocks until schedules already inside RLock exit.
	pool.shutdownMu.Lock()
	defer pool.shutdownMu.Unlock()
}

func (pool *Q[any]) deactivateWorkers() {
	for {
		token := pool.registry.popLast()
		if token == nil {
			return
		}

		token.cancel()
		pool.metrics.decWorkerCount()
	}
}
