package qpool

import (
	"context"
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

const (
	testTimeout    = 10 * time.Second
	channelTimeout = 5 * time.Second
	cleanupTimeout = 500 * time.Millisecond
)

func TestQuantumPool(t *testing.T) {
	Convey("Given a new quantum pool", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer func() {
			cancel()
			time.Sleep(cleanupTimeout)
		}()

		config := &Config{
			SchedulingTimeout: 5 * time.Second,
		}
		q := NewQ(ctx, 2, 5, config)

		Reset(func() {
			cancel()
			time.Sleep(cleanupTimeout)
		})

		Convey("When scheduling a simple job", func() {
			ctx, cancel := context.WithTimeout(ctx, channelTimeout)
			defer cancel()

			result := q.Schedule("test-job", func() (any, error) {
				return "success", nil
			})

			Convey("It should complete successfully", func() {
				select {
				case <-ctx.Done():
					t.Fatal("Test timed out waiting for job completion")
				case value := <-result:
					So(value.Error, ShouldBeNil)
					So(value.Value, ShouldEqual, "success")
				}
			})
		})

		Convey("When scheduling a job with retries", func() {
			ctx, cancel := context.WithTimeout(ctx, channelTimeout)
			defer cancel()

			attempts := 0
			result := q.Schedule("retry-job", func() (any, error) {
				attempts++
				if attempts < 3 {
					return nil, errors.New("temporary error")
				}
				return "success after retry", nil
			}, WithRetry(3, &ExponentialBackoff{Initial: time.Millisecond}))

			Convey("It should retry and eventually succeed", func() {
				select {
				case <-ctx.Done():
					t.Fatal("Test timed out waiting for retries")
				case value := <-result:
					So(value.Error, ShouldBeNil)
					So(value.Value, ShouldEqual, "success after retry")
					So(attempts, ShouldEqual, 3)
				}
			})
		})

		Convey("When using a circuit breaker", func() {
			ctx, cancel := context.WithTimeout(ctx, channelTimeout)
			defer cancel()

			failures := 0
			cb := &CircuitBreaker{
				maxFailures:  2,
				resetTimeout: time.Millisecond * 100,
				halfOpenMax:  1,
			}
			q.breakers["test-circuit"] = cb

			Convey("It should open after max failures", func() {
				for i := 0; i < 2; i++ {
					result := q.Schedule("circuit-job", func() (any, error) {
						failures++
						return nil, errors.New("failure")
					}, WithCircuitBreaker("test-circuit", 2, time.Millisecond*100))

					select {
					case <-ctx.Done():
						t.Fatal("Test timed out waiting for circuit breaker job")
					case <-result:
						// We expect these to fail, just ensuring they complete
					}
				}

				So(cb.Allow(), ShouldBeFalse)
				So(failures, ShouldEqual, 2)

				Convey("And should reset after timeout", func() {
					time.Sleep(cb.resetTimeout + time.Millisecond*10)
					So(cb.Allow(), ShouldBeTrue)
					So(cb.state, ShouldEqual, CircuitHalfOpen)
				})
			})
		})

		Convey("When using broadcast groups", func() {
			ctx, cancel := context.WithTimeout(ctx, channelTimeout)
			defer cancel()

			const groupName = "test-group"
			const broadcast = "broadcast test"

			group := q.CreateBroadcastGroup(groupName, time.Minute)
			sub1 := q.Subscribe(groupName)
			sub2 := q.Subscribe(groupName)

			defer func() {
				close(sub1)
				close(sub2)
			}()

			Convey("Messages should be received by all subscribers", func() {
				testValue := QuantumValue{Value: broadcast, CreatedAt: time.Now()}
				group.Send(testValue)

				select {
				case <-ctx.Done():
					t.Fatal("Test timed out waiting for first subscriber")
				case value1 := <-sub1:
					So(value1.Value, ShouldEqual, broadcast)
				}

				select {
				case <-ctx.Done():
					t.Fatal("Test timed out waiting for second subscriber")
				case value2 := <-sub2:
					So(value2.Value, ShouldEqual, broadcast)
				}
			})
		})

		Convey("When testing dependencies", func() {
			ctx, cancel := context.WithTimeout(ctx, channelTimeout)
			defer cancel()

			parentResult := q.Schedule("parent-job", func() (any, error) {
				return "parent done", nil
			})

			select {
			case <-ctx.Done():
				t.Fatal("Test timed out waiting for parent job")
			case value := <-parentResult:
				So(value.Error, ShouldBeNil)
				So(value.Value, ShouldEqual, "parent done")
			}

			childResult := q.Schedule("child-job", func() (any, error) {
				return "child done", nil
			}, func(j *Job) {
				j.Dependencies = []string{"parent-job"}
			})

			Convey("Child job should execute after parent completes", func() {
				select {
				case <-ctx.Done():
					t.Fatal("Test timed out waiting for child job")
				case value := <-childResult:
					So(value.Error, ShouldBeNil)
					So(value.Value, ShouldEqual, "child done")
				}
			})
		})

		Convey("When testing TTL", func() {
			ctx, cancel := context.WithTimeout(ctx, channelTimeout)
			defer cancel()

			result := q.Schedule("ttl-job", func() (any, error) {
				return "ttl test", nil
			}, WithTTL(time.Millisecond*100))

			Convey("Value should be available before TTL expires", func() {
				select {
				case <-ctx.Done():
					t.Fatal("Test timed out waiting for TTL job")
				case value := <-result:
					So(value.Error, ShouldBeNil)
					So(value.Value, ShouldEqual, "ttl test")
				}
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
