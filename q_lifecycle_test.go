package qpool

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWorkerRegistryPushPopRemove(test *testing.T) {
	Convey("Given a worker registry", test, func() {
		registry := newWorkerRegistry()
		first := &workerToken{id: 1, cancel: func() {}}
		second := &workerToken{id: 2, cancel: func() {}}
		third := &workerToken{id: 3, cancel: func() {}}

		registry.push(first)
		registry.push(second)
		registry.push(third)

		Convey("It should remove arbitrary workers and pop the newest remaining worker", func() {
			registry.remove(second.id)

			So(registry.popLast(), ShouldEqual, third)
			So(registry.popLast(), ShouldEqual, first)
			So(registry.popLast(), ShouldBeNil)
		})
	})
}

func TestQCloseUnblocksBlockedSchedule(test *testing.T) {
	Convey("Given a Q pool with a full job channel", test, func() {
		ctx, cancel := context.WithCancel(context.Background())
		pool := NewQ(ctx, 1, 1, &Config{
			SchedulingTimeout:  5 * time.Second,
			JobChannelCapacity: 1,
			Scaler:             nil,
		})

		defer cancel()
		defer pool.Close()

		started := make(chan struct{})

		_ = pool.Schedule("running", func(jobCtx context.Context) (any, error) {
			close(started)
			<-jobCtx.Done()

			return nil, jobCtx.Err()
		})

		select {
		case <-started:
		case <-time.After(time.Second):
			test.Fatal("running job did not start")
		}

		_ = pool.Schedule("queued", func(jobCtx context.Context) (any, error) {
			return "queued", nil
		})

		scheduleEntered := make(chan struct{})
		blockedResult := make(chan *QValue, 1)

		go func() {
			close(scheduleEntered)

			resultChannel := pool.Schedule("blocked", func(jobCtx context.Context) (any, error) {
				return "blocked", nil
			})

			result, ok := <-resultChannel

			if !ok {
				blockedResult <- &QValue{Error: fmt.Errorf("result channel closed")}

				return
			}

			blockedResult <- result
		}()

		select {
		case <-scheduleEntered:
		case <-time.After(time.Second):
			test.Fatal("blocked schedule goroutine did not start")
		}

		time.Sleep(25 * time.Millisecond)

		closeDuration := make(chan time.Duration, 1)
		startedAt := time.Now()

		go func() {
			pool.Close()
			closeDuration <- time.Since(startedAt)
		}()

		select {
		case duration := <-closeDuration:
			So(duration, ShouldBeLessThan, time.Second)
		case <-time.After(time.Second):
			test.Fatal("Close waited for the scheduling timeout")
		}

		select {
		case result := <-blockedResult:
			So(result, ShouldNotBeNil)
			So(result.Error, ShouldNotBeNil)
			So(result.Error.Error(), ShouldNotContainSubstring, "scheduling timeout")
		case <-time.After(time.Second):
			test.Fatal("blocked schedule did not unblock")
		}
	})
}

func BenchmarkWorkerRegistryPushPop(benchmark *testing.B) {
	registry := newWorkerRegistry()
	token := &workerToken{id: 1, cancel: func() {}}

	benchmark.ReportAllocs()
	benchmark.ResetTimer()

	for benchmark.Loop() {
		registry.push(token)
		_ = registry.popLast()
	}
}
