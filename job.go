package qpool

import "time"

// Job represents work to be done
type Job struct {
	ID                    string
	Fn                    func() (any, error)
	RetryPolicy           *RetryPolicy
	CircuitID             string
	Dependencies          []string
	TTL                   time.Duration
	Attempt               int
	LastError             error
	DependencyRetryPolicy *RetryPolicy
	StartTime             time.Time
}

// JobOption is a function type for configuring jobs
type JobOption func(*Job)
