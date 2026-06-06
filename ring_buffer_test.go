package qpool

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRingBuffer_NewRingBuffer(test *testing.T) {
	Convey("Given NewRingBuffer", test, func() {
		Convey("It should round capacity up to a power of two", func() {
			buffer := NewRingBuffer[int](3)

			So(buffer.Capacity(), ShouldEqual, 4)
		})
	})
}

func TestRingBuffer_nextPowerOfTwo(test *testing.T) {
	Convey("Given RingBuffer nextPowerOfTwo", test, func() {
		buffer := RingBuffer[int]{}

		Convey("It should return 1 for non-positive inputs", func() {
			So(buffer.next(0), ShouldEqual, 1)
			So(buffer.next(-4), ShouldEqual, 1)
		})

		Convey("It should round up to the next power of two", func() {
			So(buffer.next(1), ShouldEqual, 1)
			So(buffer.next(2), ShouldEqual, 2)
			So(buffer.next(3), ShouldEqual, 4)
			So(buffer.next(5), ShouldEqual, 8)
		})
	})
}

func TestRingBuffer_Slot(test *testing.T) {
	Convey("Given RingBuffer Slot", test, func() {
		buffer := NewRingBuffer[int](4)

		Convey("It should wrap sequence numbers with the capacity mask", func() {
			*buffer.Slot(0) = 10
			*buffer.Slot(1) = 20

			So(*buffer.Slot(0), ShouldEqual, 10)
			So(*buffer.Slot(1), ShouldEqual, 20)
			So(*buffer.Slot(4), ShouldEqual, 10)
			So(buffer.Capacity(), ShouldEqual, 4)
		})
	})
}

func BenchmarkRingBuffer_Slot(benchmark *testing.B) {
	buffer := NewRingBuffer[int](1024)

	for benchmark.Loop() {
		*buffer.Slot(int64(benchmark.N)) = benchmark.N
	}
}
