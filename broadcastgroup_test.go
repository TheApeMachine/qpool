package qpool

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func testBroadcastArtifact(payload string) *datura.Artifact {
	artifact := datura.Acquire("broadcast-test", datura.Artifact_Type_json)

	if err := artifact.SetDestination("broadcast-test"); err != nil {
		panic(err)
	}

	return artifact.WithPayload([]byte(payload))
}

func TestBroadcastGroupSendRingPush(test *testing.T) {
	Convey("Given a broadcast group with two callback-free subscribers", test, func() {
		ctx := context.Background()
		group := NewBroadcastGroup(ctx, "fanout", time.Minute)
		first := group.Acquire("subscriber-a", nil)
		second := group.Acquire("subscriber-b", nil)

		So(first, ShouldNotBeNil)
		So(second, ShouldNotBeNil)

		artifact := testBroadcastArtifact("fanout-payload")

		Convey("When Send delivers an artifact", func() {
			err := group.Send(artifact)

			So(err, ShouldBeNil)
			So(first.Poll(), ShouldEqual, artifact)
			So(second.Poll(), ShouldEqual, artifact)
		})
	})
}

func TestBroadcastGroupRelease(test *testing.T) {
	Convey("Given a registered broadcast subscriber", test, func() {
		ctx := context.Background()
		group := NewBroadcastGroup(ctx, "release-test", time.Minute)

		So(group.Acquire("subscriber-a", nil), ShouldNotBeNil)
		So(broadcastGroupSubscriberCount(group), ShouldEqual, 1)

		Convey("When the subscriber is released", func() {
			err := group.Release("subscriber-a")

			So(err, ShouldBeNil)
			So(broadcastGroupSubscriberCount(group), ShouldEqual, 0)

			Convey("Poll should report the subscriber as missing", func() {
				_, ok := group.Poll("subscriber-a")

				So(ok, ShouldBeFalse)
			})
		})
	})
}

func TestBroadcastGroupSendNilArtifact(test *testing.T) {
	Convey("Given a broadcast group", test, func() {
		ctx := context.Background()
		group := NewBroadcastGroup(ctx, "nil-artifact", time.Minute)

		Convey("When Send receives a nil artifact", func() {
			err := group.Send(nil)

			So(err, ShouldNotBeNil)
		})
	})
}
