package qpool

import (
	"log"
	"sync"
	"time"
)

// CircuitState represents the state of the circuit breaker
type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	mu               sync.RWMutex
	maxFailures      int
	resetTimeout     time.Duration
	halfOpenMax      int
	failureCount     int
	state            CircuitState
	openTime         time.Time
	halfOpenAttempts int
}

// RecordFailure records a failure and updates the circuit state
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	if cb.failureCount >= cb.maxFailures {
		if cb.state == CircuitHalfOpen {
			// If we fail in half-open state, go back to open
			cb.state = CircuitOpen
			cb.openTime = time.Now()
			log.Printf("Circuit breaker reopened from half-open state")
		} else if cb.state == CircuitClosed {
			// Only open the circuit if we were closed
			cb.state = CircuitOpen
			cb.openTime = time.Now()
			log.Printf("Circuit breaker opened")
		}
	}
}

// RecordSuccess records a successful attempt and updates the circuit state
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == CircuitHalfOpen {
		cb.halfOpenAttempts++
		if cb.halfOpenAttempts >= cb.halfOpenMax {
			cb.state = CircuitClosed
			cb.failureCount = 0
			cb.halfOpenAttempts = 0
			log.Printf("Circuit breaker closed from half-open")
		}
	}
}

// Allow determines if a request is allowed based on the circuit state
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if time.Since(cb.openTime) > cb.resetTimeout {
			cb.mu.RUnlock()
			cb.mu.Lock()
			cb.state = CircuitHalfOpen
			cb.halfOpenAttempts = 0
			cb.mu.Unlock()
			cb.mu.RLock()
			return true
		}
		return false
	case CircuitHalfOpen:
		return cb.halfOpenAttempts < cb.halfOpenMax
	default:
		return false
	}
}
