package qpool

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func TestQPoolScheduleSimple(t *testing.T) {
	Convey("Given a new Q pool", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		q := NewQ[any](ctx, 2, 5, &Config{
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
			wait := q.Schedule("test-job", func(ctx context.Context) (any, error) {
				return "success", nil
			})

			result, err := wait.Get(ctx)

			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(ArtifactError(result), ShouldBeNil)

			value, valueErr := ArtifactValue[string](result)

			So(valueErr, ShouldBeNil)
			So(value, ShouldEqual, "success")
		})
	})
}

func TestQPoolTelemetryPublish(t *testing.T) {
	Convey("Given a Q pool with telemetry publisher", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		events := make(chan *datura.Artifact, 8)

		q := NewQ[any](ctx, 1, 1, &Config{
			SchedulingTimeout: time.Second,
			Scaler:            nil,
			TelemetryPublish: func(artifact *datura.Artifact) error {
				events <- artifact

				return nil
			},
		})

		Reset(func() {
			cancel()

			if q != nil {
				q.Close()
			}
		})

		Convey("It should route pool events through the configured publisher", func() {
			wait := q.Schedule("telemetry-job", func(ctx context.Context) (any, error) {
				return "ok", nil
			})

			result, err := wait.Get(context.Background())

			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(ArtifactError(result), ShouldBeNil)

			observed := false

			deadline := time.Now().Add(2 * time.Second)

			for !observed && time.Now().Before(deadline) {
				select {
				case event := <-events:
					origin, originErr := event.Origin()

					So(originErr, ShouldBeNil)
					So(origin, ShouldEqual, "qpool")

					observed = true
				default:
					time.Sleep(time.Millisecond)
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

		q := NewQ[any](ctx, 2, 2, poolConfig)

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

					wait := q.Schedule(jobID, func(jobCtx context.Context) (any, error) {
						return nil, errors.New("intentional failure")
					}, WithCircuitBreaker(circuitID, row.maxFailures, time.Minute))

					result, err := wait.Get(context.Background())

					So(err, ShouldBeNil)
					So(result, ShouldNotBeNil)
					So(ArtifactError(result), ShouldNotBeNil)
				}

				blockedWait := q.Schedule(fmt.Sprintf("%s-blocked", circuitID), func(jobCtx context.Context) (any, error) {
					return "skipped", nil
				}, WithCircuitBreaker(circuitID, row.maxFailures, time.Minute))

				result, err := blockedWait.Get(context.Background())

				So(err, ShouldBeNil)
				So(result, ShouldNotBeNil)
				So(ArtifactError(result), ShouldNotBeNil)
				So(ArtifactError(result).Error(), ShouldContainSubstring, "circuit breaker")
				So(ArtifactError(result).Error(), ShouldContainSubstring, circuitID)
				So(ArtifactError(result).Error(), ShouldContainSubstring, "open")
			})
		}
	})
}
