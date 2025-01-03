package qpool

import (
	"sync"
	"time"
)

/*
RateLimiter implements the Regulator interface using a token bucket algorithm.
It controls the rate of operations by maintaining a bucket of tokens that are consumed
by each operation and replenished at a fixed rate.

Like a water tank with a steady inflow and controlled outflow, this regulator ensures
that operations occur at a sustainable rate, preventing system overload while allowing
for brief bursts of activity when the token bucket is full.

Key features:
  - Smooth rate limiting with burst capacity
  - Configurable token replenishment rate
  - Thread-safe operation
  - Metric-aware for adaptive rate limiting
*/
type RateLimiter struct {
	tokens     int           // Current number of available tokens
	maxTokens  int           // Maximum token capacity
	refillRate time.Duration // Time between token replenishments
	lastRefill time.Time     // Last time tokens were added
	mu         sync.Mutex    // Ensures thread-safe access to tokens
	metrics    *Metrics      // System metrics for adaptive behavior
}

/*
NewRateLimiter creates a new rate limit regulator with specified parameters.

Parameters:
  - maxTokens: Maximum number of tokens (burst capacity)
  - refillRate: Duration between token replenishments

Returns:
  - *RateLimiter: A new rate limit regulator instance

Example:
    limiter := NewRateLimiter(100, time.Second) // 100 ops/second with burst capacity
*/
func NewRateLimiter(maxTokens int, refillRate time.Duration) *RateLimiter {
	now := time.Now()
	return &RateLimiter{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: now.Add(-refillRate), // Start with a full refill period elapsed
	}
}

/*
Observe implements the Regulator interface by monitoring system metrics.
The rate limiter can use these metrics to dynamically adjust its rate limits
based on system conditions.

For example, it might:
  - Reduce rates during high system load
  - Increase limits when resources are abundant
  - Adjust burst capacity based on queue length

Parameters:
  - metrics: Current system metrics including performance and health indicators
*/
func (rl *RateLimiter) Observe(metrics *Metrics) {
	rl.metrics = metrics
}

/*
Limit implements the Regulator interface by determining if an operation should be limited.
It consumes a token if available, allowing the operation to proceed. If no tokens
are available, the operation is limited.

Returns:
  - bool: true if the operation should be limited, false if it can proceed

Thread-safety: This method is thread-safe through mutex protection.
*/
func (rl *RateLimiter) Limit() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.refill()
	if rl.tokens > 0 {
		rl.tokens--
		return false // Don't limit
	}
	return true // Limit
}

/*
Renormalize implements the Regulator interface by attempting to restore normal operation.
This method triggers a token refill, potentially allowing more operations to proceed
if enough time has passed since the last refill.

The rate limiter uses this method to maintain a steady flow of operations while
adhering to the configured rate limits.
*/
func (rl *RateLimiter) Renormalize() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.refill()
}

/*
refill adds tokens to the bucket based on elapsed time.
This is an internal method that implements the token bucket algorithm's
replenishment logic.

The number of tokens added is proportional to the time elapsed since the last
refill, up to the maximum capacity of the bucket.

Thread-safety: This method assumes the caller holds the mutex lock.
*/
func (rl *RateLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill)
	
	// Convert to nanoseconds for integer division
	elapsedNs := elapsed.Nanoseconds()
	refillRateNs := rl.refillRate.Nanoseconds()

	// Calculate tokens to add - only round up if we're at least halfway through a period
	tokensToAdd := (elapsedNs + (refillRateNs / 2)) / refillRateNs
	
	if tokensToAdd > 0 {
		rl.tokens = Min(rl.maxTokens, rl.tokens+int(tokensToAdd))
		// Only move lastRefill forward by the number of complete periods
		rl.lastRefill = rl.lastRefill.Add(time.Duration(tokensToAdd) * rl.refillRate)
	}
} 