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

func (cache *circuitBreakerCache) find(id string) *CircuitBreaker {
	for node := cache.head.Load(); node != nil; node = node.next.Load() {
		if node.entry != nil && node.entry.id == id {
			return node.entry.breaker
		}
	}

	return nil
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
		head := cache.head.Load()
		if head == nil {
			return
		}

		next := head.next.Load()

		if cache.head.CompareAndSwap(head, next) {
			cache.count.Add(-1)
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
