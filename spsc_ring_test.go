package qpool

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSPSCRing_NewSPSCRing(test *testing.T) {
	Convey("Given NewSPSCRing", test, func() {
		ring := NewSPSCRing[QValue[erasedAny]](1, true)

		Convey("It should create a single-slot ring when dropping oldest", func() {
			So(ring, ShouldNotBeNil)
			So(len(ring.slots), ShouldEqual, 1)
		})
	})
}

func TestSPSCRing_normalizeCapacity(test *testing.T) {
	Convey("Given SPSCRing normalizeCapacity", test, func() {
		Convey("It should allow single-slot rings when dropping oldest", func() {
			So(normalizeSPSCCapacity(1, true), ShouldEqual, 1)
		})

		Convey("It should enforce at least two slots when rejecting on full", func() {
			So(normalizeSPSCCapacity(1, false), ShouldEqual, 2)
		})
	})
}

func TestSPSCRing_Push(test *testing.T) {
	Convey("Given SPSCRing Push", test, func() {
		ring := NewSPSCRing[QValue[erasedAny]](1, true)
		first := &QValue[erasedAny]{Value: "first"}
		second := &QValue[erasedAny]{Value: "second"}

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
		ring := NewSPSCRing[QValue[erasedAny]](4, false)
		value := &QValue[erasedAny]{Value: "payload"}

		Convey("It should return nil when the ring is empty", func() {
			So(ring.Pop(), ShouldBeNil)
		})

		Convey("It should return a pushed value", func() {
			So(ring.Push(value), ShouldBeTrue)

			So(ring.Pop(), ShouldEqual, value)
			So(ring.Pop(), ShouldBeNil)
		})

		Convey("It should return values in FIFO order", func() {
			first := &QValue[erasedAny]{Value: "first"}
			second := &QValue[erasedAny]{Value: "second"}

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
		ring := NewSPSCRing[QValue[erasedAny]](4, false)

		Convey("It should drain all queued values", func() {
			_ = ring.Push(&QValue[erasedAny]{Value: "one"})
			_ = ring.Push(&QValue[erasedAny]{Value: "two"})

			ring.Close()

			So(ring.Pop(), ShouldBeNil)
		})
	})
}

func BenchmarkSPSCRing_PushPop(benchmark *testing.B) {
	ring := NewSPSCRing[QValue[erasedAny]](64, false)
	value := &QValue[erasedAny]{Value: "payload"}

	for benchmark.Loop() {
		_ = ring.Push(value)
		_ = ring.Pop()
	}
}
