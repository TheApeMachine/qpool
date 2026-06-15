package qpool

import (
	"context"
	"testing"

	"github.com/theapemachine/datura"
)

func BenchmarkQ_Schedule_simpleJob(b *testing.B) {
	ctx := context.Background()

	cfg := NewConfig()
	cfg.Scaler = nil
	cfg.TelemetryPublish = func(*datura.Artifact) error { return nil }

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
			b.Fatal("unexpected nil artifact")
		}

		if artifactErr := ArtifactError(qv); artifactErr != nil {
			b.Fatalf("unexpected schedule error: %v", artifactErr)
		}
	}
}

func BenchmarkQ_Schedule_parallel(b *testing.B) {
	ctx := context.Background()

	cfg := NewConfig()
	cfg.Scaler = nil
	cfg.TelemetryPublish = func(*datura.Artifact) error { return nil }

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
				b.Fatal("unexpected nil artifact")
			}

			if artifactErr := ArtifactError(qv); artifactErr != nil {
				b.Fatalf("unexpected schedule error: %v", artifactErr)
			}
		}
	})
}
