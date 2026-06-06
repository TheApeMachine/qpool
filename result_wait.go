package qpool

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
	"unsafe"
)

const (
	slotPending uint32 = iota
	slotReady
	slotClosed
)

var errResultClosed = errors.New("qpool: result closed")

type waiterNode struct {
	gp   uintptr
	next atomic.Pointer[waiterNode]
}

type resultSlot struct {
	state   atomic.Uint32
	value   atomic.Pointer[QValue[erasedAny]]
	waiters atomic.Pointer[waiterNode]
}

func newResultSlot() *resultSlot {
	return &resultSlot{}
}

func (slot *resultSlot) pushWaiter(gp unsafe.Pointer) {
	if gp == nil {
		return
	}

	node := &waiterNode{gp: uintptr(gp)}

	for {
		head := slot.waiters.Load()
		node.next.Store(head)

		if slot.waiters.CompareAndSwap(head, node) {
			return
		}
	}
}

func (slot *resultSlot) wakeWaiters() {
	head := slot.waiters.Swap(nil)

	for node := head; node != nil; node = node.next.Load() {
		if node.gp != 0 {
			safe_ready(unsafe.Pointer(node.gp))
		}
	}
}

func (slot *resultSlot) Wait(ctx context.Context) (*QValue[erasedAny], error) {
	for {
		switch slot.state.Load() {
		case slotReady:
			if value := slot.value.Load(); value != nil {
				return value, nil
			}

			return nil, fmt.Errorf("qpool: ready slot missing value")
		case slotClosed:
			return nil, errResultClosed
		}

		if err := ctx.Err(); err != nil {
			return nil, err
		}

		gp := GetG()
		slot.pushWaiter(gp)

		switch slot.state.Load() {
		case slotReady:
			if value := slot.value.Load(); value != nil {
				return value, nil
			}
		case slotClosed:
			return nil, errResultClosed
		}

		if gp != nil {
			fast_park(gp)
			continue
		}

		runtime.Gosched()
	}
}

func (slot *resultSlot) Deliver(value *QValue[erasedAny]) {
	slot.value.Store(value)

	if slot.state.CompareAndSwap(slotPending, slotReady) {
		slot.wakeWaiters()
	}
}

func (slot *resultSlot) Close() {
	for {
		state := slot.state.Load()

		switch state {
		case slotClosed:
			return
		case slotPending:
			if slot.state.CompareAndSwap(slotPending, slotClosed) {
				slot.wakeWaiters()

				return
			}
		case slotReady:
			if slot.state.CompareAndSwap(slotReady, slotClosed) {
				slot.wakeWaiters()

				return
			}
		}
	}
}

/*
ResultWait is a lock-free, channel-free completion handle for scheduled work.
*/
type ResultWait[T any] struct {
	slot      *resultSlot
	immediate *QValue[T]
}

func readyResultWait[T any](value *QValue[erasedAny]) *ResultWait[T] {
	return &ResultWait[T]{immediate: qValuePtr[T](value)}
}

func pendingResultWait[T any](slot *resultSlot) *ResultWait[T] {
	return &ResultWait[T]{slot: slot}
}

func typedResultWait[T any](wait *ResultWait[erasedAny]) *ResultWait[T] {
	if wait == nil {
		return nil
	}

	if wait.immediate != nil {
		return &ResultWait[T]{immediate: qValuePtr[T](wait.immediate)}
	}

	return &ResultWait[T]{slot: wait.slot}
}

func errorResultWait[T any](err error) *ResultWait[T] {
	value, qvalueErr := NewQValue[erasedAny]("", "", nil, 0)
	if qvalueErr != nil {
		value = &QValue[erasedAny]{Error: qvalueErr}
	} else {
		value.Error = err
	}

	return readyResultWait[T](value)
}

/*
Get blocks until the result is ready or ctx is canceled.
*/
func (wait *ResultWait[T]) Get(ctx context.Context) (*QValue[T], error) {
	if wait == nil {
		return nil, errResultClosed
	}

	if wait.immediate != nil {
		return wait.immediate, nil
	}

	if wait.slot == nil {
		return nil, errResultClosed
	}

	value, err := wait.slot.Wait(ctx)
	if err != nil {
		return nil, err
	}

	return qValuePtr[T](value), nil
}
