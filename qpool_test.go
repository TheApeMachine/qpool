package qpool

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

const (
	testTimeout    = 10 * time.Second
	channelTimeout = 5 * time.Second
	cleanupTimeout = 500 * time.Millisecond
)

func TestQuantumPool(t *testing.T) {
	Convey("Given a new quantum pool", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		q := NewQ(ctx, 2, 5, &Config{
			SchedulingTimeout: time.Second,
		})

		Reset(func() {
			cancel()
			if q != nil {
				q.Close()
			}
		})

		Convey("When scheduling a simple job", func() {
			done := make(chan struct{})
			resultCh := q.Schedule("test-job", func() (any, error) {
				defer close(done)
				return "success", nil
			})

			select {
			case <-time.After(2 * time.Second):
				t.Fatal("Test timed out waiting for job completion")
			case <-done:
				select {
				case result := <-resultCh:
					So(result.Error, ShouldBeNil)
					So(result.Value, ShouldEqual, "success")
				case <-time.After(time.Second):
					t.Fatal("Test timed out waiting for result")
				}
			}
		})
	})
}
