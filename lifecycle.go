package qpool

import (
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/theapemachine/datura"
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
		func(prev, current, next *workerStackNode) bool {
			return prev.next.CompareAndSwap(current, next)
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
	if registry == nil {
		return
	}

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

	artifact := datura.Acquire("qpool", datura.Artifact_Type_json)
	artifact.SetRole("op")
	artifact.SetPayload([]byte(fmt.Sprintf("worker started; workers=%d", pool.metrics.workerCount.Load())))
	artifact.SetTimestamp(time.Now().UnixNano())
	artifact.SetScope("debug")
	artifact.Poke("workers", strconv.FormatInt(pool.metrics.workerCount.Load(), 10))
	pool.publishTelemetry(artifact)
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

		artifact := datura.Acquire("qpool", datura.Artifact_Type_json)
		artifact.SetRole("op")
		artifact.SetPayload([]byte("worker deactivated"))
		artifact.SetTimestamp(time.Now().UnixNano())
		artifact.SetScope("debug")
		artifact.Poke("worker", strconv.FormatUint(token.id, 10))
		pool.publishTelemetry(artifact)
	}
}

func (pool *Q[T]) closePool() {
	if pool == nil {
		return
	}

	artifact := datura.Acquire("qpool", datura.Artifact_Type_json)
	artifact.SetRole("op")
	artifact.SetPayload([]byte("closing Q pool"))
	artifact.SetTimestamp(time.Now().UnixNano())
	artifact.SetScope("debug")
	pool.publishTelemetry(artifact)

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

	artifact = datura.Acquire("qpool", datura.Artifact_Type_json)
	artifact.SetRole("op")
	artifact.SetPayload([]byte("Q pool closed"))
	artifact.SetTimestamp(time.Now().UnixNano())
	artifact.SetScope("debug")
	pool.publishTelemetry(artifact)
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
