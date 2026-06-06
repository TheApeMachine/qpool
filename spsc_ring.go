package qpool

import (
	"sync/atomic"
)

type SpscQValueRing struct {
	slots            []atomic.Pointer[QValue[erasedAny]]
	mask             uint64
	head             atomic.Uint64
	tail             atomic.Uint64
	dropOldestOnFull bool
}

func NewSPSCQValueRing(capacity int, dropOldestOnFull bool) *SpscQValueRing {
	if capacity < 1 {
		capacity = 1
	}

	if !dropOldestOnFull && capacity < 2 {
		capacity = 2
	}

	capacity = nextPowerOfTwo(capacity)

	return &SpscQValueRing{
		slots:            make([]atomic.Pointer[QValue[erasedAny]], capacity),
		mask:             uint64(capacity - 1),
		dropOldestOnFull: dropOldestOnFull,
	}
}

func (ring *SpscQValueRing) Push(value *QValue[erasedAny]) bool {
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

func (ring *SpscQValueRing) Pop() *QValue[erasedAny] {
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

func (ring *SpscQValueRing) Close() {
	if ring == nil {
		return
	}

	for ring.Pop() != nil {
	}
}
