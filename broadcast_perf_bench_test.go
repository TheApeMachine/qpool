package qpool

import (
	"context"
	"testing"
)

// BenchmarkBroadcastSendNoWaiter measures the producer hot path: Send to a
// subscriber that is NOT parked (the steady-state case when the consumer keeps
// up). This is the exact path where the park/wake fix could add cost, so it is
// the benchmark that must show no regression.
func BenchmarkBroadcastSendNoWaiter(b *testing.B) {
	pool := NewQ[any](context.Background(), 1, 4, nil)
	bg := pool.CreateBroadcastGroup("bench-no-waiter", 0)
	// dropOldestOnFull via NewBroadcaster semantics is not set here; use a large
	// buffer and drain so Push never reports full.
	consumer := bg.Subscribe("c", 1<<16)

	qv := &QValue[erasedAny]{Value: 1}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bg.Send(qv)
		// Drain to keep the ring from filling without involving park/wake.
		consumer.Poll()
	}
}

// BenchmarkBroadcastSendDrained measures realistic throughput: one producer
// Sending while a separate goroutine drains via Poll (no parking).
func BenchmarkBroadcastSendDrained(b *testing.B) {
	pool := NewQ[any](context.Background(), 1, 4, nil)
	bg := pool.CreateBroadcastGroup("bench-drained", 0)
	consumer := bg.Subscribe("c", 1<<16)

	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				consumer.Poll()
			}
		}
	}()

	qv := &QValue[erasedAny]{Value: 1}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bg.Send(qv)
	}
	b.StopTimer()
	close(done)
}
