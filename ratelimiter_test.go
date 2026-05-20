package qpool

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewRateLimiter(t *testing.T) {
	Convey("Given NewRateLimiter constructor normalization", t, func() {
		cases := []struct {
			name            string
			maxTokens       int
			refill          time.Duration
			wantCapacity    int64
			wantRefill      time.Duration
			wantFirstReject bool
		}{
			{
				name:            "negative capacity clamps to zero and rejects immediately",
				maxTokens:       -3,
				refill:          100 * time.Millisecond,
				wantCapacity:    0,
				wantRefill:      100 * time.Millisecond,
				wantFirstReject: true,
			},
			{
				name:            "zero refill defaults to one second",
				maxTokens:       2,
				refill:          0,
				wantCapacity:    2,
				wantRefill:      time.Second,
				wantFirstReject: false,
			},
			{
				name:            "negative refill defaults to one second",
				maxTokens:       1,
				refill:          -time.Nanosecond,
				wantCapacity:    1,
				wantRefill:      time.Second,
				wantFirstReject: false,
			},
		}

		for _, row := range cases {
			label := row.name

			Convey(fmt.Sprintf("When %s", label), func() {
				limiter := NewRateLimiter(row.maxTokens, row.refill)

				So(limiter.maxTokens, ShouldEqual, row.wantCapacity)
				So(limiter.refillRate, ShouldEqual, row.wantRefill)
				So(limiter.Limit(), ShouldEqual, row.wantFirstReject)
			})
		}
	})
}

func TestRateLimiter_NewRateLimiter_burstAndRefillInterval(t *testing.T) {
	Convey("Given NewRateLimiter with burst 2 and 50ms refill", t, func() {
		rl := NewRateLimiter(2, 50*time.Millisecond)

		Convey("It should use at least the configured refill interval internally", func() {
			So(rl.refillRate, ShouldEqual, 50*time.Millisecond)
			So(rl.maxTokens, ShouldEqual, int64(2))
		})

		Convey("It should allow two immediate Limit acquisitions then reject until refill", func() {
			So(rl.Limit(), ShouldBeFalse)
			So(rl.Limit(), ShouldBeFalse)
			So(rl.Limit(), ShouldBeTrue)

			time.Sleep(250 * time.Millisecond)

			So(rl.Limit(), ShouldBeFalse)
		})
	})
}

func TestRateLimiter_Observe_doesNotRefillTokens(t *testing.T) {
	Convey("Given an exhausted RateLimiter", t, func() {
		rl := NewRateLimiter(1, time.Minute)
		So(rl.Limit(), ShouldBeFalse)
		So(rl.Limit(), ShouldBeTrue)

		Convey("Observe should not add tokens by itself", func() {
			rl.Observe(&MetricReading{TotalJobs: 100, FailedJobs: 2, ThrottledJobs: 1})
			So(rl.Limit(), ShouldBeTrue)
		})
	})
}

func TestRateLimiter_Renormalize_andRefill(t *testing.T) {
	Convey("Given a RateLimiter exhausted by Limit calls", t, func() {
		rl := NewRateLimiter(1, 50*time.Millisecond)
		So(rl.Limit(), ShouldBeFalse)
		So(rl.Limit(), ShouldBeTrue)

		Convey("Renormalize after waiting the refill interval should allow another acquisition", func() {
			time.Sleep(250 * time.Millisecond)
			rl.Renormalize()
			So(rl.Limit(), ShouldBeFalse)
		})
	})
}
