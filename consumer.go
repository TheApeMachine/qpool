package qpool

import (
	"context"
	"sync/atomic"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

/*
BroadcastConsumer receives values from a
broadcast group without channels.
*/
type BroadcastConsumer struct {
	ring     *SpscArtifactRing
	callback func(*datura.Artifact) error
	sema     uint32
	wantWake atomic.Bool
}

/*
NewBroadcastConsumer creates a new broadcast consumer with a callback.
*/
func NewBroadcastConsumer(
	ring *SpscArtifactRing, callback func(*datura.Artifact) error,
) *BroadcastConsumer {
	return &BroadcastConsumer{
		ring:     ring,
		callback: callback,
		sema:     uint32(0),
		wantWake: atomic.Bool{},
	}
}

/*
wake releases the consumer if it is blocked in Wait.
It uses the runtime semaphore (the same GC-safe primitive
sync.Mutex uses) rather than manual goroutine-state manipulation.
The steady-state fast path is a single uncontended atomic load
(false when the consumer is keeping up), so it adds no measurable
cost to Send/Push. The CAS gates exactly one Semrelease per arm, so
the semaphore count never drifts.
*/
func (consumer *BroadcastConsumer) wake() {
	if !consumer.wantWake.Load() {
		return
	}

	if consumer.wantWake.CompareAndSwap(true, false) {
		runtime_Semrelease(&consumer.sema, false, 0)
	}
}

/*
park blocks the single consumer goroutine until a producer pushes
or ctx is canceled. It arms wantWake, re-checks the ring to close
the lost-wakeup window, then blocks on the runtime semaphore; the
producer's wake (or ctx watcher) releases it. This runs only on
the idle path (empty ring), it costs nothing while data is flowing.
*/
func (consumer *BroadcastConsumer) park(ctx context.Context) {
	consumer.wantWake.Store(true)

	// A producer may have pushed (and tried to wake) between Wait's
	// Pop and the arm above. If we reclaim our own arm, return so
	// Wait's next Pop takes the value. If a producer already claimed
	// it, a Semrelease is in flight, so absorb it with exactly one
	// acquire to keep the count balanced.
	if !consumer.ring.Empty() || ctx.Err() != nil {
		if consumer.wantWake.CompareAndSwap(true, false) {
			return
		}

		runtime_Semacquire(&consumer.sema)
		return
	}

	var stopAfterFunc func() bool

	if ctx.Done() != nil {
		stopAfterFunc = context.AfterFunc(ctx, func() {
			consumer.wake()
		})
	}

	runtime_Semacquire(&consumer.sema)

	if stopAfterFunc != nil {
		stopAfterFunc()
	}
}

/*
Poll returns the next queued value when one is available.
*/
func (consumer *BroadcastConsumer) Poll() *datura.Artifact {
	if consumer == nil || consumer.ring == nil {
		return nil
	}

	return consumer.ring.Pop()
}

/*
Wait blocks until a value is available or ctx is canceled.
*/
func (consumer *BroadcastConsumer) Wait(
	ctx context.Context,
) (*datura.Artifact, error) {
	if consumer == nil || consumer.ring == nil {
		return nil, errnie.Err(
			errnie.Conflict,
			"result closed",
			nil,
		)
	}

	for {
		if value := consumer.ring.Pop(); value != nil {
			return value, nil
		}

		if err := ctx.Err(); err != nil {
			return nil, err
		}

		consumer.park(ctx)
	}
}
