package qpool

import (
	"sync/atomic"
	"time"
)

/*
CircuitState represents the state of the circuit breaker.
*/
type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

const (
	cbClosed uint32 = iota
	cbOpen
	cbHalfOpen
)

/*
CircuitBreaker implements the circuit breaker pattern and Regulator interface using
atomic operations only (no mutex in this package).
*/
type CircuitBreaker struct {
	maxFailures      int
	resetTimeout     time.Duration
	halfOpenMax      int
	state            atomic.Uint32
	failureCount     atomic.Uint32
	openSinceNs      atomic.Int64
	halfOpenSuccess  atomic.Uint32
	halfOpenInflight atomic.Int32
}

/*
NewCircuitBreaker creates a new circuit breaker instance with specified parameters.
*/
func NewCircuitBreaker(
	maxFailures int, resetTimeout time.Duration, halfOpenMax int,
) *CircuitBreaker {
	if halfOpenMax <= 0 {
		halfOpenMax = 1
	}

	cb := &CircuitBreaker{
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		halfOpenMax:  halfOpenMax,
	}
	cb.state.Store(cbClosed)
	return cb
}

/*
newCircuitBreakerFromConfig builds a breaker from job configuration fields.
*/
func newCircuitBreakerFromConfig(cfg *CircuitBreakerConfig) *CircuitBreaker {
	return NewCircuitBreaker(cfg.MaxFailures, cfg.ResetTimeout, cfg.HalfOpenMax)
}

/*
Observe implements Regulator (circuit breaker does not currently consume readings).
*/
func (cb *CircuitBreaker) Observe(reading *MetricReading) {
	_ = reading
}

/*
Limit implements Regulator: true when the breaker disallows traffic.
*/
func (cb *CircuitBreaker) Limit() bool {
	return !cb.Allow()
}

/*
Renormalize attempts to move from open toward half-open after the reset timeout.
*/
func (cb *CircuitBreaker) Renormalize() {
	cb.tryOpenToHalfOpen(time.Now())
}

/*
RecordFailure records a failure and updates circuit state.
*/
func (cb *CircuitBreaker) RecordFailure() {
	cb.failureCount.Add(1)

	switch cb.state.Load() {
	case cbHalfOpen:
		cb.safeDecHalfOpenInflight()
		cb.transitionToOpen()
	case cbClosed:
		if int(cb.failureCount.Load()) >= cb.maxFailures {
			cb.transitionToOpen()
		}
	}
}

/*
RecordSuccess records a successful completion.
*/
func (cb *CircuitBreaker) RecordSuccess() {
	cb.failureCount.Store(0)

	switch cb.state.Load() {
	case cbHalfOpen:
		cb.safeDecHalfOpenInflight()
		n := cb.halfOpenSuccess.Add(1)
		if int(n) >= cb.halfOpenMax {
			cb.halfOpenSuccess.Store(0)
			cb.state.Store(cbClosed)
		}
	case cbClosed:
	}
}

/*
Allow reports whether a new attempt may proceed.
*/
func (cb *CircuitBreaker) Allow() bool {
	now := time.Now()

	for {
		switch cb.state.Load() {
		case cbClosed:
			return true

		case cbOpen:
			if cb.tryOpenToHalfOpen(now) {
				continue
			}
			return false

		case cbHalfOpen:
			return cb.acquireHalfOpenSlot()

		default:
			return false
		}
	}
}

func (cb *CircuitBreaker) tryOpenToHalfOpen(now time.Time) bool {
	if cb.state.Load() != cbOpen {
		return false
	}

	resetNs := cb.resetTimeout.Nanoseconds()
	if resetNs <= 0 {
		resetNs = int64(time.Minute)
	}

	opened := cb.openSinceNs.Load()
	if opened == 0 || now.UnixNano()-opened <= resetNs {
		return false
	}

	if cb.state.CompareAndSwap(cbOpen, cbHalfOpen) {
		cb.halfOpenSuccess.Store(0)
		cb.halfOpenInflight.Store(0)
		Publish(NewWarningEvent(
			"qpool",
			"circuit-half-open",
			"circuit breaker half-open after reset timeout",
			nil,
		))

		return true
	}

	return cb.state.Load() != cbOpen
}

func (cb *CircuitBreaker) acquireHalfOpenSlot() bool {
	for {
		if cb.state.Load() != cbHalfOpen {
			return false
		}

		live := cb.halfOpenInflight.Load()
		if cb.halfOpenMax >= 0 && int(live) >= cb.halfOpenMax {
			return false
		}

		if cb.halfOpenInflight.CompareAndSwap(live, live+1) {
			return true
		}
	}
}

func (cb *CircuitBreaker) transitionToOpen() {
	cb.state.Store(cbOpen)
	now := time.Now().UnixNano()
	for {
		cur := cb.openSinceNs.Load()
		if now <= cur {
			break
		}
		if cb.openSinceNs.CompareAndSwap(cur, now) {
			break
		}
	}
	cb.halfOpenSuccess.Store(0)
	cb.halfOpenInflight.Store(0)
	Publish(NewWarningEvent(
		"qpool",
		"circuit-open",
		"circuit breaker open",
		nil,
	))
}

func (cb *CircuitBreaker) safeDecHalfOpenInflight() {
	for {
		cur := cb.halfOpenInflight.Load()
		if cur <= 0 {
			return
		}
		if cb.halfOpenInflight.CompareAndSwap(cur, cur-1) {
			return
		}
	}
}
