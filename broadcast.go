package qpool

import "sync"

const defaultBroadcastBuffer = 128

/*
Broadcaster fans qpool events out to independent subscribers.
*/
type Broadcaster struct {
	lock        sync.RWMutex
	nextID      uint64
	subscribers map[uint64]*Subscription
}

/*
Subscription receives qpool events from a Broadcaster.
*/
type Subscription struct {
	broadcaster *Broadcaster
	id          uint64
	events      chan Event
	mu          sync.Mutex
	once        sync.Once
}

var defaultBroadcaster = NewBroadcaster()

/*
NewBroadcaster creates an event broadcaster.
*/
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		subscribers: make(map[uint64]*Subscription),
	}
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

	broadcaster.lock.Lock()
	defer broadcaster.lock.Unlock()

	broadcaster.nextID++

	subscription := &Subscription{
		broadcaster: broadcaster,
		id:          broadcaster.nextID,
		events:      make(chan Event, buffer),
	}
	broadcaster.subscribers[subscription.id] = subscription

	return subscription
}

/*
Publish broadcasts an event to all subscribers.
*/
func (broadcaster *Broadcaster) Publish(event Event) {
	broadcaster.lock.RLock()
	defer broadcaster.lock.RUnlock()

	for _, subscription := range broadcaster.subscribers {
		subscription.deliver(event.clone())
	}
}

/*
Events returns the receive side of the subscription channel.
*/
func (subscription *Subscription) Events() <-chan Event {
	if subscription == nil {
		return nil
	}

	return subscription.events
}

/*
Close unregisters the subscription and closes its event channel.
*/
func (subscription *Subscription) Close() {
	if subscription == nil {
		return
	}

	subscription.once.Do(func() {
		subscription.broadcaster.unsubscribe(subscription)
	})
}

func (subscription *Subscription) deliver(event Event) {
	subscription.mu.Lock()
	defer subscription.mu.Unlock()

	select {
	case subscription.events <- event:
		return
	default:
	}

	select {
	case <-subscription.events:
	default:
	}

	select {
	case subscription.events <- event:
	default:
	}
}

func (broadcaster *Broadcaster) unsubscribe(subscription *Subscription) {
	broadcaster.lock.Lock()
	defer broadcaster.lock.Unlock()

	if _, exists := broadcaster.subscribers[subscription.id]; !exists {
		return
	}

	delete(broadcaster.subscribers, subscription.id)
	close(subscription.events)
}

func (event Event) clone() Event {
	if len(event.Fields) == 0 {
		return event
	}

	event.Fields = append([]Field(nil), event.Fields...)

	return event
}
