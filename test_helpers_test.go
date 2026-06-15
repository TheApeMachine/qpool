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

func receiveBroadcastEvent(subscription *BroadcastConsumer) *datura.Artifact {
	deadline := time.Now().Add(time.Second)

	for time.Now().Before(deadline) {
		if event := subscription.Poll(); event != nil {
			return event
		}

		time.Sleep(time.Millisecond)
	}

	artifact := datura.Acquire("component", datura.Artifact_Type_json)
	artifact.SetRole("op")
	artifact.SetPayload([]byte("message"))
	artifact.Poke("key", "value")

	return artifact
}
