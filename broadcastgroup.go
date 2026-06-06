package qpool

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
)

type groupRegistryEntryGlobal struct {
	keyHash uint64
	key     string
	group   atomic.Pointer[BroadcastGroup]
	next    atomic.Pointer[groupRegistryEntryGlobal]
}

var broadcastGroupHead atomic.Pointer[groupRegistryEntryGlobal]

/*
BroadcastConsumer receives values from a broadcast group without channels.
*/
type BroadcastConsumer struct {
	ring *spscQValueRing
}

/*
Poll returns the next queued value when one is available.
*/
func (consumer *BroadcastConsumer) Poll() *QValue[erasedAny] {
	if consumer == nil || consumer.ring == nil {
		return nil
	}

	return consumer.ring.Pop()
}

/*
Wait blocks until a value is available or ctx is canceled.
*/
func (consumer *BroadcastConsumer) Wait(ctx context.Context) (*QValue[erasedAny], error) {
	if consumer == nil || consumer.ring == nil {
		return nil, errResultClosed
	}

	for {
		if value := consumer.ring.Pop(); value != nil {
			return value, nil
		}

		if err := ctx.Err(); err != nil {
			return nil, err
		}

		gp := GetG()
		if gp != nil {
			fast_park(gp)
		}
	}
}

/*
Subscriber represents a broadcast group subscriber.
*/
type Subscriber struct {
	ID       string
	consumer *BroadcastConsumer
}

/*
BroadcastGroup routes publisher messages to subscribers without mutexes or channels.
*/
type BroadcastGroup struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	ID          string
	subscribers atomic.Pointer[subscriberEntry]
}

type subscriberEntry struct {
	id         string
	subscriber *Subscriber
	next       atomic.Pointer[subscriberEntry]
}

/*
NewBroadcastGroup starts a group that owns subscriber registrations.
*/
func NewBroadcastGroup(
	ctx context.Context, id string, ttl time.Duration,
) (*BroadcastGroup, error) {
	if existing := findBroadcastGroup(id); existing != nil {
		return existing, nil
	}

	ctx, cancel := context.WithCancel(ctx)

	bg := &BroadcastGroup{
		ctx:    ctx,
		cancel: cancel,
		ID:     id,
	}

	storeBroadcastGroup(id, bg)

	return bg, errnie.Require(
		map[string]any{
			"ctx":    bg.ctx,
			"cancel": bg.cancel,
			"id":     bg.ID,
		},
	)
}

func findBroadcastGroup(id string) *BroadcastGroup {
	keyHash := fnvHash(id)

	for entry := broadcastGroupHead.Load(); entry != nil; entry = entry.next.Load() {
		if entry.keyHash == keyHash && entry.key == id {
			return entry.group.Load()
		}
	}

	return nil
}

func storeBroadcastGroup(id string, group *BroadcastGroup) {
	entry := &groupRegistryEntryGlobal{
		keyHash: fnvHash(id),
		key:     id,
	}

	entry.group.Store(group)

	for {
		head := broadcastGroupHead.Load()
		entry.next.Store(head)

		if broadcastGroupHead.CompareAndSwap(head, entry) {
			return
		}
	}
}

func deleteBroadcastGroup(id string) {
	keyHash := fnvHash(id)

	for {
		prev := (*groupRegistryEntryGlobal)(nil)
		current := broadcastGroupHead.Load()

		for current != nil {
			next := current.next.Load()

			if current.keyHash == keyHash && current.key == id {
				if prev == nil {
					if broadcastGroupHead.CompareAndSwap(current, next) {
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

/*
Subscribe registers a subscriber ring sized with bufferSize.
*/
func (bg *BroadcastGroup) Subscribe(
	subscriberID string, bufferSize int,
) *BroadcastConsumer {
	if bg == nil {
		return nil
	}

	select {
	case <-bg.ctx.Done():
		return nil
	default:
	}

	consumer := &BroadcastConsumer{ring: newSPSCQValueRing(bufferSize)}
	subscriber := &Subscriber{
		ID:       subscriberID,
		consumer: consumer,
	}

	entry := &subscriberEntry{
		id:         subscriberID,
		subscriber: subscriber,
	}

	for {
		head := bg.subscribers.Load()
		entry.next.Store(head)

		if bg.subscribers.CompareAndSwap(head, entry) {
			return consumer
		}
	}
}

/*
Unsubscribe removes a subscriber.
*/
func (bg *BroadcastGroup) Unsubscribe(subscriberID string) {
	if bg == nil {
		return
	}

	select {
	case <-bg.ctx.Done():
		return
	default:
	}

	for {
		prev := (*subscriberEntry)(nil)
		current := bg.subscribers.Load()

		for current != nil {
			next := current.next.Load()

			if current.id == subscriberID {
				if prev == nil {
					if bg.subscribers.CompareAndSwap(current, next) {
						if current.subscriber != nil && current.subscriber.consumer != nil {
							current.subscriber.consumer.ring.Close()
						}

						return
					}

					break
				}

				prev.next.Store(next)

				if current.subscriber != nil && current.subscriber.consumer != nil {
					current.subscriber.consumer.ring.Close()
				}

				return
			}

			prev = current
			current = next
		}

		return
	}
}

/*
Send delivers qv to subscribers honoring filters and routing rules.
*/
func (bg *BroadcastGroup) Send(qv *QValue[erasedAny]) {
	if bg == nil || qv == nil {
		return
	}

	select {
	case <-bg.ctx.Done():
		return
	default:
	}

	if qv.ReceiverID != "" {
		for entry := bg.subscribers.Load(); entry != nil; entry = entry.next.Load() {
			if entry.id != qv.ReceiverID {
				continue
			}

			if entry.subscriber != nil && entry.subscriber.consumer != nil {
				entry.subscriber.consumer.ring.Push(qv)
			}

			return
		}

		return
	}

	for entry := bg.subscribers.Load(); entry != nil; entry = entry.next.Load() {
		if entry.subscriber == nil || entry.subscriber.consumer == nil {
			continue
		}

		if entry.id == qv.SenderID {
			continue
		}

		entry.subscriber.consumer.ring.Push(qv)
	}
}

/*
Close stops the group and releases subscriber rings.
*/
func (bg *BroadcastGroup) Close() {
	if bg == nil {
		return
	}

	select {
	case <-bg.ctx.Done():
		return
	default:
	}

	bg.cancel()

	for entry := bg.subscribers.Load(); entry != nil; entry = entry.next.Load() {
		if entry.subscriber != nil && entry.subscriber.consumer != nil {
			entry.subscriber.consumer.ring.Close()
		}
	}

	bg.subscribers.Store(nil)
	deleteBroadcastGroup(bg.ID)
}
