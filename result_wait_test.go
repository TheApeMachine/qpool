package qpool

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestResultSlotWait(test *testing.T) {
	Convey("Given a pending result slot", test, func() {
		slot := newResultSlot()
		waitDone := make(chan *QValue[erasedAny], 1)

		go func() {
			value, err := slot.Wait(context.Background())
			if err != nil {
				waitDone <- nil

				return
			}

			waitDone <- value
		}()

		Convey("It should deliver after Store races ahead of the waiter", func() {
			delivered, err := NewQValue[erasedAny]("", "", "ok", 0)

			So(err, ShouldBeNil)

			slot.Deliver(delivered)

			select {
			case result := <-waitDone:
				So(result, ShouldNotBeNil)
				So(result.Value, ShouldEqual, "ok")
			case <-time.After(time.Second):
				test.Fatal("timed out waiting for result slot delivery")
			}
		})
	})
}

func TestResultWaitGet(test *testing.T) {
	Convey("Given a Q pool schedule result", test, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		pool := NewQ[any](ctx, 1, 2, &Config{
			SchedulingTimeout: time.Second,
			Scaler:            nil,
		})

		defer cancel()
		defer pool.Close()

		wait := pool.Schedule("result-wait", func(jobCtx context.Context) (any, error) {
			return "done", nil
		})

		Convey("It should return the job value", func() {
			result, err := wait.Get(ctx)

			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(result.Error, ShouldBeNil)
			So(result.Value, ShouldEqual, "done")
		})
	})
}
