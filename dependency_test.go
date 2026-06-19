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

			payload := parentValue.DecryptPayload()
			So(string(payload), ShouldEqual, "parent")

			So(parentValue, ShouldNotBeNil)
			So(ArtifactError(parentValue), ShouldBeNil)

			childValue := receiveResultWait(test, childResult)

			payload = childValue.DecryptPayload()
			So(string(payload), ShouldEqual, "child")

			So(childValue, ShouldNotBeNil)
			So(string(payload), ShouldEqual, "child")
		})
	})
}
