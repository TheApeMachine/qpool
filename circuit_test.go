package qpool

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCircuitBreaker(t *testing.T) {
	Convey("Given a circuit breaker", t, func() {
		breaker := &CircuitBreaker{
			maxFailures:  2,
			resetTimeout: 100 * time.Millisecond,
			halfOpenMax:  1,
			state:        CircuitClosed,
		}

		Convey("It should start in closed state", func() {
			So(breaker.Allow(), ShouldBeTrue)
			So(breaker.state, ShouldEqual, CircuitClosed)
		})

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

		Convey("It should close after successful attempt in half-open state", func() {
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

		Convey("It should open again after failure in half-open state", func() {
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
