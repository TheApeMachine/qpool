package qpool

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMetrics_BusyWorkersDuringJob(t *testing.T) {
	Convey("BusyWorkers tracks jobs in processJob", t, func() {
		ctx := context.Background()
		q := NewQ(ctx, 1, 2, &Config{Scaler: nil})

		defer q.Close()

		started := make(chan struct{})
		release := make(chan struct{})

		_ = q.Schedule("block", func(ctx context.Context) (any, error) {
			close(started)
			<-release

			return nil, nil
		})

		<-started

		read := q.MetricSnapshot()

		So(read.WorkerCount, ShouldEqual, 1)
		So(read.BusyWorkers, ShouldEqual, 1)
		So(read.JobQueueSize, ShouldEqual, 0)

		close(release)

		deadline := time.Now().Add(2 * time.Second)

		for time.Now().Before(deadline) {
			if q.MetricSnapshot().BusyWorkers == 0 {
				break
			}

			time.Sleep(5 * time.Millisecond)
		}

		So(q.MetricSnapshot().BusyWorkers, ShouldEqual, 0)
	})
}
