package qpool

import (
	"log"
	"sync"
	"time"
)

/*
CircuitState represents the state of the circuit breaker.
This is used to track the current operational mode of the circuit breaker
as it transitions between different states based on system health.
*/
type CircuitState int

const (
	CircuitClosed CircuitState = iota // Normal operation state
	CircuitOpen                       // Failure state, rejecting requests
	CircuitHalfOpen                   // Probationary state, allowing limited requests
)

/*
CircuitBreaker implements both the circuit breaker pattern and Regulator interface.
It provides a way to automatically degrade service when the system is under stress
by temporarily stopping operations when a failure threshold is reached.

The circuit breaker operates in three states:
  - Closed: Normal operation, all requests are allowed
  - Open: Failure threshold exceeded, all requests are rejected
  - Half-Open: Probationary state allowing limited requests to test system health

This pattern helps prevent cascading failures and allows the system to recover
from failure states without overwhelming potentially unstable dependencies.
*/
type CircuitBreaker struct {
	mu               sync.RWMutex
	maxFailures      int           // Maximum failures before opening circuit
	resetTimeout     time.Duration // Time to wait before attempting recovery
	halfOpenMax      int          // Maximum requests allowed in half-open state
	failureCount     int          // Current count of consecutive failures
	state            CircuitState // Current state of the circuit breaker
	openTime         time.Time    // Time when circuit was opened
	halfOpenAttempts int          // Number of attempts made in half-open state
	metrics          *Metrics     // Current system metrics
}

/*
NewCircuitBreaker creates a new circuit breaker instance with specified parameters.

Parameters:
  - maxFailures: Number of failures allowed before opening the circuit
  - resetTimeout: Duration to wait before attempting to close an open circuit
  - halfOpenMax: Maximum number of requests allowed in half-open state

Returns:
  - *CircuitBreaker: A new circuit breaker instance initialized in closed state
*/
func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration, halfOpenMax int) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		halfOpenMax:  halfOpenMax,
		state:        CircuitClosed,
	}
}

/*
Observe implements the Regulator interface by accepting system metrics.
This method allows the circuit breaker to monitor system health and adjust
its behavior based on current conditions.

Parameters:
  - metrics: Current system metrics including performance and health indicators
*/
func (cb *CircuitBreaker) Observe(metrics *Metrics) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.metrics = metrics
}

/*
Limit implements the Regulator interface by determining if requests should be limited.
This method provides a standardized way to check if the circuit breaker is
currently preventing operations.

Returns:
  - bool: true if requests should be limited, false if they should proceed
*/
func (cb *CircuitBreaker) Limit() bool {
	return !cb.Allow()
}

/*
Renormalize implements the Regulator interface by attempting to restore normal operation.
This method checks if enough time has passed since the circuit was opened and
transitions to half-open state if appropriate, allowing for system recovery.
*/
func (cb *CircuitBreaker) Renormalize() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	if cb.state == CircuitOpen && time.Since(cb.openTime) > cb.resetTimeout {
		cb.state = CircuitHalfOpen
		cb.halfOpenAttempts = 0
		log.Printf("Circuit breaker renormalized to half-open state")
	}
}

/*
RecordFailure records a failure and updates the circuit state.
This method tracks the number of failures and opens the circuit if
the failure threshold is exceeded. It handles state transitions
differently based on the current circuit state.
*/
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

/*
RecordSuccess records a successful attempt and updates the circuit state.
This method handles the transition from half-open to closed state after
successful operations, and resets failure counts in closed state.
*/
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
	} else if cb.state == CircuitClosed {
		// Reset failure count on success in closed state
		cb.failureCount = 0
	}
}

/*
Allow determines if a request is allowed based on the circuit state.
This method implements the core circuit breaker logic, determining whether
to allow requests based on the current state and timing conditions.

Returns:
  - bool: true if the request should be allowed, false if it should be rejected
*/
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if time.Since(cb.openTime) > cb.resetTimeout {
			cb.state = CircuitHalfOpen
			cb.halfOpenAttempts = 0
			return true
		}
		return false
	case CircuitHalfOpen:
		return cb.halfOpenAttempts < cb.halfOpenMax
	default:
		return false
	}
}
