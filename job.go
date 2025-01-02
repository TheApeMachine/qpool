package qpool

import "time"

// Job represents work to be done
type Job struct {
	ID                    string
	Fn                    func() (any, error)
	RetryPolicy           *RetryPolicy
	CircuitID             string
	CircuitConfig         *CircuitBreakerConfig
	Dependencies          []string
	TTL                   time.Duration
	Attempt               int
	LastError             error
	DependencyRetryPolicy *RetryPolicy
	StartTime             time.Time
}

// JobOption is a function type for configuring jobs
type JobOption func(*Job)

// CircuitBreakerConfig defines configuration for a circuit breaker
type CircuitBreakerConfig struct {
	MaxFailures  int
	ResetTimeout time.Duration
	HalfOpenMax  int
}

// WithDependencyRetry configures retry behavior for dependencies
func WithDependencyRetry(attempts int, strategy RetryStrategy) JobOption {
	return func(j *Job) {
		j.DependencyRetryPolicy = &RetryPolicy{
			MaxAttempts: attempts,
			Strategy:    strategy,
		}
	}
}

// WithDependencies configures job dependencies
func WithDependencies(dependencies []string) JobOption {
	return func(j *Job) {
		j.Dependencies = dependencies
	}
}
