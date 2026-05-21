package qpool

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestQScheduleFastExecutesJob(test *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pool := NewQ(ctx, 1, 2, &Config{Scaler: nil})
	defer pool.Close()

	resultChannel := pool.ScheduleFast(ctx, func(jobCtx context.Context) (any, error) {
		if err := jobCtx.Err(); err != nil {
			return nil, err
		}

		return "fast", nil
	})

	select {
	case result := <-resultChannel:
		if result == nil {
			test.Fatal("expected result, got nil")
		}

		if result.Error != nil {
			test.Fatalf("expected nil error, got %v", result.Error)
		}

		if result.Value != "fast" {
			test.Fatalf("expected fast result, got %v", result.Value)
		}
	case <-time.After(time.Second):
		test.Fatal("timed out waiting for fast result")
	}
}

func TestQScheduleFastReturnsError(test *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pool := NewQ(ctx, 1, 2, &Config{Scaler: nil})
	defer pool.Close()

	expected := errors.New("fast failure")
	resultChannel := pool.ScheduleFast(ctx, func(context.Context) (any, error) {
		return nil, expected
	})

	select {
	case result := <-resultChannel:
		if result == nil {
			test.Fatal("expected result, got nil")
		}

		if !errors.Is(result.Error, expected) {
			test.Fatalf("expected %v, got %v", expected, result.Error)
		}
	case <-time.After(time.Second):
		test.Fatal("timed out waiting for fast error")
	}
}

func TestQScheduleFastRejectsClosedPool(test *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := NewQ(ctx, 1, 2, &Config{Scaler: nil})
	pool.Close()

	resultChannel := pool.ScheduleFast(ctx, func(context.Context) (any, error) {
		return "unexpected", nil
	})

	select {
	case result := <-resultChannel:
		if result == nil {
			test.Fatal("expected closed-pool error, got nil result")
		}

		if result.Error == nil {
			test.Fatal("expected closed-pool error, got nil")
		}
	case <-time.After(time.Second):
		test.Fatal("timed out waiting for closed-pool error")
	}
}

func TestQScheduleFastRespectsPreCanceledContext(test *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pool := NewQ(ctx, 1, 2, &Config{Scaler: nil})
	defer pool.Close()

	jobCtx, cancelJob := context.WithCancel(ctx)
	cancelJob()

	resultChannel := pool.ScheduleFast(jobCtx, func(context.Context) (any, error) {
		return "unexpected", nil
	})

	select {
	case result := <-resultChannel:
		if result == nil {
			test.Fatal("expected canceled-context error, got nil result")
		}

		if !errors.Is(result.Error, context.Canceled) {
			test.Fatalf("expected context.Canceled, got %v", result.Error)
		}
	case <-time.After(time.Second):
		test.Fatal("timed out waiting for canceled-context error")
	}
}
