package qpool

import (
	"context"
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestQScheduleFastExecutesJob(test *testing.T) {
	Convey("Given a Q fast path", test, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		pool := NewQ[any](ctx, 1, 2, &Config{Scaler: nil})
		defer pool.Close()

		Convey("It should execute a fast job through the disruptor", func() {
			resultChannel := pool.ScheduleFast(ctx, func(jobCtx context.Context) (any, error) {
				if err := jobCtx.Err(); err != nil {
					return nil, err
				}

				return "fast", nil
			})

			result := receiveFastResult(test, resultChannel)

			So(result, ShouldNotBeNil)
			So(result.Error, ShouldBeNil)
			So(result.Value, ShouldEqual, "fast")
		})
	})
}

func TestQScheduleFastReturnsError(test *testing.T) {
	Convey("Given a Q fast path returning an error", test, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		pool := NewQ[any](ctx, 1, 2, &Config{Scaler: nil})
		defer pool.Close()

		Convey("It should deliver the job error", func() {
			expected := errors.New("fast failure")
			resultChannel := pool.ScheduleFast(ctx, func(context.Context) (any, error) {
				return nil, expected
			})

			result := receiveFastResult(test, resultChannel)

			So(result, ShouldNotBeNil)
			So(errors.Is(result.Error, expected), ShouldBeTrue)
		})
	})
}

func TestQScheduleFastRejectsClosedPool(test *testing.T) {
	Convey("Given a closed Q", test, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := NewQ[any](ctx, 1, 2, &Config{Scaler: nil})
		pool.Close()

		Convey("It should reject fast jobs", func() {
			resultChannel := pool.ScheduleFast(ctx, func(context.Context) (any, error) {
				return "unexpected", nil
			})

			result := receiveFastResult(test, resultChannel)

			So(result, ShouldNotBeNil)
			So(result.Error, ShouldNotBeNil)
			So(result.Error.Error(), ShouldContainSubstring, "pool closed")
		})
	})
}

func TestQScheduleFastRespectsPreCanceledContext(test *testing.T) {
	Convey("Given a pre-canceled fast job context", test, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		pool := NewQ[any](ctx, 1, 2, &Config{Scaler: nil})
		defer pool.Close()

		jobCtx, cancelJob := context.WithCancel(ctx)
		cancelJob()

		Convey("It should return the context cancellation", func() {
			resultChannel := pool.ScheduleFast(jobCtx, func(context.Context) (any, error) {
				return "unexpected", nil
			})

			result := receiveFastResult(test, resultChannel)

			So(result, ShouldNotBeNil)
			So(errors.Is(result.Error, context.Canceled), ShouldBeTrue)
		})
	})
}

func TestQScheduleFastRunsWhenJobQueueIsSaturated(test *testing.T) {
	Convey("Given a saturated scheduled-job disruptor queue", test, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		pool := NewQ[any](ctx, 1, 1, &Config{
			SchedulingTimeout:  500 * time.Millisecond,
			JobChannelCapacity: 1,
			Scaler:             nil,
		})
		defer pool.Close()

		started := make(chan struct{})
		release := make(chan struct{})

		_ = pool.Schedule("running-fast-independence", func(context.Context) (any, error) {
			close(started)
			<-release

			return "running", nil
		})

		select {
		case <-started:
		case <-time.After(time.Second):
			test.Fatal("running job did not start")
		}

		_ = pool.Schedule("queued-fast-independence", func(context.Context) (any, error) {
			return "queued", nil
		})

		Convey("It should still complete a fast job", func() {
			resultChannel := pool.ScheduleFast(ctx, func(context.Context) (any, error) {
				return "fast", nil
			})

			result := receiveFastResult(test, resultChannel)

			So(result, ShouldNotBeNil)
			So(result.Error, ShouldBeNil)
			So(result.Value, ShouldEqual, "fast")

			close(release)
		})
	})
}

func receiveFastResult(
	test *testing.T,
	resultChannel <-chan *QValue[any],
) *QValue[any] {
	test.Helper()

	select {
	case result := <-resultChannel:
		return result
	case <-time.After(time.Second):
		test.Fatal("timed out waiting for fast result")
	}

	return nil
}
