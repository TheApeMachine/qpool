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

// CircuitBreakerConfig struct
type CircuitBreakerConfig struct {
	MaxFailures  int
	ResetTimeout time.Duration
	HalfOpenMax  int
}
