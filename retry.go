package qpool

import (
	"math"
	"time"
)

// RetryPolicy defines retry behavior
type RetryPolicy struct {
	MaxAttempts int
	Strategy    RetryStrategy
	BackoffFunc func(attempt int) time.Duration
	Filter      func(error) bool
}

// RetryStrategy defines the interface for retry behavior
type RetryStrategy interface {
	NextDelay(attempt int) time.Duration
}

// ExponentialBackoff implements RetryStrategy
type ExponentialBackoff struct {
	Initial time.Duration
}

func (eb *ExponentialBackoff) NextDelay(attempt int) time.Duration {
	return eb.Initial * time.Duration(math.Pow(2, float64(attempt-1)))
}

// WithCircuitBreaker configures circuit breaker for a job
func WithCircuitBreaker(id string, maxFailures int, resetTimeout time.Duration) JobOption {
	return func(j *Job) {
		j.CircuitID = id
	}
}

// WithRetry configures retry behavior for a job
func WithRetry(attempts int, strategy RetryStrategy) JobOption {
	return func(j *Job) {
		j.RetryPolicy = &RetryPolicy{
			MaxAttempts: attempts,
			Strategy:    strategy,
		}
	}
}
