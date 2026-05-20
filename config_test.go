package qpool

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewConfig(t *testing.T) {
	Convey("Given NewConfig", t, func() {
		cfg := NewConfig()

		cases := []struct {
			name  string
			check func() bool
		}{
			{
				name: "SchedulingTimeout is ten seconds",
				check: func() bool {
					return cfg.SchedulingTimeout == 10*time.Second
				},
			},
			{
				name: "Scaler is non-nil by default",
				check: func() bool {
					return cfg.Scaler != nil
				},
			},
			{
				name: "Scaler TargetLoad is two",
				check: func() bool {
					return cfg.Scaler.TargetLoad == 2.0
				},
			},
			{
				name: "Scaler ScaleUpThreshold is four",
				check: func() bool {
					return cfg.Scaler.ScaleUpThreshold == 4.0
				},
			},
			{
				name: "Scaler ScaleDownThreshold is one",
				check: func() bool {
					return cfg.Scaler.ScaleDownThreshold == 1.0
				},
			},
			{
				name: "Scaler Cooldown is five hundred milliseconds",
				check: func() bool {
					return cfg.Scaler.Cooldown == 500*time.Millisecond
				},
			},
			{
				name: "Scaler Interval is one second",
				check: func() bool {
					return cfg.Scaler.Interval == time.Second
				},
			},
			{
				name: "Regulators slice is nil",
				check: func() bool {
					return cfg.Regulators == nil
				},
			},
			{
				name: "JobChannelCapacity is zero",
				check: func() bool {
					return cfg.JobChannelCapacity == 0
				},
			},
			{
				name: "CircuitBreakerLimit is default",
				check: func() bool {
					return cfg.CircuitBreakerLimit == defaultCircuitBreakerLimit
				},
			},
			{
				name: "TelemetryPublish is nil",
				check: func() bool {
					return cfg.TelemetryPublish == nil
				},
			},
		}

		for _, row := range cases {
			label := row.name

			Convey(fmt.Sprintf("It should satisfy %s", label), func() {
				So(row.check(), ShouldBeTrue)
			})
		}
	})
}

func BenchmarkNewConfig(b *testing.B) {
	for range b.N {
		_ = NewConfig()
	}
}
