package qpool

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/phuslu/log"
)

type workerToken struct {
	id     uint64
	cancel func()
}

type workerStackNode struct {
	token *workerToken
	next  atomic.Pointer[workerStackNode]
}

type workerRegistry struct {
	workers IntrusiveList[workerStackNode]
}

func newWorkerRegistry() *workerRegistry {
	registry := &workerRegistry{}
	registry.workers.bind(
		func(node *workerStackNode) *workerStackNode {
			return node.next.Load()
		},
		func(node, next *workerStackNode) {
			node.next.Store(next)
		},
	)

	return registry
}

func (registry *workerRegistry) push(token *workerToken) {
	if registry == nil || token == nil {
		return
	}

	registry.workers.Prepend(&workerStackNode{token: token})
}

func (registry *workerRegistry) popLast() *workerToken {
	if registry == nil {
		return nil
	}

	node := registry.workers.PopHead()

	if node == nil {
		return nil
	}

	return node.token
}

func (registry *workerRegistry) remove(id uint64) {
	registry.workers.Remove(func(node *workerStackNode) bool {
		return node.token != nil && node.token.id == id
	})
}

func (pool *Q[T]) startWorker() {
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

func (pool *Q[T]) scaleDownWorkers(count int) {
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

func (pool *Q[T]) closePool() {
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

	if pool.jobQueue != nil {
		pool.jobQueue.Close()
	}

	pool.deactivateWorkers()
	pool.deps.Wait()
	pool.scalerWG.Wait()
	pool.space.Close()
	pool.publishTelemetry(Event{
		Component: "qpool",
		Op:        "closed",
		Message:   "Q pool closed",
		Time:      time.Now(),
		Level:     log.DebugLevel,
	})
}

func (pool *Q[T]) deactivateWorkers() {
	for {
		token := pool.registry.popLast()

		if token == nil {
			return
		}

		token.cancel()
		pool.metrics.decWorkerCount()
	}
}
