package qpool

/*
RingBuffer is a power-of-two slot store indexed by sequence number.
go-disruptor owns sequencing; this type only provides masked slot access.
*/
type RingBuffer[T any] struct {
	slots []T
	mask  int64
}

/*
NewRingBuffer allocates a power-of-two slot store for disruptor sequence indexing.
*/
func NewRingBuffer[T any](capacity int) RingBuffer[T] {
	normalizer := RingBuffer[T]{}
	capacity = normalizer.next(max(1, capacity))

	return RingBuffer[T]{
		slots: make([]T, capacity),
		mask:  int64(capacity - 1),
	}
}

func (buffer RingBuffer[T]) Capacity() int {
	return len(buffer.slots)
}

func (buffer RingBuffer[T]) Slot(sequence int64) *T {
	return &buffer.slots[sequence&buffer.mask]
}

func (buffer RingBuffer[T]) next(value int) int {
	if value <= 1 {
		return 1
	}

	power := 1

	for power < value {
		power <<= 1
	}

	return power
}
