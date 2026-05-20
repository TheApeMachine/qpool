package qpool

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewCircuitBreaker(t *testing.T) {
	Convey("Given NewCircuitBreaker", t, func() {
		reset := time.Minute

		cases := []struct {
			name            string
			halfOpenMaxIn   int
			halfOpenMaxWant int
		}{
			{name: "negative halfOpenMax normalizes to one", halfOpenMaxIn: -4, halfOpenMaxWant: 1},
			{name: "zero halfOpenMax normalizes to one", halfOpenMaxIn: 0, halfOpenMaxWant: 1},
			{name: "positive halfOpenMax preserved", halfOpenMaxIn: 5, halfOpenMaxWant: 5},
		}

		for _, row := range cases {
			label := row.name

			Convey(fmt.Sprintf("When %s", label), func() {
				breaker := NewCircuitBreaker(3, reset, row.halfOpenMaxIn)

				So(breaker.halfOpenMax, ShouldEqual, row.halfOpenMaxWant)
				So(breaker.maxFailures, ShouldEqual, 3)
				So(breaker.resetTimeout, ShouldEqual, reset)
				So(breaker.state.Load(), ShouldEqual, cbClosed)
			})
		}
	})
}

func BenchmarkNewCircuitBreaker(b *testing.B) {
	for b.Loop() {
		_ = NewCircuitBreaker(5, time.Minute, 2)
	}
}

func TestNewCircuitBreakerFromConfig(t *testing.T) {
	Convey("Given newCircuitBreakerFromConfig", t, func() {
		cases := []struct {
			name         string
			config       *CircuitBreakerConfig
			wantFailures int
			wantReset    time.Duration
			wantHalfOpen int
		}{
			{
				name: "copies fields from job circuit config",
				config: &CircuitBreakerConfig{
					MaxFailures:  4,
					ResetTimeout: 33 * time.Second,
					HalfOpenMax:  6,
				},
				wantFailures: 4,
				wantReset:    33 * time.Second,
				wantHalfOpen: 6,
			},
			{
				name: "zero HalfOpenMax normalizes like NewCircuitBreaker",
				config: &CircuitBreakerConfig{
					MaxFailures:  2,
					ResetTimeout: time.Millisecond,
					HalfOpenMax:  0,
				},
				wantFailures: 2,
				wantReset:    time.Millisecond,
				wantHalfOpen: 1,
			},
		}

		for _, row := range cases {
			label := row.name

			Convey(fmt.Sprintf("When %s", label), func() {
				breaker := newCircuitBreakerFromConfig(row.config)

				So(breaker.maxFailures, ShouldEqual, row.wantFailures)
				So(breaker.resetTimeout, ShouldEqual, row.wantReset)
				So(breaker.halfOpenMax, ShouldEqual, row.wantHalfOpen)
				So(breaker.state.Load(), ShouldEqual, cbClosed)
			})
		}
	})
}

func BenchmarkNewCircuitBreakerFromConfig(b *testing.B) {
	cfg := &CircuitBreakerConfig{
		MaxFailures:  3,
		ResetTimeout: time.Minute,
		HalfOpenMax:  2,
	}

	for range b.N {
		_ = newCircuitBreakerFromConfig(cfg)
	}
}

func TestCircuitBreakerRegulatorInterface(t *testing.T) {
	Convey("Given a CircuitBreaker", t, func() {
		breaker := NewCircuitBreaker(2, 100*time.Millisecond, 2)

		Convey("It should satisfy the Regulator interface", func() {
			var _ Regulator = breaker
		})

		Convey("It should not limit when observed with empty metrics", func() {
			breaker.Observe(&MetricReading{})
			So(breaker.Limit(), ShouldBeFalse)
		})
	})
}

func TestCircuitBreakerOpensAfterFailures(t *testing.T) {
	Convey("breaker opens then permits probe after timeout", t, func() {
		breaker := NewCircuitBreaker(2, 100*time.Millisecond, 2)

		So(breaker.state.Load(), ShouldEqual, cbClosed)

		breaker.RecordFailure()
		breaker.RecordFailure()

		So(breaker.Allow(), ShouldBeFalse)
		So(breaker.state.Load(), ShouldEqual, cbOpen)

		time.Sleep(150 * time.Millisecond)

		So(breaker.Allow(), ShouldBeTrue)
		So(breaker.state.Load(), ShouldEqual, cbHalfOpen)
	})
}

func TestCircuitBreakerHalfOpenClosesAfterSuccesses(t *testing.T) {
	Convey("half-open collects successes then closes", t, func() {
		breaker := NewCircuitBreaker(2, 100*time.Millisecond, 2)

		breaker.RecordFailure()
		breaker.RecordFailure()

		time.Sleep(150 * time.Millisecond)

		/* Half-open allows up to halfOpenMax concurrent probes; two Allow() calls are expected. */
		So(breaker.Allow(), ShouldBeTrue)
		So(breaker.Allow(), ShouldBeTrue)

		breaker.RecordSuccess()

		So(breaker.state.Load(), ShouldEqual, cbHalfOpen)

		breaker.RecordSuccess()

		So(breaker.state.Load(), ShouldEqual, cbClosed)
	})
}

func TestCircuitBreakerRenormalize(t *testing.T) {
	Convey("Renormalize moves open breaker toward half-open", t, func() {
		breaker := NewCircuitBreaker(2, 100*time.Millisecond, 2)

		breaker.RecordFailure()
		breaker.RecordFailure()

		So(breaker.state.Load(), ShouldEqual, cbOpen)

		time.Sleep(150 * time.Millisecond)

		breaker.Renormalize()

		So(breaker.state.Load(), ShouldEqual, cbHalfOpen)
		So(breaker.halfOpenSuccess.Load(), ShouldEqual, 0)
	})
}
