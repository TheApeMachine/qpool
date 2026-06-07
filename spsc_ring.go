package qpool

import (
	"sync/atomic"
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
SpscQValueRing carries broadcast payloads for a single subscriber.
*/
type SpscQValueRing = SPSCRing[QValue[erasedAny]]

/*
NewSPSCRing allocates a single-producer single-consumer queue.
*/
func NewSPSCRing[T any](capacity int, dropOldestOnFull bool) *SPSCRing[T] {
	capacity = normalizeSPSCCapacity(capacity, dropOldestOnFull)

	return &SPSCRing[T]{
		slots:            make([]atomic.Pointer[T], capacity),
		mask:             uint64(capacity - 1),
		dropOldestOnFull: dropOldestOnFull,
	}
}

func normalizeSPSCCapacity(capacity int, dropOldestOnFull bool) int {
	if capacity < 1 {
		capacity = 1
	}

	if !dropOldestOnFull && capacity < 2 {
		capacity = 2
	}

	normalizer := RingBuffer[int]{}

	return normalizer.next(capacity)
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

func (ring *SPSCRing[T]) Close() {
	if ring == nil {
		return
	}

	for ring.Pop() != nil {
	}
}
