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

type subscriberEntry struct {
	id         string
	subscriber *Subscriber
	next       atomic.Pointer[subscriberEntry]
}

type subscriberEntryList struct {
	list IntrusiveList[subscriberEntry]
}

func newSubscriberEntryList() *subscriberEntryList {
	entries := &subscriberEntryList{}
	entries.list.bind(
		func(entry *subscriberEntry) *subscriberEntry {
			return entry.next.Load()
		},
		func(entry, next *subscriberEntry) {
			entry.next.Store(next)
		},
		func(prev, current, next *subscriberEntry) bool {
			return prev.next.CompareAndSwap(current, next)
		},
	)

	return entries
}

func (entries *subscriberEntryList) Prepend(entry *subscriberEntry) {
	entries.list.Prepend(entry)
}

func (entries *subscriberEntryList) Remove(id string) *subscriberEntry {
	return entries.list.RemoveReturning(func(entry *subscriberEntry) bool {
		return entry.id == id
	})
}

func (entries *subscriberEntryList) IsEmpty() bool {
	return entries.list.Head() == nil
}

func (entries *subscriberEntryList) Find(id string) *subscriberEntry {
	return entries.list.Find(func(entry *subscriberEntry) bool {
		return entry.id == id
	})
}

func (entries *subscriberEntryList) Walk(visitor func(*subscriberEntry)) {
	entries.list.Walk(visitor)
}

func (entries *subscriberEntryList) Clear() {
	entries.list.Clear()
}

func (entries *subscriberEntryList) count() int {
	count := 0

	entries.list.Walk(func(*subscriberEntry) {
		count++
	})

	return count
}

/*
BroadcastConsumer receives values from a broadcast group without channels.
*/
type BroadcastConsumer struct {
	ring *SpscQValueRing
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

		mcall(fast_park)
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
	subscribers      *subscriberEntryList
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
		subscribers:      newSubscriberEntryList(),
	}
}

func initDefaultTelemetryGroup() *BroadcastGroup {
	ctx, cancel := context.WithCancel(context.Background())

	return &BroadcastGroup{
		ctx:              ctx,
		cancel:           cancel,
		ID:               telemetryGroupID,
		dropOldestOnFull: true,
		subscribers:      newSubscriberEntryList(),
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
	_ = ttl

	ctx, cancel := context.WithCancel(ctx)

	bg := &BroadcastGroup{
		ctx:         ctx,
		cancel:      cancel,
		ID:          id,
		subscribers: newSubscriberEntryList(),
	}

	return bg, errnie.Require(
		map[string]any{
			"ctx":    bg.ctx,
			"cancel": bg.cancel,
			"id":     bg.ID,
		},
	)
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
		ring: NewSPSCRing[QValue[erasedAny]](bufferSize, bg.dropOldestOnFull),
	}
	subscriber := &Subscriber{
		ID:       subscriberID,
		consumer: consumer,
	}

	entry := &subscriberEntry{
		id:         subscriberID,
		subscriber: subscriber,
	}

	bg.subscribers.Prepend(entry)

	return consumer
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

	bg.subscribers.Walk(func(entry *subscriberEntry) {
		if entry.subscriber == nil || entry.subscriber.consumer == nil {
			return
		}

		cloned := event.clone()
		value := &QValue[erasedAny]{Value: cloned}

		entry.subscriber.consumer.ring.Push(value)
	})
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

	entry := bg.subscribers.Remove(subscriberID)

	if entry != nil && entry.subscriber != nil && entry.subscriber.consumer != nil {
		entry.subscriber.consumer.ring.Close()
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
		entry := bg.subscribers.Find(qv.ReceiverID)

		if entry != nil && entry.subscriber != nil && entry.subscriber.consumer != nil {
			entry.subscriber.consumer.ring.Push(qv)
		}

		return
	}

	bg.subscribers.Walk(func(entry *subscriberEntry) {
		if entry.subscriber == nil || entry.subscriber.consumer == nil {
			return
		}

		if entry.id == qv.SenderID {
			return
		}

		entry.subscriber.consumer.ring.Push(qv)
	})
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

	bg.subscribers.Walk(func(entry *subscriberEntry) {
		if entry.subscriber != nil && entry.subscriber.consumer != nil {
			entry.subscriber.consumer.ring.Close()
		}
	})

	bg.subscribers.Clear()
}
