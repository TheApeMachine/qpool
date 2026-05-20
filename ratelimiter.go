package qpool

import (
	"sync/atomic"
	"time"
)

/*
RateLimiter implements Regulator using a token bucket with atomic accounting only.
*/
type RateLimiter struct {
	tokens     atomic.Int64
	maxTokens  int64
	refillRate time.Duration
	lastRefill atomic.Int64
}

/*
NewRateLimiter creates a rate limit regulator.
*/
func NewRateLimiter(maxTokens int, refillRate time.Duration) *RateLimiter {
	capacity := maxTokens
	if capacity < 0 {
		capacity = 0
	}

	rl := &RateLimiter{
		maxTokens:  int64(capacity),
		refillRate: refillRate,
	}

	if rl.refillRate <= 0 {
		rl.refillRate = time.Second
	}

	rl.tokens.Store(int64(capacity))
	rl.lastRefill.Store(time.Now().UnixNano())

	return rl
}

/*
Observe implements Regulator (reserved for adaptive extensions).
*/
func (rl *RateLimiter) Observe(reading *MetricReading) {
	_ = reading
}

/*
Limit implements Regulator: true when this schedule should be rejected (no token).
*/
func (rl *RateLimiter) Limit() bool {
	rl.refillTokens(time.Now().UnixNano())

	for {
		cur := rl.tokens.Load()
		if cur <= 0 {
			return true
		}

		if rl.tokens.CompareAndSwap(cur, cur-1) {
			return false
		}
	}
}

/*
Renormalize triggers refill without consuming a token.
*/
func (rl *RateLimiter) Renormalize() {
	rl.refillTokens(time.Now().UnixNano())
}

func (rl *RateLimiter) refillTokens(nowUnixNano int64) {
	refillNs := rl.refillRate.Nanoseconds()
	if refillNs <= 0 {
		refillNs = int64(time.Second)
	}

	for {
		last := rl.lastRefill.Load()
		elapsed := nowUnixNano - last
		if elapsed < refillNs {
			return
		}

		periods := elapsed / refillNs

		nextLast := last + periods*refillNs
		if !rl.lastRefill.CompareAndSwap(last, nextLast) {
			continue
		}

		for {
			cur := rl.tokens.Load()
			cappedTokens := min(cur+periods, rl.maxTokens)
			if rl.tokens.CompareAndSwap(cur, cappedTokens) {
				break
			}
		}

		return
	}
}
