package qpool

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestQScheduleJobLifecycle(t *testing.T) {
	Convey("Schedule runs a job and delivers the result", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		defer cancel()

		q := NewQ[any](ctx, 2, 5, &Config{
			SchedulingTimeout: time.Second,
			Scaler:            nil,
		})

		defer q.Close()

		var ran atomic.Bool

		wait := q.Schedule("job-1", func(ctx context.Context) (any, error) {
			ran.Store(true)

			return "ok", nil
		})

		deadline := time.Now().Add(2 * time.Second)

		for !ran.Load() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}

		So(ran.Load(), ShouldBeTrue)

		res, err := wait.Get(ctx)

		So(err, ShouldBeNil)
		So(res, ShouldNotBeNil)
		So(ArtifactError(res), ShouldBeNil)

		value, valueErr := ArtifactValue[string](res)

		So(valueErr, ShouldBeNil)
		So(value, ShouldEqual, "ok")
	})
}

func TestQScheduleRegulatorRejects(t *testing.T) {
	Convey("Regulators can reject Schedule before enqueue", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		defer cancel()

		cfg := NewConfig()
		cfg.Scaler = nil
		cfg.Regulators = []Regulator{NewRateLimiter(0, time.Hour)}

		q := NewQ[any](ctx, 1, 2, cfg)

		defer q.Close()

		wait := q.Schedule("blocked", func(ctx context.Context) (any, error) {
			return "nope", nil
		})

		res, err := wait.Get(context.Background())

		So(err, ShouldBeNil)
		So(res, ShouldNotBeNil)

		artifactErr := ArtifactError(res)

		So(artifactErr, ShouldNotBeNil)
		So(artifactErr.Error(), ShouldContainSubstring, "rejected")
	})
}

func TestPeekResultReadsStoredJob(t *testing.T) {
	Convey("PeekResult returns stored completion after job finishes", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		defer cancel()

		q := NewQ[any](ctx, 1, 2, &Config{Scaler: nil})

		defer q.Close()

		wait := q.Schedule("peek-src", func(ctx context.Context) (any, error) {
			return 77, nil
		})

		res, err := wait.Get(context.Background())

		So(err, ShouldBeNil)
		So(res, ShouldNotBeNil)
		So(ArtifactError(res), ShouldBeNil)

		pv, ok := q.PeekResult("peek-src")

		So(ok, ShouldBeTrue)
		So(pv, ShouldNotBeNil)
		So(ArtifactError(pv), ShouldBeNil)

		peekValue, peekErr := ArtifactValue[int](pv)

		So(peekErr, ShouldBeNil)
		So(peekValue, ShouldEqual, 77)
	})
}

func TestDependencyFailureStoresError(t *testing.T) {
	Convey("missing dependency yields error result", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)

		defer cancel()

		q := NewQ[any](ctx, 1, 2, &Config{Scaler: nil})

		defer q.Close()

		wait := q.Schedule("child", func(ctx context.Context) (any, error) {
			return "x", nil
		}, WithDependencies([]string{"missing"}))

		res, err := wait.Get(context.Background())

		So(err, ShouldBeNil)
		So(res, ShouldNotBeNil)

		artifactErr := ArtifactError(res)

		So(artifactErr, ShouldNotBeNil)
		So(artifactErr.Error(), ShouldContainSubstring, "dependency missing")
	})
}
