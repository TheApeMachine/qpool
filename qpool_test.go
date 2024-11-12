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
	Convey("Given a new quantum pool", t, func(c C) {
		ctx, cancel := context.WithCancel(context.Background())
		q := NewQ(ctx, 2, 5)

		Reset(func() {
			cancel()
			time.Sleep(100 * time.Millisecond)
		})

		Convey("When scheduling a simple job", func(c C) {
			result := q.Schedule("test-job", func() (any, error) {
				return "success", nil
			})

			value := <-result
			c.So(value.Error, ShouldBeNil)
			c.So(value.Value, ShouldEqual, "success")
		})

		Convey("When scheduling a job with retries", func(c C) {
			attempts := 0
			result := q.Schedule("retry-job", func() (any, error) {
				attempts++
				if attempts < 3 {
					return nil, errors.New("temporary error")
				}
				return "success after retry", nil
			}, WithRetry(3, &ExponentialBackoff{Initial: time.Millisecond}))

			value := <-result
			c.So(value.Error, ShouldBeNil)
			c.So(value.Value, ShouldEqual, "success after retry")
			c.So(attempts, ShouldEqual, 3)
		})

		Convey("When using a circuit breaker", func(c C) {
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

			<-result // First failure
			<-result // Second failure

			c.So(cb.Allow(), ShouldBeFalse)
			c.So(failures, ShouldEqual, 2)
		})

		Convey("When using broadcast groups", func(c C) {
			group := q.CreateBroadcastGroup("test-group", time.Minute)
			sub1 := q.Subscribe("test-group")
			sub2 := q.Subscribe("test-group")

			testValue := QuantumValue{Value: "broadcast test", CreatedAt: time.Now()}
			group.Send(testValue)

			value1 := <-sub1
			value2 := <-sub2

			c.So(value1.Value, ShouldEqual, "broadcast test")
			c.So(value2.Value, ShouldEqual, "broadcast test")

			// Clean up
			close(sub1)
			close(sub2)
		})
	})
}

func TestScaler(t *testing.T) {
	Convey("Given a pool with scaling enabled", t, func(c C) {
		ctx, cancel := context.WithCancel(context.Background())
		q := NewQ(ctx, 2, 10)

		Reset(func() {
			cancel()
			time.Sleep(100 * time.Millisecond)
		})

		Convey("When load increases", func(c C) {
			// Simulate high load
			for i := 0; i < 20; i++ {
				q.Schedule(fmt.Sprintf("load-test-%d", i), func() (any, error) {
					time.Sleep(time.Millisecond * 50)
					return nil, nil
				})
			}

			// Allow time for scaling
			time.Sleep(time.Second)

			q.metrics.mu.RLock()
			count := q.metrics.WorkerCount
			q.metrics.mu.RUnlock()

			c.So(count, ShouldBeGreaterThan, 2)
		})
	})
}

func TestQuantumSpace(t *testing.T) {
	Convey("Given a quantum space", t, func(c C) {
		qs := newQuantumSpace()

		Reset(func() {
			qs.Close()
		})

		Convey("When storing and retrieving values", func(c C) {
			qs.Store("test-key", "test-value", nil, time.Minute)
			ch := qs.Await("test-key")
			value := <-ch
			c.So(value.Value, ShouldEqual, "test-value")
			c.So(value.Error, ShouldBeNil)
		})

		Convey("When using broadcast groups", func(c C) {
			group := qs.CreateBroadcastGroup("test-group", time.Minute)
			sub1 := qs.Subscribe("test-group")
			sub2 := qs.Subscribe("test-group")

			testValue := QuantumValue{Value: "broadcast test", CreatedAt: time.Now()}
			group.Send(testValue)

			value1 := <-sub1
			value2 := <-sub2

			c.So(value1.Value, ShouldEqual, "broadcast test")
			c.So(value2.Value, ShouldEqual, "broadcast test")

			// Clean up
			close(sub1)
			close(sub2)
		})
	})
}
