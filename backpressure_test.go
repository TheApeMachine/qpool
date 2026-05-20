package qpool

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBackPressureObserveLimitRenormalize(t *testing.T) {
	Convey("BackPressure derives pressure from readings", t, func() {
		bp := NewBackPressureRegulator(100, time.Second, time.Minute)

		So(bp.Limit(), ShouldBeFalse)

		bp.Observe(&MetricReading{
			JobQueueSize:      80,
			AverageJobLatency: 2 * time.Second,
		})

		So(math.Abs(bp.GetPressure()-1.0) < 0.02, ShouldBeTrue)

		So(bp.Limit(), ShouldBeTrue)
	})

	Convey("Renormalize bleeds pressure when readings calm down", t, func() {
		bp := NewBackPressureRegulator(100, time.Second, time.Minute)

		bp.Observe(&MetricReading{
			JobQueueSize:      90,
			AverageJobLatency: 2 * time.Second,
		})

		So(bp.Limit(), ShouldBeTrue)

		bp.Observe(&MetricReading{
			JobQueueSize:      10,
			AverageJobLatency: time.Millisecond,
		})

		bp.Renormalize()

		So(bp.GetPressure(), ShouldBeLessThan, 0.85)
	})
}
