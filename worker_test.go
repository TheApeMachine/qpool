package qpool

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestQScheduleJobLifecycle(t *testing.T) {
	Convey("Schedule runs a job and delivers the result", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		defer cancel()

		q := NewQ(ctx, 2, 5, &Config{
			SchedulingTimeout: time.Second,
			Scaler:            nil,
		})

		defer q.Close()

		done := make(chan struct{})

		ch := q.Schedule("job-1", func(ctx context.Context) (any, error) {
			close(done)

			return "ok", nil
		})

		select {
		case <-time.After(2 * time.Second):
			t.Fatal("job fn never ran")
		case <-done:
		}

		select {
		case <-time.After(2 * time.Second):
			t.Fatal("result timeout")
		case res := <-ch:
			So(res, ShouldNotBeNil)
			So(res.Error, ShouldBeNil)
			So(res.Value, ShouldEqual, "ok")
		}
	})
}

func TestQScheduleRegulatorRejects(t *testing.T) {
	Convey("Regulators can reject Schedule before enqueue", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		defer cancel()

		cfg := NewConfig()
		cfg.Scaler = nil
		cfg.Regulators = []Regulator{NewRateLimiter(0, time.Hour)}

		q := NewQ(ctx, 1, 2, cfg)

		defer q.Close()

		ch := q.Schedule("blocked", func(ctx context.Context) (any, error) {
			return "nope", nil
		})

		select {
		case <-time.After(time.Second):
			t.Fatal("expected immediate regulator rejection")
		case res := <-ch:
			So(res, ShouldNotBeNil)
			So(res.Error, ShouldNotBeNil)
			So(res.Error.Error(), ShouldContainSubstring, "rejected")
		}
	})
}

func TestPeekResultReadsStoredJob(t *testing.T) {
	Convey("PeekResult returns stored completion after job finishes", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		defer cancel()

		q := NewQ(ctx, 1, 2, &Config{Scaler: nil})

		defer q.Close()

		ch := q.Schedule("peek-src", func(ctx context.Context) (any, error) {
			return 77, nil
		})

		var res *QValue

		select {
		case <-time.After(2 * time.Second):
			t.Fatal("result timeout waiting for peek-src job")
		case res = <-ch:
		}

		So(res, ShouldNotBeNil)
		So(res.Error, ShouldBeNil)

		pv, ok := q.PeekResult("peek-src")

		So(ok, ShouldBeTrue)
		So(pv, ShouldNotBeNil)
		So(pv.Error, ShouldBeNil)
		So(pv.Value, ShouldEqual, 77)
	})
}

func TestDependencyFailureStoresError(t *testing.T) {
	Convey("missing dependency yields error result", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)

		defer cancel()

		q := NewQ(ctx, 1, 2, &Config{Scaler: nil})

		defer q.Close()

		ch := q.Schedule("child", func(ctx context.Context) (any, error) {
			return "x", nil
		}, WithDependencies([]string{"missing"}))

		select {
		case <-time.After(3 * time.Second):
			t.Fatal("timeout waiting dependency failure")
		case res := <-ch:
			So(res, ShouldNotBeNil)
			So(res.Error, ShouldNotBeNil)
			So(res.Error.Error(), ShouldContainSubstring, "dependency missing")
		}
	})
}
