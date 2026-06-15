package qpool

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func testArtifact(payload string) *datura.Artifact {
	artifact, err := newResultArtifact("test", payload, 0)

	if err != nil {
		panic(err)
	}

	return artifact
}

func TestSPSCRing_NewSPSCRing(test *testing.T) {
	Convey("Given NewSPSCRing", test, func() {
		ring := NewSPSCRing[datura.Artifact](1, true)

		Convey("It should create a single-slot ring when dropping oldest", func() {
			So(ring, ShouldNotBeNil)
			So(len(ring.slots), ShouldEqual, 1)
		})
	})
}

func TestSPSCRing_Push(test *testing.T) {
	Convey("Given SPSCRing Push", test, func() {
		ring := NewSPSCRing[datura.Artifact](1, true)
		first := testArtifact("first")
		second := testArtifact("second")

		Convey("It should drop the oldest value when the ring is full", func() {
			So(ring.Push(first), ShouldBeTrue)
			So(ring.Push(second), ShouldBeTrue)

			popped := ring.Pop()

			So(popped, ShouldEqual, second)
		})
	})
}

func TestSPSCRing_Pop(test *testing.T) {
	Convey("Given SPSCRing Pop", test, func() {
		ring := NewSPSCRing[datura.Artifact](4, false)
		value := testArtifact("payload")

		Convey("It should return nil when the ring is empty", func() {
			So(ring.Pop(), ShouldBeNil)
		})

		Convey("It should return a pushed value", func() {
			So(ring.Push(value), ShouldBeTrue)

			So(ring.Pop(), ShouldEqual, value)
			So(ring.Pop(), ShouldBeNil)
		})

		Convey("It should return values in FIFO order", func() {
			first := testArtifact("first")
			second := testArtifact("second")

			So(ring.Push(first), ShouldBeTrue)
			So(ring.Push(second), ShouldBeTrue)

			So(ring.Pop(), ShouldEqual, first)
			So(ring.Pop(), ShouldEqual, second)
			So(ring.Pop(), ShouldBeNil)
		})
	})
}

func TestSPSCRing_Close(test *testing.T) {
	Convey("Given SPSCRing Close", test, func() {
		ring := NewSPSCRing[datura.Artifact](4, false)

		Convey("It should drain all queued values", func() {
			_ = ring.Push(testArtifact("one"))
			_ = ring.Push(testArtifact("two"))

			ring.Close()

			So(ring.Pop(), ShouldBeNil)
		})
	})
}

func BenchmarkSPSCRing_PushPop(benchmark *testing.B) {
	ring := NewSPSCRing[datura.Artifact](64, false)
	value := testArtifact("payload")

	for benchmark.Loop() {
		_ = ring.Push(value)
		_ = ring.Pop()
	}
}
