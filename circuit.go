package qpool

import (
	"log"
	"time"
)

func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if time.Since(cb.lastFailure) > cb.resetTimeout {
			cb.state = CircuitHalfOpen
			cb.halfOpenPass = 0
			log.Printf("Circuit breaker state changed to HALF-OPEN")
			return true
		}
		return false
	case CircuitHalfOpen:
		return cb.halfOpenPass < cb.halfOpenMax
	default:
		return false
	}
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == CircuitHalfOpen {
		cb.halfOpenPass++
		if cb.halfOpenPass >= cb.halfOpenMax {
			cb.state = CircuitClosed
			cb.failures = 0
			log.Printf("Circuit breaker state changed to CLOSED")
		}
	}
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	if cb.failures >= cb.maxFailures && cb.state != CircuitOpen {
		cb.state = CircuitOpen
		log.Printf("Circuit breaker state changed to OPEN")
	}
}
