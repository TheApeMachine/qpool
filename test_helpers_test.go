package qpool

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/datura"
)

func receiveResultWait[T any](test *testing.T, wait *ResultWait[T]) *datura.Artifact {
	test.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	result, err := wait.Get(ctx)

	if err != nil {
		test.Fatalf("timed out waiting for qpool result: %v", err)
	}

	return result
}

func broadcastGroupSubscriberCount(group *BroadcastGroup) int {
	if group == nil {
		return 0
	}

	count := 0

	group.consumers.Range(func(key, value any) bool {
		count++
		return true
	})

	return count
}

