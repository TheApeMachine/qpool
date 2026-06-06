package qpool

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestQScheduleDependencyWaitDoesNotStarveWorkers(test *testing.T) {
	Convey("Given a single-worker pool and a dependent job scheduled first", test, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		pool := NewQ[any](ctx, 1, 1, &Config{
			SchedulingTimeout: time.Second,
			Scaler:            nil,
		})

		defer cancel()
		defer pool.Close()

		childResult := pool.Schedule("child", func(jobCtx context.Context) (any, error) {
			return "child", nil
		}, WithDependencies([]string{"parent"}), WithDependencyAwaitTimeout(3*time.Second))

		parentResult := pool.Schedule("parent", func(jobCtx context.Context) (any, error) {
			return "parent", nil
		})

		Convey("It should run the dependency producer before the dependent job occupies the worker", func() {
			parentValue := receiveResultWait(test, parentResult)

			So(parentValue, ShouldNotBeNil)
			So(parentValue.Error, ShouldBeNil)
			So(parentValue.Value, ShouldEqual, "parent")

			childValue := receiveResultWait(test, childResult)

			So(childValue, ShouldNotBeNil)
			So(childValue.Error, ShouldBeNil)
			So(childValue.Value, ShouldEqual, "child")
		})
	})
}

