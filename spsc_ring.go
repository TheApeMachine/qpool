package qpool

import (
	"sync/atomic"
)

type spscQValueRing struct {
	slots []atomic.Pointer[QValue[erasedAny]]
	mask  uint64
	head  atomic.Uint64
	tail  atomic.Uint64
}

func newSPSCQValueRing(capacity int) *spscQValueRing {
	if capacity < 2 {
		capacity = 2
	}

	capacity = nextPowerOfTwo(capacity)

	return &spscQValueRing{
		slots: make([]atomic.Pointer[QValue[erasedAny]], capacity),
		mask:  uint64(capacity - 1),
	}
}

func (ring *spscQValueRing) Push(value *QValue[erasedAny]) bool {
	if ring == nil || value == nil {
		return false
	}

	for {
		head := ring.head.Load()
		tail := ring.tail.Load()

		if head-tail >= uint64(len(ring.slots)) {
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

func (ring *spscQValueRing) Pop() *QValue[erasedAny] {
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

func (ring *spscQValueRing) Close() {
	if ring == nil {
		return
	}

	for ring.Pop() != nil {
	}
}
