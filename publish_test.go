package qpool

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPublish(test *testing.T) {
	Convey("Given Publish", test, func() {
		subscription := Subscribe(1)
		defer subscription.Close()

		Convey("It should broadcast the published event", func() {
			Publish(NewDebugEvent("component", "op", "message", nil))

			event := receiveBroadcastEvent(subscription)

			So(event.Component, ShouldEqual, "component")
			So(event.Op, ShouldEqual, "op")
			So(event.Message, ShouldEqual, "message")
		})
	})
}
