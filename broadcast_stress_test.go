package qpool

import (
	"context"
	"runtime/debug"
	"sync"
	"testing"
	"time"
)

// TestBroadcastWaitStressUnderGC hammers Send + Wait concurrently while forcing
// frequent GC, reproducing the conditions of the gcAssistAlloc crash. It must
// run clean (no fatal scheduler error) and every consumer must keep receiving.
func TestBroadcastWaitStressUnderGC(t *testing.T) {
	old := debug.SetGCPercent(10) // force frequent GC
	defer debug.SetGCPercent(old)

	pool := NewQ[any](context.Background(), 1, 8, nil)
	bg := pool.CreateBroadcastGroup("stress", time.Second)

	const consumers = 8
	const producers = 4

	ctx, cancel := context.WithCancel(context.Background())

	subs := make([]*BroadcastConsumer, consumers)
	for i := range subs {
		subs[i] = bg.Subscribe("", 256)
	}

	var received [consumers]int
	var wg sync.WaitGroup

	for i := 0; i < consumers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for {
				v, err := subs[idx].Wait(ctx)
				if err != nil {
					return
				}
				if v != nil {
					received[idx]++
					_ = make([]byte, 64) // allocate to provoke GC assist on this g
				}
			}
		}(i)
	}

	var pwg sync.WaitGroup
	for p := 0; p < producers; p++ {
		pwg.Add(1)
		go func() {
			defer pwg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					bg.Send(&QValue[erasedAny]{Value: make([]byte, 32)})
				}
			}
		}()
	}

	time.Sleep(3 * time.Second)
	cancel()
	pwg.Wait()
	wg.Wait()

	total := 0
	for i := 0; i < consumers; i++ {
		if received[i] == 0 {
			t.Fatalf("consumer %d received nothing — wake path broken", i)
		}
		total += received[i]
	}
	t.Logf("stress ok: %d frames delivered across %d consumers under GC", total, consumers)
}
