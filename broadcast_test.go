package qpool

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewBroadcaster(test *testing.T) {
	Convey("Given NewBroadcaster", test, func() {
		broadcaster := NewBroadcaster()

		Convey("It should create an empty subscriber set", func() {
			So(broadcaster, ShouldNotBeNil)
			So(broadcaster.subscribers, ShouldNotBeNil)
			So(len(broadcaster.subscribers), ShouldEqual, 0)
		})
	})
}

func TestBroadcaster_Subscribe(test *testing.T) {
	Convey("Given Broadcaster Subscribe", test, func() {
		broadcaster := NewBroadcaster()
		subscription := broadcaster.Subscribe(1)
		defer subscription.Close()

		Convey("It should receive published events", func() {
			event := NewInfoEvent(
				"component",
				"op",
				"message",
				[]Field{{Key: "key", Value: "value"}},
			)

			broadcaster.Publish(event)

			received := receiveBroadcastEvent(subscription)

			So(received.Component, ShouldEqual, "component")
			So(received.Op, ShouldEqual, "op")
			So(received.Message, ShouldEqual, "message")
			So(received.Fields, ShouldResemble, event.Fields)
		})
	})
}

func TestBroadcaster_Publish(test *testing.T) {
	Convey("Given Broadcaster Publish", test, func() {
		broadcaster := NewBroadcaster()
		firstSubscription := broadcaster.Subscribe(1)
		secondSubscription := broadcaster.Subscribe(1)
		defer firstSubscription.Close()
		defer secondSubscription.Close()

		Convey("It should broadcast one event to every subscriber", func() {
			event := NewInfoEvent("component", "op", "message", nil)

			broadcaster.Publish(event)

			So(receiveBroadcastEvent(firstSubscription).Message, ShouldEqual, "message")
			So(receiveBroadcastEvent(secondSubscription).Message, ShouldEqual, "message")
		})

		Convey("It should keep the latest event when a subscriber is full", func() {
			broadcaster.Publish(NewInfoEvent("component", "op", "first", nil))
			broadcaster.Publish(NewInfoEvent("component", "op", "second", nil))

			So(receiveBroadcastEvent(firstSubscription).Message, ShouldEqual, "second")
		})
	})
}

func TestSubscription_Close(test *testing.T) {
	Convey("Given Subscription Close", test, func() {
		broadcaster := NewBroadcaster()
		subscription := broadcaster.Subscribe(1)

		Convey("It should close the event channel and unregister the subscriber", func() {
			subscription.Close()

			_, channelOpen := <-subscription.Events()

			So(channelOpen, ShouldBeFalse)
			So(len(broadcaster.subscribers), ShouldEqual, 0)
		})
	})
}

func BenchmarkBroadcaster_Publish(benchmark *testing.B) {
	broadcaster := NewBroadcaster()
	subscription := broadcaster.Subscribe(64)
	defer subscription.Close()

	event := NewInfoEvent("component", "op", "message", nil)

	for benchmark.Loop() {
		broadcaster.Publish(event)
	}
}

func receiveBroadcastEvent(subscription *Subscription) Event {
	select {
	case event := <-subscription.Events():
		return event
	case <-time.After(time.Second):
		return Event{Message: "timeout"}
	}
}
