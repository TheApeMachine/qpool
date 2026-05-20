package qpool

import (
	"container/list"
	"sync"
)

const defaultCircuitBreakerLimit = 1024

type circuitBreakerEntry struct {
	id      string
	breaker *CircuitBreaker
}

type circuitBreakerCache struct {
	mu      sync.Mutex
	entries map[string]*list.Element
	order   *list.List
	limit   int
}

func newCircuitBreakerCache(limit int) *circuitBreakerCache {
	if limit <= 0 {
		limit = defaultCircuitBreakerLimit
	}

	return &circuitBreakerCache{
		entries: make(map[string]*list.Element, limit),
		order:   list.New(),
		limit:   limit,
	}
}

func (cache *circuitBreakerCache) getOrCreate(
	id string,
	config *CircuitBreakerConfig,
) *CircuitBreaker {
	if id == "" || config == nil {
		return nil
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()

	if element, ok := cache.entries[id]; ok {
		cache.order.MoveToFront(element)

		entry := element.Value.(*circuitBreakerEntry)

		return entry.breaker
	}

	breaker := newCircuitBreakerFromConfig(config)
	entry := &circuitBreakerEntry{
		id:      id,
		breaker: breaker,
	}

	cache.entries[id] = cache.order.PushFront(entry)
	cache.evictOverflow()

	return breaker
}

func (cache *circuitBreakerCache) evictOverflow() {
	for len(cache.entries) > cache.limit {
		element := cache.order.Back()

		if element == nil {
			return
		}

		entry := element.Value.(*circuitBreakerEntry)

		delete(cache.entries, entry.id)
		cache.order.Remove(element)
	}
}

func (pool *Q) breakerFor(job *Job) *CircuitBreaker {
	if pool.breakers == nil {
		return nil
	}

	return pool.breakers.getOrCreate(job.CircuitID, job.CircuitConfig)
}

func (pool *Q) breakerForJob(job *Job) *CircuitBreaker {
	if job.circuitBreaker != nil {
		return job.circuitBreaker
	}

	return pool.breakerFor(job)
}
