package qpool

import (
	"sort"
	"sync/atomic"
	"time"
)

/*
FilterFunc filters broadcast payloads before delivery.
*/
type FilterFunc func(*QValue) bool

/*
RoutingRule binds subscriber-specific routing metadata.
*/
type RoutingRule struct {
	SubscriberID string
	Filter       FilterFunc
	Priority     int
}

/*
BroadcastMetrics tracks coarse broadcast statistics (updated only from the broadcast actor).
*/
type BroadcastMetrics struct {
	MessagesSent         int64
	MessagesDropped      int64
	LastBroadcastLatency time.Duration
	ActiveSubscribers    int64
	LastBroadcastTime    time.Time
}

type broadcastActorState struct {
	ID           string
	subscribers  map[string]chan *QValue
	routingRules map[string][]RoutingRule
	filters      []FilterFunc
	metrics      BroadcastMetrics
	maxQueueSize int
}

/*
BroadcastGroup routes publisher messages to subscribers without mutexes.
*/
type BroadcastGroup struct {
	TTL            time.Duration
	lastUsedUnixNs atomic.Int64

	ops    chan func(*broadcastActorState)
	closed chan struct{}
	closer atomic.Uint32
}

/*
NewBroadcastGroup starts an actor that owns subscriber maps.
*/
func NewBroadcastGroup(id string, ttl time.Duration, maxQueue int) *BroadcastGroup {
	bg := &BroadcastGroup{
		TTL:    ttl,
		ops:    make(chan func(*broadcastActorState), 256),
		closed: make(chan struct{}),
	}

	bg.lastUsedUnixNs.Store(time.Now().UnixNano())

	go bg.loop(&broadcastActorState{
		ID:           id,
		subscribers:  make(map[string]chan *QValue),
		routingRules: make(map[string][]RoutingRule),
		maxQueueSize: maxQueue,
	})

	return bg
}

func (bg *BroadcastGroup) loop(initial *broadcastActorState) {
	st := initial

	for {
		select {
		case fn := <-bg.ops:
			if fn != nil {
				fn(st)
			}

		case <-bg.closed:
			for _, ch := range st.subscribers {
				close(ch)
			}

			st.subscribers = nil
			st.routingRules = nil
			st.filters = nil

			return
		}
	}
}

/*
Subscribe registers a subscriber channel sized with bufferSize.
*/
func (bg *BroadcastGroup) Subscribe(subscriberID string, bufferSize int, rules ...RoutingRule) chan *QValue {
	if bg.closer.Load() != 0 {
		return nil
	}

	reply := make(chan chan *QValue, 1)

	select {
	case <-bg.closed:
		return nil
	case bg.ops <- func(st *broadcastActorState) {
		ch := make(chan *QValue, bufferSize)
		st.subscribers[subscriberID] = ch

		if len(rules) > 0 {
			st.routingRules[subscriberID] = append([]RoutingRule(nil), rules...)
		}

		st.metrics.ActiveSubscribers = int64(len(st.subscribers))
		select {
		case reply <- ch:
		default:
		}
	}:
	}

	select {
	case <-bg.closed:
		return nil
	case ch := <-reply:
		return ch
	}
}

/*
Unsubscribe removes a subscriber.
*/
func (bg *BroadcastGroup) Unsubscribe(subscriberID string) {
	select {
	case <-bg.closed:
		return
	default:
	}

	done := make(chan struct{})

	select {
	case <-bg.closed:
		return
	case bg.ops <- func(st *broadcastActorState) {
		defer close(done)

		if ch, ok := st.subscribers[subscriberID]; ok {
			close(ch)
			delete(st.subscribers, subscriberID)
			delete(st.routingRules, subscriberID)
			st.metrics.ActiveSubscribers = int64(len(st.subscribers))
		}
	}:
	}

	select {
	case <-bg.closed:
		return
	case <-done:
	}
}

/*
Send delivers qv to subscribers honoring filters and routing rules.
*/
func (bg *BroadcastGroup) Send(qv *QValue) {
	if bg.closer.Load() != 0 {
		return
	}

	bg.lastUsedUnixNs.Store(time.Now().UnixNano())

	done := make(chan struct{})

	select {
	case <-bg.closed:
		return
	case bg.ops <- func(st *broadcastActorState) {
		defer close(done)

		startTime := time.Now()

		for _, filter := range st.filters {
			if filter != nil && !filter(qv) {
				st.metrics.MessagesDropped++
				return
			}
		}

		for subID, ch := range st.subscribers {
			if rules, ok := st.routingRules[subID]; ok && len(rules) > 0 {
				shouldSend := false

				for _, rule := range rules {
					if rule.Filter != nil && rule.Filter(qv) {
						shouldSend = true
						break
					}
				}

				if !shouldSend {
					continue
				}
			}

			select {
			case ch <- qv:
				st.metrics.MessagesSent++
			default:
				st.metrics.MessagesDropped++
			}
		}

		st.metrics.LastBroadcastTime = startTime
		st.metrics.LastBroadcastLatency = time.Since(startTime)
	}:
	}

	select {
	case <-bg.closed:
	case <-done:
	}
}

/*
AddFilter appends a global filter evaluated before routing.
*/
func (bg *BroadcastGroup) AddFilter(filter FilterFunc) {
	select {
	case <-bg.closed:
		return
	case bg.ops <- func(st *broadcastActorState) {
		st.filters = append(st.filters, filter)
	}:
	}
}

/*
AddRoutingRule registers an extra routing rule for subscriberID.
*/
func (bg *BroadcastGroup) AddRoutingRule(subscriberID string, rule RoutingRule) {
	select {
	case <-bg.closed:
		return
	case bg.ops <- func(st *broadcastActorState) {
		st.routingRules[subscriberID] = append(st.routingRules[subscriberID], rule)
	}:
	}
}

/*
SubscriberIDs returns the registered subscriber ids in sorted order for
diagnostics and lightweight UIs that list who is listening.
*/
func (bg *BroadcastGroup) SubscriberIDs() []string {
	reply := make(chan []string, 1)

	select {
	case <-bg.closed:
		return nil
	case bg.ops <- func(st *broadcastActorState) {
		out := make([]string, 0, len(st.subscribers))

		for id := range st.subscribers {
			out = append(out, id)
		}

		sort.Strings(out)
		reply <- out
	}:
	}

	select {
	case <-bg.closed:
		return nil
	case out := <-reply:
		return out
	}
}

/*
GetMetrics returns a shallow copy of broadcast metrics.
*/
func (bg *BroadcastGroup) GetMetrics() BroadcastMetrics {
	reply := make(chan BroadcastMetrics, 1)

	select {
	case <-bg.closed:
		return BroadcastMetrics{}
	case bg.ops <- func(st *broadcastActorState) {
		reply <- st.metrics
	}:
	}

	select {
	case <-bg.closed:
		return BroadcastMetrics{}
	case m := <-reply:
		return m
	}
}

/*
Close stops the actor and closes subscriber channels.
*/
func (bg *BroadcastGroup) Close() {
	if !bg.closer.CompareAndSwap(0, 1) {
		return
	}

	select {
	case <-bg.closed:
		return
	default:
		close(bg.closed)
	}
}

/*
Expired reports whether the group's idle duration exceeds TTL when TTL is positive.
*/
func (bg *BroadcastGroup) Expired(now time.Time) bool {
	if bg.TTL <= 0 {
		return false
	}

	last := time.Unix(0, bg.lastUsedUnixNs.Load())

	return now.Sub(last) > bg.TTL
}
