package qpool

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestResultWaitCtxCancelRaceUnderGC reproduces the conditions of the
// `casgstatus: waiting for Gwaiting but is Grunnable` crash: many goroutines
// hammer Schedule().Get(ctx) with deadlines tight enough that ctx cancellation
// frequently races job delivery (the double-wake the old fast_park/safe_ready
// path turned into a fatal double-goready), all under forced GC. With the
// per-waiter semaphore it must run clean.
func TestResultWaitCtxCancelRaceUnderGC(t *testing.T) {
	old := debug.SetGCPercent(10)
	defer debug.SetGCPercent(old)

	pool := NewQ[any](context.Background(), 4, 16, nil)

	const workers = 16
	stop := time.Now().Add(3 * time.Second)

	var wg sync.WaitGroup
	var ok, timeouts int64

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			i := 0
			for time.Now().Before(stop) {
				i++
				n := i // capture per-iteration; the job runs async on a worker
				rw := pool.Schedule(fmt.Sprintf("job-%d-%d", seed, n), func(ctx context.Context) (any, error) {
					_ = make([]byte, 128) // allocate to provoke GC assist on the delivering g
					return n, nil
				})
				// Tight, varying deadline so ctx cancellation frequently races delivery.
				d := time.Duration((n%5)*40) * time.Microsecond
				ctx, cancel := context.WithTimeout(context.Background(), d)
				_, err := rw.Get(ctx)
				cancel()
				if err != nil {
					atomic.AddInt64(&timeouts, 1)
				} else {
					atomic.AddInt64(&ok, 1)
				}
			}
		}(w)
	}

	wg.Wait()
	t.Logf("ctx-race stress: completed=%d timedout=%d (no fatal scheduler crash)", ok, timeouts)
	if ok == 0 {
		t.Fatal("no jobs completed — scheduler wedged")
	}
}
