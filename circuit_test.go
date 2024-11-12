package qpool

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCircuitBreaker(t *testing.T) {
	Convey("Given a circuit breaker", t, func() {
		cb := &CircuitBreaker{
			maxFailures:  3,
			resetTimeout: time.Second * 2, // Increased from 1 second
			halfOpenMax:  2,
		}

		// Test initial state
		So(cb.state, ShouldEqual, CircuitClosed)

		// Add failures until circuit opens
		for i := 0; i < cb.maxFailures; i++ {
			cb.RecordFailure()
		}

		// Wait for state transition
		time.Sleep(10 * time.Millisecond)
		So(cb.state, ShouldEqual, CircuitOpen)

		Convey("After reset timeout", func() {
			time.Sleep(time.Millisecond * 300)

			Convey("Circuit should be half-open", func() {
				So(cb.Allow(), ShouldBeTrue)
				So(cb.state, ShouldEqual, CircuitHalfOpen)
			})

			Convey("Successful requests in half-open state should close circuit", func() {
				cb.RecordSuccess()
				cb.RecordSuccess()
				So(cb.state, ShouldEqual, CircuitClosed)
			})
		})
	})
}
