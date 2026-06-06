package qpool

import (
	"context"
	"testing"
	"time"
)

func receiveResultWait[T any](test *testing.T, wait *ResultWait[T]) *QValue[T] {
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

	return group.subscribers.count()
}

func receiveBroadcastEvent(subscription *Subscription) Event {
	deadline := time.Now().Add(time.Second)

	for time.Now().Before(deadline) {
		if event, ok := subscription.Poll(); ok {
			return event
		}

		time.Sleep(time.Millisecond)
	}

	return Event{Message: "timeout"}
}
