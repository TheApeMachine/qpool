package qpool

import (
	"context"
	"sync/atomic"
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
	Convey("Given a Q pool with a full scheduled-job disruptor queue", test, func() {
		ctx, cancel := context.WithCancel(context.Background())
		pool := NewQ[any](ctx, 1, 1, &Config{
			SchedulingTimeout:  5 * time.Second,
			JobChannelCapacity: 1,
			Scaler:             nil,
		})

		defer cancel()
		defer pool.Close()

		var runningStarted atomic.Bool

		_ = pool.Schedule("running", func(jobCtx context.Context) (any, error) {
			runningStarted.Store(true)
			<-jobCtx.Done()

			return nil, jobCtx.Err()
		})

		deadline := time.Now().Add(time.Second)

		for !runningStarted.Load() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}

		So(runningStarted.Load(), ShouldBeTrue)

		_ = pool.Schedule("queued", func(jobCtx context.Context) (any, error) {
			return "queued", nil
		})

		var scheduleStarted atomic.Bool
		var blockedResult atomic.Pointer[QValue[any]]

		go func() {
			scheduleStarted.Store(true)

			wait := pool.Schedule("blocked", func(jobCtx context.Context) (any, error) {
				return "blocked", nil
			})

			result, err := wait.Get(context.Background())
			if err != nil {
				blockedResult.Store(&QValue[any]{Error: err})

				return
			}

			blockedResult.Store(result)
		}()

		deadline = time.Now().Add(time.Second)

		for !scheduleStarted.Load() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}

		So(scheduleStarted.Load(), ShouldBeTrue)

		time.Sleep(25 * time.Millisecond)

		var closeDuration atomic.Int64
		startedAt := time.Now()

		go func() {
			pool.Close()
			closeDuration.Store(time.Since(startedAt).Nanoseconds())
		}()

		deadline = time.Now().Add(time.Second)

		for closeDuration.Load() == 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}

		So(closeDuration.Load(), ShouldBeGreaterThan, 0)
		So(time.Duration(closeDuration.Load()), ShouldBeLessThan, time.Second)

		deadline = time.Now().Add(time.Second)

		for blockedResult.Load() == nil && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}

		result := blockedResult.Load()

		So(result, ShouldNotBeNil)
		So(result.Error, ShouldNotBeNil)
		So(result.Error.Error(), ShouldNotContainSubstring, "scheduling timeout")
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
