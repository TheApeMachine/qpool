package qpool

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLogController_Suppress(test *testing.T) {
	Convey("Given LogController Suppress", test, func() {
		controller := &LogController{}

		Convey("It should suppress logging until restored", func() {
			restore := controller.Suppress()

			So(controller.Suppressed(), ShouldBeTrue)

			restore()

			So(controller.Suppressed(), ShouldBeFalse)
		})

		Convey("It should restore exactly once", func() {
			restore := controller.Suppress()

			restore()
			restore()

			So(controller.Suppressed(), ShouldBeFalse)
		})

		Convey("It should preserve nested suppression", func() {
			restoreOuter := controller.Suppress()
			restoreInner := controller.Suppress()

			restoreInner()

			So(controller.Suppressed(), ShouldBeTrue)

			restoreOuter()

			So(controller.Suppressed(), ShouldBeFalse)
		})
	})
}

func BenchmarkLogController_Suppressed(benchmark *testing.B) {
	controller := &LogController{}
	restore := controller.Suppress()
	defer restore()

	for benchmark.Loop() {
		_ = controller.Suppressed()
	}
}
