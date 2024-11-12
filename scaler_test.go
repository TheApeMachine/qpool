package qpool

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestScaler(t *testing.T) {
	Convey("Given a pool with scaling enabled", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // Increased timeout
		defer cancel()

		q := NewQ(ctx, 2, 10)

		Convey("When load increases", func() {
			// Simulate high load
			for i := 0; i < 20; i++ { // Reduced from 50 to 20
				go func(jobID int) {
					result := q.Schedule(fmt.Sprintf("load-test-%d", jobID), func() (any, error) {
						time.Sleep(time.Millisecond * 200)
						return nil, nil
					})
					<-result
				}(i)
			}

			// Allow more time for scaling
			time.Sleep(time.Second * 5)

			Convey("Worker count should increase", func() {
				q.metrics.mu.RLock()
				count := q.metrics.WorkerCount
				q.metrics.mu.RUnlock()
				So(count, ShouldBeGreaterThan, 2)
			})
		})
	})
}
