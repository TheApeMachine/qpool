package qpool

import (
	"context"
	"time"
)

/*
Job represents work to be done
*/
type Job struct {
	ID                    string
	Fn                    func(context.Context) (any, error)
	RetryPolicy           *RetryPolicy
	CircuitID             string
	CircuitConfig         *CircuitBreakerConfig
	Dependencies          []string
	TTL                   time.Duration
	ExecTimeout           time.Duration
	Attempt               int
	LastError             error
	DependencyRetryPolicy *RetryPolicy
	StartTime             time.Time
	circuitBreaker        *CircuitBreaker
}

/*
JobOption is a function type for configuring jobs
*/
type JobOption func(*Job)

/*
CircuitBreakerConfig defines configuration for a circuit breaker
*/
type CircuitBreakerConfig struct {
	MaxFailures  int
	ResetTimeout time.Duration
	HalfOpenMax  int
}

/*
WithDependencyRetry configures retry behavior for dependencies
*/
func WithDependencyRetry(attempts int, strategy RetryStrategy) JobOption {
	return func(j *Job) {
		j.DependencyRetryPolicy = &RetryPolicy{
			MaxAttempts: attempts,
			Strategy:    strategy,
		}
	}
}

/*
WithDependencies configures job dependencies
*/
func WithDependencies(dependencies []string) JobOption {
	return func(j *Job) {
		if len(dependencies) == 0 {
			j.Dependencies = nil

			return
		}

		j.Dependencies = append([]string(nil), dependencies...)
	}
}

/*
DependenciesConfiguredByOption reports whether applying opt to a fresh Job sets non-empty
Dependencies. Callers that own merge or sensor ordering (for example manifold.ScheduleReact) use
this to reject user-supplied WithDependencies values that would corrupt job chains.
*/
func DependenciesConfiguredByOption(opt JobOption) bool {
	if opt == nil {
		return false
	}

	var j Job

	opt(&j)

	return len(j.Dependencies) > 0
}
