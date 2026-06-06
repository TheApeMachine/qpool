package qpool

import (
	"sync/atomic"
)

const defaultBroadcastBuffer = 128

/*
Broadcaster fans qpool events out to independent subscribers.
*/
type Broadcaster struct {
	nextID      atomic.Uint64
	subscribers atomic.Pointer[subscriptionEntry]
}

type subscriptionEntry struct {
	id           uint64
	subscription *Subscription
	next         atomic.Pointer[subscriptionEntry]
}

/*
Subscription receives qpool events from a Broadcaster.
*/
type Subscription struct {
	broadcaster *Broadcaster
	id          uint64
	ring        *spscEventRing
	closed      atomic.Bool
}

var defaultBroadcaster = NewBroadcaster()

/*
NewBroadcaster creates an event broadcaster.
*/
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{}
}

/*
Subscribe registers a listener on the default qpool event stream.
*/
func Subscribe(buffer int) *Subscription {
	return defaultBroadcaster.Subscribe(buffer)
}

/*
Subscribe registers a listener on this broadcaster.
*/
func (broadcaster *Broadcaster) Subscribe(buffer int) *Subscription {
	if buffer < 1 {
		buffer = defaultBroadcastBuffer
	}

	id := broadcaster.nextID.Add(1)

	subscription := &Subscription{
		broadcaster: broadcaster,
		id:          id,
		ring:        newSPSCEventRing(buffer),
	}

	entry := &subscriptionEntry{
		id:           id,
		subscription: subscription,
	}

	for {
		head := broadcaster.subscribers.Load()
		entry.next.Store(head)

		if broadcaster.subscribers.CompareAndSwap(head, entry) {
			return subscription
		}
	}
}

/*
Publish broadcasts an event to all subscribers.
*/
func (broadcaster *Broadcaster) Publish(event Event) {
	for entry := broadcaster.subscribers.Load(); entry != nil; entry = entry.next.Load() {
		if entry.subscription != nil {
			entry.subscription.deliver(event.clone())
		}
	}
}

/*
Poll returns the next event when one is available.
*/
func (subscription *Subscription) Poll() (Event, bool) {
	if subscription == nil || subscription.ring == nil {
		return Event{}, false
	}

	return subscription.ring.Pop()
}

/*
Close unregisters the subscription and releases its ring.
*/
func (subscription *Subscription) Close() {
	if subscription == nil || subscription.closed.Swap(true) {
		return
	}

	subscription.broadcaster.unsubscribe(subscription)

	if subscription.ring != nil {
		subscription.ring.Close()
	}
}

func (subscription *Subscription) deliver(event Event) {
	if subscription == nil || subscription.closed.Load() || subscription.ring == nil {
		return
	}

	subscription.ring.Push(event)
}

func (broadcaster *Broadcaster) unsubscribe(subscription *Subscription) {
	if broadcaster == nil || subscription == nil {
		return
	}

	for {
		prev := (*subscriptionEntry)(nil)
		current := broadcaster.subscribers.Load()

		for current != nil {
			next := current.next.Load()

			if current.id == subscription.id {
				if prev == nil {
					if broadcaster.subscribers.CompareAndSwap(current, next) {
						return
					}

					break
				}

				prev.next.Store(next)

				return
			}

			prev = current
			current = next
		}

		return
	}
}

func (event Event) clone() Event {
	if len(event.Fields) == 0 {
		return event
	}

	event.Fields = append([]Field(nil), event.Fields...)

	return event
}

type spscEventRing struct {
	slots []atomic.Pointer[eventSlot]
	mask  uint64
	head  atomic.Uint64
	tail  atomic.Uint64
}

type eventSlot struct {
	event Event
}

func newSPSCEventRing(capacity int) *spscEventRing {
	if capacity < 2 {
		capacity = 2
	}

	capacity = nextPowerOfTwo(capacity)

	return &spscEventRing{
		slots: make([]atomic.Pointer[eventSlot], capacity),
		mask:  uint64(capacity - 1),
	}
}

func (ring *spscEventRing) Push(event Event) bool {
	if ring == nil {
		return false
	}

	for {
		head := ring.head.Load()
		tail := ring.tail.Load()

		if head-tail >= uint64(len(ring.slots)) {
			_, _ = ring.Pop()

			continue
		}

		index := head & ring.mask
		slot := &eventSlot{event: event}

		if !ring.slots[index].CompareAndSwap(nil, slot) {
			continue
		}

		if ring.head.CompareAndSwap(head, head+1) {
			return true
		}

		ring.slots[index].Store(nil)
	}
}

func (ring *spscEventRing) Pop() (Event, bool) {
	if ring == nil {
		return Event{}, false
	}

	for {
		tail := ring.tail.Load()
		head := ring.head.Load()

		if tail >= head {
			return Event{}, false
		}

		index := tail & ring.mask
		slot := ring.slots[index].Swap(nil)

		if slot == nil {
			continue
		}

		if ring.tail.CompareAndSwap(tail, tail+1) {
			return slot.event, true
		}

		ring.slots[index].Store(slot)
	}
}

func (ring *spscEventRing) Close() {
	if ring == nil {
		return
	}

	for {
		if _, ok := ring.Pop(); !ok {
			return
		}
	}
}
