package qpool

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func TestResultSlotWait(test *testing.T) {
	Convey("Given a pending result slot", test, func() {
		slot := newResultSlot()
		waitDone := make(chan *datura.Artifact, 1)

		go func() {
			value, err := slot.Wait(context.Background())
			if err != nil {
				waitDone <- nil

				return
			}

			waitDone <- value
		}()

		Convey("It should deliver after Store races ahead of the waiter", func() {
			delivered, err := newResultArtifact("job", "ok", 0)

			So(err, ShouldBeNil)

			slot.Deliver(delivered)

			select {
			case result := <-waitDone:
				So(result, ShouldNotBeNil)

				payload := result.DecryptPayload()
				So(string(payload), ShouldEqual, "ok")
			case <-time.After(time.Second):
				test.Fatal("timed out waiting for result slot delivery")
			}
		})
	})
}

func TestResultSlotWaitRespectsContextCancel(test *testing.T) {
	Convey("Given a pending result slot", test, func() {
		slot := newResultSlot()
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)

		defer cancel()

		Convey("It should return when the context expires", func() {
			_, err := slot.Wait(ctx)

			So(err, ShouldNotBeNil)
			So(err, ShouldEqual, context.DeadlineExceeded)
		})
	})
}
