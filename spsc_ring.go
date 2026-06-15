package qpool

import (
	"sync/atomic"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

/*
SPSCRing is a single-producer single-consumer queue backed by atomic slots.
Broadcast subscribers poll values from one of these rings.
*/
type SPSCRing[T any] struct {
	slots            []atomic.Pointer[T]
	mask             uint64
	head             atomic.Uint64
	tail             atomic.Uint64
	dropOldestOnFull bool
}

/*
SpscArtifactRing carries broadcast payloads for a single subscriber.
*/
type SpscArtifactRing = SPSCRing[datura.Artifact]

/*
NewSPSCRing allocates a single-producer single-consumer queue.
*/
func NewSPSCRing[T any](capacity int, dropOldestOnFull bool) *SPSCRing[T] {
	return &SPSCRing[T]{
		head:             atomic.Uint64{},
		tail:             atomic.Uint64{},
		slots:            make([]atomic.Pointer[T], capacity),
		mask:             uint64(capacity - 1),
		dropOldestOnFull: dropOldestOnFull,
	}
}

func (ring *SPSCRing[T]) Push(value *T) bool {
	if ring == nil || value == nil {
		return false
	}

	for {
		head := ring.head.Load()
		tail := ring.tail.Load()

		if head-tail >= uint64(len(ring.slots)) {
			if ring.dropOldestOnFull {
				ring.Pop()

				continue
			}

			return false
		}

		index := head & ring.mask

		if !ring.slots[index].CompareAndSwap(nil, value) {
			continue
		}

		if ring.head.CompareAndSwap(head, head+1) {
			return true
		}

		ring.slots[index].Store(nil)
	}
}

func (ring *SPSCRing[T]) Pop() *T {
	if ring == nil {
		return nil
	}

	for {
		tail := ring.tail.Load()
		head := ring.head.Load()

		if tail >= head {
			return nil
		}

		index := tail & ring.mask
		value := ring.slots[index].Swap(nil)

		if value == nil {
			continue
		}

		if ring.tail.CompareAndSwap(tail, tail+1) {
			return value
		}

		ring.slots[index].Store(value)
	}
}

// Empty reports whether the ring currently holds no values. Read-only hint used
// by the broadcast consumer's idle park path; Push and Pop are untouched.
func (ring *SPSCRing[T]) Empty() bool {
	return ring == nil || ring.tail.Load() >= ring.head.Load()
}

func (ring *SPSCRing[T]) Close() error {
	if ring == nil {
		return errnie.Err(
			errnie.Conflict,
			"ring is nil",
			nil,
		)
	}

	for value := ring.Pop(); value != nil; value = ring.Pop() {
		value = nil
	}

	return nil
}
