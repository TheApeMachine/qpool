package qpool

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
)

const (
	defaultBroadcastBuffer = 128
	telemetryGroupID       = "__qpool.telemetry__"
)

var defaultTelemetryGroup *BroadcastGroup

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
Subscription receives telemetry events from a BroadcastGroup.
*/
type Subscription struct {
	group        *BroadcastGroup
	subscriberID string
	consumer     *BroadcastConsumer
	closed       atomic.Bool
}

/*
BroadcastGroup routes publisher messages to subscribers without mutexes or channels.
*/
type BroadcastGroup struct {
	ctx              context.Context
	cancel           context.CancelFunc
	err              error
	ID               string
	nextSubscriberID atomic.Uint64
	dropOldestOnFull bool
	subscribers      atomic.Pointer[subscriberEntry]
}

type subscriberEntry struct {
	id         string
	subscriber *Subscriber
	next       atomic.Pointer[subscriberEntry]
}

/*
NewBroadcaster creates a standalone broadcast group for telemetry-style fan-out.
*/
func NewBroadcaster() *BroadcastGroup {
	return newStandaloneBroadcastGroup(true)
}

func newStandaloneBroadcastGroup(dropOldestOnFull bool) *BroadcastGroup {
	ctx, cancel := context.WithCancel(context.Background())

	return &BroadcastGroup{
		ctx:              ctx,
		cancel:           cancel,
		dropOldestOnFull: dropOldestOnFull,
	}
}

func initDefaultTelemetryGroup() *BroadcastGroup {
	ctx, cancel := context.WithCancel(context.Background())

	return &BroadcastGroup{
		ctx:              ctx,
		cancel:           cancel,
		ID:               telemetryGroupID,
		dropOldestOnFull: true,
	}
}

/*
Subscribe registers a listener on the default qpool telemetry stream.
*/
func Subscribe(buffer int) *Subscription {
	return defaultTelemetryGroup.SubscribeEvents(buffer)
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

	bg.store()

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

func (bg *BroadcastGroup) store() {
	entry := &groupRegistryEntryGlobal{
		keyHash: fnvHash(bg.ID),
		key:     bg.ID,
	}

	entry.group.Store(bg)

	for {
		head := broadcastGroupHead.Load()
		entry.next.Store(head)

		if broadcastGroupHead.CompareAndSwap(head, entry) {
			return
		}
	}
}

func (bg *BroadcastGroup) delete() {
	keyHash := fnvHash(bg.ID)

	for {
		prev := (*groupRegistryEntryGlobal)(nil)
		current := broadcastGroupHead.Load()
		retry := false

		for current != nil {
			next := current.next.Load()

			if current.keyHash == keyHash && current.key == bg.ID {
				if prev == nil {
					if broadcastGroupHead.CompareAndSwap(current, next) {
						return
					}

					retry = true

					break
				}

				prev.next.Store(next)

				return
			}

			prev = current
			current = next
		}

		if !retry {
			return
		}
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

	if subscriberID == "" {
		subscriberID = fmt.Sprintf("%d", bg.nextSubscriberID.Add(1))
	}

	if bufferSize < 1 {
		bufferSize = defaultBroadcastBuffer
	}

	consumer := &BroadcastConsumer{
		ring: newSPSCQValueRing(bufferSize, bg.dropOldestOnFull),
	}
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
SubscribeEvents registers a telemetry listener with an auto-assigned subscriber id.
*/
func (bg *BroadcastGroup) SubscribeEvents(buffer int) *Subscription {
	if bg == nil {
		return nil
	}

	subscriberID := fmt.Sprintf("%d", bg.nextSubscriberID.Add(1))
	consumer := bg.Subscribe(subscriberID, buffer)

	if consumer == nil {
		return nil
	}

	return &Subscription{
		group:        bg,
		subscriberID: subscriberID,
		consumer:     consumer,
	}
}

/*
Publish broadcasts a telemetry event to every subscriber.
*/
func (bg *BroadcastGroup) Publish(event Event) {
	if bg == nil {
		return
	}

	select {
	case <-bg.ctx.Done():
		return
	default:
	}

	for entry := bg.subscribers.Load(); entry != nil; entry = entry.next.Load() {
		if entry.subscriber == nil || entry.subscriber.consumer == nil {
			continue
		}

		cloned := event.clone()
		value := &QValue[erasedAny]{Value: cloned}

		entry.subscriber.consumer.ring.Push(value)
	}
}

/*
Poll returns the next telemetry event when one is available.
*/
func (subscription *Subscription) Poll() (Event, bool) {
	if subscription == nil || subscription.consumer == nil {
		return Event{}, false
	}

	value := subscription.consumer.Poll()

	if value == nil {
		return Event{}, false
	}

	event, ok := value.Value.(Event)

	return event, ok
}

/*
Close unregisters the subscription and releases its ring.
*/
func (subscription *Subscription) Close() {
	if subscription == nil || subscription.closed.Swap(true) {
		return
	}

	subscription.group.Unsubscribe(subscription.subscriberID)

	if subscription.consumer != nil && subscription.consumer.ring != nil {
		subscription.consumer.ring.Close()
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
		retry := false

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

					retry = true

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

		if !retry {
			return
		}
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
	bg.delete()
}
