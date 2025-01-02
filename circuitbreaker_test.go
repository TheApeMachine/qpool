package qpool

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCircuitBreakerInterface(t *testing.T) {
	Convey("Given a circuit breaker implementing Regulator interface", t, func() {
		breaker := NewCircuitBreaker(2, 100*time.Millisecond, 1)
		
		Convey("It should implement Regulator interface", func() {
			var _ Regulator = breaker // Compile-time interface check
			
			metrics := &Metrics{}
			breaker.Observe(metrics)
			So(breaker.Limit(), ShouldBeFalse) // Initially should not limit
		})
	})
}

func TestCircuitBreakerInitialState(t *testing.T) {
	Convey("Given a newly created circuit breaker", t, func() {
		breaker := NewCircuitBreaker(2, 100*time.Millisecond, 1)
		
		Convey("It should start in closed state", func() {
			So(breaker.Allow(), ShouldBeTrue)
			So(breaker.state, ShouldEqual, CircuitClosed)
		})
	})
}

func TestCircuitBreakerFailureThreshold(t *testing.T) {
	Convey("Given a circuit breaker with failure threshold", t, func() {
		breaker := NewCircuitBreaker(2, 100*time.Millisecond, 1)
		
		Convey("It should open after max failures", func() {
			breaker.RecordFailure()
			breaker.RecordFailure()

			So(breaker.Allow(), ShouldBeFalse)
			So(breaker.state, ShouldEqual, CircuitOpen)

			// Wait for reset timeout
			time.Sleep(150 * time.Millisecond)

			So(breaker.Allow(), ShouldBeTrue)
			So(breaker.state, ShouldEqual, CircuitHalfOpen)
		})
	})
}

func TestCircuitBreakerHalfOpenSuccess(t *testing.T) {
	Convey("Given a circuit breaker in half-open state", t, func() {
		breaker := NewCircuitBreaker(2, 100*time.Millisecond, 1)
		
		Convey("It should close after successful attempt", func() {
			breaker.RecordFailure()
			breaker.RecordFailure()

			time.Sleep(150 * time.Millisecond)

			So(breaker.Allow(), ShouldBeTrue)
			So(breaker.state, ShouldEqual, CircuitHalfOpen)

			// Simulate a successful attempt
			So(breaker.Allow(), ShouldBeTrue)
			breaker.RecordSuccess()

			So(breaker.state, ShouldEqual, CircuitClosed)
		})
	})
}

func TestCircuitBreakerHalfOpenFailure(t *testing.T) {
	Convey("Given a circuit breaker in half-open state", t, func() {
		breaker := NewCircuitBreaker(2, 100*time.Millisecond, 1)
		
		Convey("It should open again after failure", func() {
			breaker.RecordFailure()
			breaker.RecordFailure()

			time.Sleep(150 * time.Millisecond)

			So(breaker.Allow(), ShouldBeTrue)
			So(breaker.state, ShouldEqual, CircuitHalfOpen)

			// Simulate a failed attempt
			breaker.RecordFailure()

			So(breaker.state, ShouldEqual, CircuitOpen)
		})
	})
}

func TestCircuitBreakerSuccessReset(t *testing.T) {
	Convey("Given a circuit breaker in closed state", t, func() {
		breaker := NewCircuitBreaker(2, 100*time.Millisecond, 1)
		
		Convey("It should reset failure count on success", func() {
			// Record one failure (not enough to open)
			breaker.RecordFailure()

			// Record success which should reset the failure count
			breaker.RecordSuccess()

			// Record another single failure
			breaker.RecordFailure()

			// Circuit should still be closed since failure count was reset
			So(breaker.Allow(), ShouldBeTrue)
			So(breaker.state, ShouldEqual, CircuitClosed)
			So(breaker.failureCount, ShouldEqual, 1)
		})
	})
}

func TestCircuitBreakerRenormalize(t *testing.T) {
	Convey("Given a circuit breaker in open state", t, func() {
		breaker := NewCircuitBreaker(2, 100*time.Millisecond, 1)
		
		Convey("It should properly renormalize", func() {
			breaker.RecordFailure()
			breaker.RecordFailure()

			So(breaker.Allow(), ShouldBeFalse)
			So(breaker.state, ShouldEqual, CircuitOpen)

			time.Sleep(150 * time.Millisecond)
			breaker.Renormalize()

			So(breaker.state, ShouldEqual, CircuitHalfOpen)
			So(breaker.halfOpenAttempts, ShouldEqual, 0)
		})
	})
}
