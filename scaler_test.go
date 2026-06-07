package qpool

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestScalerLimit(test *testing.T) {
	Convey("Given a scaler", test, func() {
		scaler := &Scaler{
			maxWorkers:       4,
			scaleUpThreshold: 2,
		}

		scaler.reading.Store(&MetricReading{
			WorkerCount:  4,
			JobQueueSize: 100,
		})

		Convey("It should never reject schedules", func() {
			So(scaler.Limit(), ShouldBeFalse)
		})
	})
}

func TestScalerObserveScalesUpUnderLoad(test *testing.T) {
	Convey("Given a pool with the default scaler", test, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

		defer cancel()

		pool := NewQ[any](ctx, 1, 4, NewConfig())

		defer pool.Close()

		waits := make([]*ResultWait[any], 0, 12)

		for index := range 12 {
			index := index

			wait := pool.Schedule(fmt.Sprintf("job-%d", index), func(jobCtx context.Context) (any, error) {
				time.Sleep(25 * time.Millisecond)

				return index, nil
			})

			waits = append(waits, wait)
		}

		Convey("It should scale workers up when queue depth exceeds the threshold", func() {
			deadline := time.Now().Add(2 * time.Second)

			for pool.MetricSnapshot().WorkerCount < 2 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}

			So(pool.MetricSnapshot().WorkerCount, ShouldBeGreaterThan, 1)

			for _, wait := range waits {
				result, err := wait.Get(ctx)

				So(err, ShouldBeNil)
				So(result, ShouldNotBeNil)
				So(result.Error, ShouldBeNil)
			}
		})
	})
}

func TestScheduleWithScalerDoesNotReturnPoolSaturated(test *testing.T) {
	Convey("Given a pool at max workers under load", test, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)

		defer cancel()

		pool := NewQ[any](ctx, 2, 2, NewConfig())

		defer pool.Close()

		waits := make([]*ResultWait[any], 0, 16)

		for index := range 16 {
			index := index

			wait := pool.Schedule(fmt.Sprintf("burst-%d", index), func(jobCtx context.Context) (any, error) {
				time.Sleep(5 * time.Millisecond)

				return index, nil
			})

			waits = append(waits, wait)
		}

		Convey("It should complete every schedule without pool saturated errors", func() {
			for _, wait := range waits {
				result, err := wait.Get(ctx)

				So(err, ShouldBeNil)
				So(result, ShouldNotBeNil)

				if result.Error != nil {
					So(result.Error.Error(), ShouldNotContainSubstring, "pool saturated")
				}
			}
		})
	})
}

func BenchmarkScalerObserve(benchmark *testing.B) {
	ctx := context.Background()
	pool := NewQ[any](ctx, 2, 8, NewConfig())

	defer pool.Close()

	reading := pool.metrics.CollectReading()

	benchmark.ReportAllocs()
	benchmark.ResetTimer()

	for benchmark.Loop() {
		pool.scaler.Observe(reading)
	}
}
