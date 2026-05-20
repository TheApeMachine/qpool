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
		pool := NewQ(ctx, 1, 1, &Config{
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
			parentValue := receiveQValue(test, parentResult)

			So(parentValue, ShouldNotBeNil)
			So(parentValue.Error, ShouldBeNil)
			So(parentValue.Value, ShouldEqual, "parent")

			childValue := receiveQValue(test, childResult)

			So(childValue, ShouldNotBeNil)
			So(childValue.Error, ShouldBeNil)
			So(childValue.Value, ShouldEqual, "child")
		})
	})
}

func receiveQValue(test *testing.T, resultChannel <-chan *QValue) *QValue {
	test.Helper()

	select {
	case result := <-resultChannel:
		return result
	case <-time.After(time.Second):
		test.Fatal("timed out waiting for qpool result")
	}

	return nil
}
