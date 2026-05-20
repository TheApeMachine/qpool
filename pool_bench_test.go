package qpool

import (
	"context"
	"testing"
)

func BenchmarkQ_Schedule_simpleJob(b *testing.B) {
	ctx := context.Background()

	cfg := NewConfig()
	cfg.Scaler = nil
	cfg.TelemetryPublish = func(Event) {}

	q := NewQ(ctx, 4, 8, cfg)

	defer q.Close()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		ch := q.Schedule("bench", func(ctx context.Context) (any, error) {
			return 1, nil
		})

		qv := <-ch

		if qv == nil {
			b.Fatal("unexpected nil QValue")
		}

		if qv.Error != nil {
			b.Fatalf("unexpected schedule error: %v", qv.Error)
		}
	}
}

func BenchmarkQ_Schedule_parallel(b *testing.B) {
	ctx := context.Background()

	cfg := NewConfig()
	cfg.Scaler = nil
	cfg.TelemetryPublish = func(Event) {}

	q := NewQ(ctx, 4, 8, cfg)

	defer q.Close()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ch := q.Schedule("bench", func(ctx context.Context) (any, error) {
				return 1, nil
			})

			qv := <-ch

			if qv == nil {
				b.Fatal("unexpected nil QValue")
			}

			if qv.Error != nil {
				b.Fatalf("unexpected schedule error: %v", qv.Error)
			}
		}
	})
}
