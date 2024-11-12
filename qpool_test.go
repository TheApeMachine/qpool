package qpool

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestQuantumPool(t *testing.T) {
	Convey("Given a new quantum pool", t, func() {
		ctx := context.Background()
		q := NewQ(ctx, 2, 5)

		Convey("When scheduling a simple job", func() {
			result := q.Schedule("test-job", func() (any, error) {
				return "success", nil
			})

			Convey("It should complete successfully", func() {
				value := <-result
				So(value.Error, ShouldBeNil)
				So(value.Value, ShouldEqual, "success")
			})
		})

		Convey("When scheduling a job with retries", func() {
			attempts := 0
			result := q.Schedule("retry-job", func() (any, error) {
				attempts++
				if attempts < 3 {
					return nil, errors.New("temporary error")
				}
				return "success after retry", nil
			}, WithRetry(3, &ExponentialBackoff{Initial: time.Millisecond}))

			Convey("It should retry and eventually succeed", func() {
				value := <-result
				So(value.Error, ShouldBeNil)
				So(value.Value, ShouldEqual, "success after retry")
				So(attempts, ShouldEqual, 3)
			})
		})

		Convey("When using a circuit breaker", func() {
			failures := 0
			cb := &CircuitBreaker{
				maxFailures:  2,
				resetTimeout: time.Millisecond * 100,
				halfOpenMax:  1,
			}
			q.breakers["test-circuit"] = cb

			result := q.Schedule("circuit-job", func() (any, error) {
				failures++
				return nil, errors.New("failure")
			}, WithCircuitBreaker("test-circuit", 2, time.Millisecond*100))

			Convey("It should open after max failures", func() {
				<-result // First failure
				<-result // Second failure

				So(cb.Allow(), ShouldBeFalse)
				So(failures, ShouldEqual, 2)
			})
		})

		Convey("When using broadcast groups", func() {
			group := q.CreateBroadcastGroup("test-group", time.Minute)
			sub1 := q.Subscribe("test-group")
			sub2 := q.Subscribe("test-group")

			Convey("Messages should be received by all subscribers", func() {
				testValue := QuantumValue{Value: "broadcast test", CreatedAt: time.Now()}
				group.Send(testValue)

				value1 := <-sub1
				value2 := <-sub2

				So(value1.Value, ShouldEqual, "broadcast test")
				So(value2.Value, ShouldEqual, "broadcast test")
			})
		})

		Convey("When testing dependencies", func() {
			// Schedule parent job
			parentResult := q.Schedule("parent-job", func() (any, error) {
				return "parent done", nil
			})

			// Wait for parent to complete
			<-parentResult

			// Schedule child job with dependency
			childResult := q.Schedule("child-job", func() (any, error) {
				return "child done", nil
			}, func(j *Job) {
				j.Dependencies = []string{"parent-job"}
			})

			Convey("Child job should execute after parent completes", func() {
				value := <-childResult
				So(value.Error, ShouldBeNil)
				So(value.Value, ShouldEqual, "child done")
			})
		})

		Convey("When testing TTL", func() {
			result := q.Schedule("ttl-job", func() (any, error) {
				return "ttl test", nil
			}, WithTTL(time.Millisecond*100))

			Convey("Value should be available before TTL expires", func() {
				value := <-result
				So(value.Error, ShouldBeNil)
				So(value.Value, ShouldEqual, "ttl test")
			})

			Convey("Value should be cleaned up after TTL", func() {
				time.Sleep(time.Millisecond * 150)
				q.space.mu.RLock()
				_, exists := q.space.values["ttl-job"]
				q.space.mu.RUnlock()
				So(exists, ShouldBeFalse)
			})
		})
	})
}

func TestScaler(t *testing.T) {
	Convey("Given a pool with scaling enabled", t, func() {
		ctx := context.Background()
		q := NewQ(ctx, 2, 10)

		Convey("When load increases", func() {
			// Simulate high load
			for i := 0; i < 20; i++ {
				q.Schedule(fmt.Sprintf("load-test-%d", i), func() (any, error) {
					time.Sleep(time.Millisecond * 100)
					return nil, nil
				})
			}

			time.Sleep(time.Second * 2) // Allow time for scaling

			Convey("Worker count should increase", func() {
				q.metrics.mu.RLock()
				count := q.metrics.WorkerCount
				q.metrics.mu.RUnlock()
				So(count, ShouldBeGreaterThan, 2)
			})
		})
	})
}

func TestCircuitBreaker(t *testing.T) {
	Convey("Given a circuit breaker", t, func() {
		cb := &CircuitBreaker{
			maxFailures:  3,
			resetTimeout: time.Millisecond * 100,
			halfOpenMax:  2,
		}

		Convey("When failures occur", func() {
			for i := 0; i < 3; i++ {
				cb.RecordFailure()
			}

			Convey("Circuit should open", func() {
				So(cb.Allow(), ShouldBeFalse)
			})

			Convey("After reset timeout", func() {
				time.Sleep(time.Millisecond * 150)

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
	})
}

func TestQuantumSpace(t *testing.T) {
	Convey("Given a quantum space", t, func() {
		qs := newQuantumSpace()

		Convey("When storing and retrieving values", func() {
			qs.Store("test-key", "test-value", nil, time.Minute)

			Convey("Value should be retrievable", func() {
				ch := qs.Await("test-key")
				value := <-ch
				So(value.Value, ShouldEqual, "test-value")
				So(value.Error, ShouldBeNil)
			})
		})

		Convey("When using broadcast groups", func() {
			group := qs.CreateBroadcastGroup("test-group", time.Minute)
			sub1 := qs.Subscribe("test-group")
			sub2 := qs.Subscribe("test-group")

			Convey("All subscribers should receive messages", func() {
				testValue := QuantumValue{Value: "broadcast message", CreatedAt: time.Now()}
				group.Send(testValue)

				msg1 := <-sub1
				msg2 := <-sub2

				So(msg1.Value, ShouldEqual, "broadcast message")
				So(msg2.Value, ShouldEqual, "broadcast message")
			})
		})

		Reset(func() {
			qs.Close()
		})
	})
}
