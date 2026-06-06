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

	q := NewQ[any](ctx, 4, 8, cfg)

	defer q.Close()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		wait := q.Schedule("bench", func(ctx context.Context) (any, error) {
			return 1, nil
		})

		qv, err := wait.Get(ctx)

		if err != nil {
			b.Fatalf("unexpected wait error: %v", err)
		}

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

	q := NewQ[any](ctx, 4, 8, cfg)

	defer q.Close()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			wait := q.Schedule("bench", func(ctx context.Context) (any, error) {
				return 1, nil
			})

			qv, err := wait.Get(ctx)

			if err != nil {
				b.Fatalf("unexpected wait error: %v", err)
			}

			if qv == nil {
				b.Fatal("unexpected nil QValue")
			}

			if qv.Error != nil {
				b.Fatalf("unexpected schedule error: %v", qv.Error)
			}
		}
	})
}

func BenchmarkQ_ScheduleFast_simpleJob(b *testing.B) {
	ctx := context.Background()

	cfg := NewConfig()
	cfg.Scaler = nil
	cfg.TelemetryPublish = func(Event) {}

	q := NewQ[any](ctx, 4, 8, cfg)

	defer q.Close()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		wait := q.ScheduleFast(ctx, func(ctx context.Context) (any, error) {
			return 1, nil
		})

		qv, err := wait.Get(ctx)

		if err != nil {
			b.Fatalf("unexpected wait error: %v", err)
		}

		if qv == nil {
			b.Fatal("unexpected nil QValue")
		}

		if qv.Error != nil {
			b.Fatalf("unexpected schedule error: %v", qv.Error)
		}
	}
}

func BenchmarkQ_ScheduleFast_parallel(b *testing.B) {
	ctx := context.Background()

	cfg := NewConfig()
	cfg.Scaler = nil
	cfg.TelemetryPublish = func(Event) {}

	q := NewQ[any](ctx, 4, 8, cfg)

	defer q.Close()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			wait := q.ScheduleFast(ctx, func(ctx context.Context) (any, error) {
				return 1, nil
			})

			qv, err := wait.Get(ctx)

			if err != nil {
				b.Fatalf("unexpected wait error: %v", err)
			}

			if qv == nil {
				b.Fatal("unexpected nil QValue")
			}

			if qv.Error != nil {
				b.Fatalf("unexpected schedule error: %v", qv.Error)
			}
		}
	})
}
