package qpool

import (
	"sync/atomic"
)

const defaultCircuitBreakerLimit = 1024

type circuitBreakerEntry struct {
	id      string
	breaker *CircuitBreaker
}

type breakerCacheNode struct {
	entry *circuitBreakerEntry
	next  atomic.Pointer[breakerCacheNode]
}

type circuitBreakerCache struct {
	head  atomic.Pointer[breakerCacheNode]
	count atomic.Int64
	limit int
}

func newCircuitBreakerCache(limit int) *circuitBreakerCache {
	if limit <= 0 {
		limit = defaultCircuitBreakerLimit
	}

	return &circuitBreakerCache{limit: limit}
}

func (cache *circuitBreakerCache) findNode(id string) *breakerCacheNode {
	for node := cache.head.Load(); node != nil; node = node.next.Load() {
		if node.entry != nil && node.entry.id == id {
			return node
		}
	}

	return nil
}

func (cache *circuitBreakerCache) find(id string) *CircuitBreaker {
	node := cache.findNode(id)

	if node == nil {
		return nil
	}

	cache.promote(node)

	return node.entry.breaker
}

func (cache *circuitBreakerCache) promote(node *breakerCacheNode) {
	if node == nil {
		return
	}

	for {
		if cache.head.Load() == node {
			return
		}

		if !cache.detach(node) {
			continue
		}

		for {
			head := cache.head.Load()
			node.next.Store(head)

			if cache.head.CompareAndSwap(head, node) {
				return
			}
		}
	}
}

func (cache *circuitBreakerCache) detach(node *breakerCacheNode) bool {
	for {
		head := cache.head.Load()

		if head == node {
			next := node.next.Load()

			if cache.head.CompareAndSwap(head, next) {
				node.next.Store(nil)

				return true
			}

			continue
		}

		for prev := head; prev != nil; {
			next := prev.next.Load()

			if next == node {
				rest := node.next.Load()

				if prev.next.CompareAndSwap(node, rest) {
					node.next.Store(nil)

					return true
				}

				return false
			}

			prev = next
		}

		return false
	}
}

func (cache *circuitBreakerCache) getOrCreate(
	id string,
	config *CircuitBreakerConfig,
) *CircuitBreaker {
	if id == "" || config == nil {
		return nil
	}

	if breaker := cache.find(id); breaker != nil {
		return breaker
	}

	breaker := newCircuitBreakerFromConfig(config)
	entry := &circuitBreakerEntry{
		id:      id,
		breaker: breaker,
	}

	node := &breakerCacheNode{entry: entry}

	for {
		head := cache.head.Load()
		node.next.Store(head)

		if cache.head.CompareAndSwap(head, node) {
			if existing := cache.find(id); existing != nil && existing != breaker {
				return existing
			}

			cache.count.Add(1)
			cache.evictOverflow()

			return breaker
		}

		if existing := cache.find(id); existing != nil {
			return existing
		}
	}
}

func (cache *circuitBreakerCache) evictOverflow() {
	for cache.count.Load() > int64(cache.limit) {
		if !cache.evictTail() {
			return
		}
	}
}

func (cache *circuitBreakerCache) evictTail() bool {
	for {
		head := cache.head.Load()

		if head == nil {
			return false
		}

		if head.next.Load() == nil {
			if cache.head.CompareAndSwap(head, nil) {
				cache.count.Add(-1)

				return true
			}

			continue
		}

		prev := head

		for {
			next := prev.next.Load()

			if next == nil {
				return false
			}

			if next.next.Load() == nil {
				if prev.next.CompareAndSwap(next, nil) {
					cache.count.Add(-1)

					return true
				}

				return false
			}

			prev = next
		}
	}
}

func (pool *Q[T]) breakerFor(job *Job) *CircuitBreaker {
	if pool.breakers == nil {
		return nil
	}

	return pool.breakers.getOrCreate(job.CircuitID, job.CircuitConfig)
}

func (pool *Q[T]) breakerForJob(job *Job) *CircuitBreaker {
	if job.circuitBreaker != nil {
		return job.circuitBreaker
	}

	return pool.breakerFor(job)
}
