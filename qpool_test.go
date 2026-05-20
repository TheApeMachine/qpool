package qpool

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestQPoolScheduleSimple(t *testing.T) {
	Convey("Given a new Q pool", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		q := NewQ(ctx, 2, 5, &Config{
			SchedulingTimeout: time.Second,
			Scaler:            nil,
		})

		Reset(func() {
			cancel()

			if q != nil {
				q.Close()
			}
		})

		Convey("When scheduling a simple job", func() {
			resultCh := q.Schedule("test-job", func(ctx context.Context) (any, error) {
				return "success", nil
			})

			select {
			case <-ctx.Done():
				So(ctx.Err(), ShouldBeNil)
			case result := <-resultCh:
				So(result, ShouldNotBeNil)
				So(result.Error, ShouldBeNil)
				So(result.Value, ShouldEqual, "success")
			}
		})
	})
}

func TestQPoolTelemetryPublish(t *testing.T) {
	Convey("Given a Q pool with telemetry publisher", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		events := make(chan Event, 8)

		q := NewQ(ctx, 1, 1, &Config{
			SchedulingTimeout: time.Second,
			Scaler:            nil,
			TelemetryPublish: func(event Event) {
				events <- event
			},
		})

		Reset(func() {
			cancel()

			if q != nil {
				q.Close()
			}
		})

		Convey("It should route pool events through the configured publisher", func() {
			resultChannel := q.Schedule("telemetry-job", func(ctx context.Context) (any, error) {
				return "ok", nil
			})

			select {
			case result := <-resultChannel:
				So(result, ShouldNotBeNil)
				So(result.Error, ShouldBeNil)
			case <-time.After(2 * time.Second):
				So("telemetry job", ShouldEqual, "timed out")
			}

			observed := false

			for !observed {
				select {
				case event := <-events:
					observed = event.Component == "qpool"
				case <-time.After(2 * time.Second):
					So("qpool telemetry event", ShouldEqual, "timed out")
					return
				}
			}

			So(observed, ShouldBeTrue)
		})
	})
}

func TestSchedule_circuitBreakerOpenRejectsFurtherSchedules(t *testing.T) {
	Convey("Given Q with circuit breaker jobs", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)

		poolConfig := &Config{
			Scaler:            nil,
			SchedulingTimeout: 5 * time.Second,
		}

		q := NewQ(ctx, 2, 2, poolConfig)

		Reset(func() {
			cancel()

			if q != nil {
				q.Close()
			}
		})

		cases := []struct {
			name           string
			maxFailures    int
			failuresToOpen int
		}{
			{name: "opens after one recorded failure", maxFailures: 1, failuresToOpen: 1},
			{name: "opens after two recorded failures", maxFailures: 2, failuresToOpen: 2},
		}

		for _, row := range cases {
			label := row.name

			Convey(fmt.Sprintf("When %s", label), func() {
				circuitID := fmt.Sprintf("cb-%d-%s", row.maxFailures, label)

				for failureIndex := range row.failuresToOpen {
					jobID := fmt.Sprintf("%s-fail-%d", circuitID, failureIndex)

					resultChannel := q.Schedule(jobID, func(jobCtx context.Context) (any, error) {
						return nil, errors.New("intentional failure")
					}, WithCircuitBreaker(circuitID, row.maxFailures, time.Minute))

					select {
					case result := <-resultChannel:
						So(result, ShouldNotBeNil)
						So(result.Error, ShouldNotBeNil)
					case <-time.After(6 * time.Second):
						So("failed job completion", ShouldEqual, "timed out")
					}
				}

				blockedChannel := q.Schedule(fmt.Sprintf("%s-blocked", circuitID), func(jobCtx context.Context) (any, error) {
					return "skipped", nil
				}, WithCircuitBreaker(circuitID, row.maxFailures, time.Minute))

				select {
				case result := <-blockedChannel:
					So(result, ShouldNotBeNil)
					So(result.Error, ShouldNotBeNil)
					So(result.Error.Error(), ShouldContainSubstring, "circuit breaker")
					So(result.Error.Error(), ShouldContainSubstring, circuitID)
					So(result.Error.Error(), ShouldContainSubstring, "open")
				case <-time.After(time.Second):
					So("circuit rejection", ShouldEqual, "timed out")
				}
			})
		}
	})
}
