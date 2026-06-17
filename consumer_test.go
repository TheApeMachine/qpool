package qpool

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func TestBroadcastConsumerPoll(test *testing.T) {
	Convey("Given a broadcast consumer backed by a real SpscArtifactRing", test, func() {
		ring := NewSPSCRing[datura.Artifact](128, true)
		consumer := NewBroadcastConsumer(ring, nil)
		artifact := testArtifact("poll-payload")

		Convey("When the ring holds a value", func() {
			So(ring.Push(artifact), ShouldBeTrue)

			Convey("Poll should return the queued artifact", func() {
				So(consumer.Poll(), ShouldEqual, artifact)
				So(consumer.Poll(), ShouldBeNil)
			})
		})
	})
}

func TestBroadcastConsumerWait(test *testing.T) {
	Convey("Given a broadcast consumer backed by a real SpscArtifactRing", test, func() {
		ring := NewSPSCRing[datura.Artifact](128, true)
		consumer := NewBroadcastConsumer(ring, nil)
		artifact := testArtifact("wait-payload")

		Convey("When the ring already holds a value", func() {
			So(ring.Push(artifact), ShouldBeTrue)

			Convey("Wait should return the artifact without blocking", func() {
				received, err := consumer.Wait(context.Background())

				So(err, ShouldBeNil)
				So(received, ShouldEqual, artifact)
			})
		})
	})
}

func TestBroadcastConsumerParkWake(test *testing.T) {
	Convey("Given a blocked broadcast consumer on an empty ring", test, func() {
		ring := NewSPSCRing[datura.Artifact](128, true)
		consumer := NewBroadcastConsumer(ring, nil)
		artifact := testArtifact("park-wake-payload")
		waitDone := make(chan *datura.Artifact, 1)

		go func() {
			received, err := consumer.Wait(context.Background())

			if err != nil {
				waitDone <- nil
				return
			}

			waitDone <- received
		}()

		time.Sleep(10 * time.Millisecond)

		So(ring.Push(artifact), ShouldBeTrue)
		consumer.wake()

		Convey("When a producer pushes while the consumer waits", func() {
			select {
			case received := <-waitDone:
				So(received, ShouldEqual, artifact)
			case <-time.After(time.Second):
				test.Fatal("timed out waiting for park/wake delivery")
			}
		})
	})
}

func TestBroadcastConsumerWaitContextCancel(test *testing.T) {
	Convey("Given a broadcast consumer waiting on an empty ring", test, func() {
		ring := NewSPSCRing[datura.Artifact](128, true)
		consumer := NewBroadcastConsumer(ring, nil)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)

		defer cancel()

		Convey("When the context is canceled during Wait", func() {
			_, err := consumer.Wait(ctx)

			So(err, ShouldNotBeNil)
			So(err, ShouldEqual, context.DeadlineExceeded)
		})
	})
}

func TestBroadcastConsumerNilPaths(test *testing.T) {
	Convey("Given nil broadcast consumer or ring handles", test, func() {
		ctx := context.Background()

		Convey("When Wait is called on a nil consumer", func() {
			var consumer *BroadcastConsumer

			_, err := consumer.Wait(ctx)

			So(err, ShouldNotBeNil)
		})

		Convey("When Wait is called on a consumer with a nil ring", func() {
			consumer := NewBroadcastConsumer(nil, nil)

			_, err := consumer.Wait(ctx)

			So(err, ShouldNotBeNil)
		})

		Convey("When Poll is called on a nil consumer", func() {
			var consumer *BroadcastConsumer

			So(consumer.Poll(), ShouldBeNil)
		})

		Convey("When Poll is called on a consumer with a nil ring", func() {
			consumer := NewBroadcastConsumer(nil, nil)

			So(consumer.Poll(), ShouldBeNil)
		})
	})
}
