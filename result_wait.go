package qpool

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"unsafe"

	"github.com/theapemachine/datura"
)

const (
	slotPending uint32 = iota
	slotReady
	slotClosed
)

var errResultClosed = errors.New("qpool: result closed")

type waiterNode struct {
	gp   unsafe.Pointer
	sema uint32
	next atomic.Pointer[waiterNode]
}

type resultSlot struct {
	state   atomic.Uint32
	value   atomic.Pointer[datura.Artifact]
	waiters atomic.Pointer[waiterNode]
}

func newResultSlot() *resultSlot {
	return &resultSlot{}
}

func (slot *resultSlot) pushWaiter(node *waiterNode) {
	if node == nil {
		return
	}

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
		runtime_Semrelease(&node.sema, false, 0)
	}
}

func (slot *resultSlot) Wait(ctx context.Context) (*datura.Artifact, error) {
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

		switch slot.state.Load() {
		case slotReady:
			if value := slot.value.Load(); value != nil {
				return value, nil
			}
		case slotClosed:
			return nil, errResultClosed
		}

		slot.parkForWait(ctx)
	}
}

func (slot *resultSlot) removeWaiter(gp unsafe.Pointer) {
	if gp == nil {
		return
	}

	for {
		head := slot.waiters.Load()

		if head == nil {
			return
		}

		if head.gp == gp {
			if slot.waiters.CompareAndSwap(head, head.next.Load()) {
				return
			}

			continue
		}

		prev := head

		for node := head.next.Load(); node != nil; node = node.next.Load() {
			if node.gp == gp {
				if prev.next.CompareAndSwap(node, node.next.Load()) {
					return
				}

				break
			}

			prev = node
		}

		return
	}
}

func (slot *resultSlot) parkForWait(ctx context.Context) {
	if slot.state.Load() != slotPending {
		return
	}

	gp := GetG()
	node := &waiterNode{gp: gp}

	slot.pushWaiter(node)

	if slot.state.Load() != slotPending {
		slot.removeWaiter(gp)

		return
	}

	var stopAfterFunc func() bool

	if ctx.Done() != nil {
		stopAfterFunc = context.AfterFunc(ctx, func() {
			runtime_Semrelease(&node.sema, false, 0)
		})
	}

	runtime_Semacquire(&node.sema)
	slot.removeWaiter(gp)

	if stopAfterFunc != nil {
		stopAfterFunc()
	}
}

func (slot *resultSlot) Deliver(value *datura.Artifact) {
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
	immediate *datura.Artifact
}

func readyResultWait[T any](value *datura.Artifact) *ResultWait[T] {
	return &ResultWait[T]{immediate: value}
}

func pendingResultWait[T any](slot *resultSlot) *ResultWait[T] {
	return &ResultWait[T]{slot: slot}
}

func typedResultWait[T any](wait *ResultWait[erasedAny]) *ResultWait[T] {
	if wait == nil {
		return nil
	}

	if wait.immediate != nil {
		return &ResultWait[T]{immediate: wait.immediate}
	}

	return &ResultWait[T]{slot: wait.slot}
}

func errorResultWait[T any](err error) *ResultWait[T] {
	artifact, artifactErr := newErrorArtifact("", err, 0)

	if artifactErr != nil {
		return readyResultWait[T](artifact)
	}

	return readyResultWait[T](artifact)
}

/*
Get blocks until the result is ready or ctx is canceled.
*/
func (wait *ResultWait[T]) Get(ctx context.Context) (*datura.Artifact, error) {
	if wait == nil {
		return nil, errResultClosed
	}

	if wait.immediate != nil {
		return wait.immediate, nil
	}

	if wait.slot == nil {
		return nil, errResultClosed
	}

	return wait.slot.Wait(ctx)
}
