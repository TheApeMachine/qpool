package qpool

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
)

var broadcastGroups = sync.Map{}

/*
Subscriber represents a broadcast group subscriber.
*/
type Subscriber struct {
	ID       string
	Incoming chan *QValue[any]
}

/*
BroadcastGroup routes publisher messages to subscribers without mutexes.
*/
type BroadcastGroup struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	ID          string
	subscribers sync.Map
}

/*
NewBroadcastGroup starts an actor that owns subscriber maps.
*/
func NewBroadcastGroup(
	ctx context.Context, id string, ttl time.Duration,
) (*BroadcastGroup, error) {
	// If a group with the same id already exists, just return it.
	if bg, ok := broadcastGroups.Load(id); ok {
		return bg.(*BroadcastGroup), nil
	}

	ctx, cancel := context.WithCancel(ctx)

	// If no group with the same id exists, create a new one.
	bg := &BroadcastGroup{
		ctx:         ctx,
		cancel:      cancel,
		ID:          id,
		subscribers: sync.Map{},
	}

	// Store the new group in the global map.
	broadcastGroups.Store(id, bg)

	return bg, errnie.Require(
		map[string]any{
			"ctx":         bg.ctx,
			"cancel":      bg.cancel,
			"id":          bg.ID,
			"subscribers": &bg.subscribers,
		},
	)
}

/*
Subscribe registers a subscriber channel sized with bufferSize.
*/
func (bg *BroadcastGroup) Subscribe(
	subscriberID string, bufferSize int,
) *Subscriber {
	select {
	case <-bg.ctx.Done():
		return nil
	default:
		subscriber := &Subscriber{
			ID:       subscriberID,
			Incoming: make(chan *QValue[any], bufferSize),
		}

		bg.subscribers.Store(subscriberID, subscriber)
		return subscriber
	}
}

/*
Unsubscribe removes a subscriber.
*/
func (bg *BroadcastGroup) Unsubscribe(subscriberID string) {
	select {
	case <-bg.ctx.Done():
		return
	default:
		bg.subscribers.Delete(subscriberID)
	}
}

/*
Send delivers qv to subscribers honoring filters and routing rules.
*/
func (bg *BroadcastGroup) Send(qv *QValue[any]) {
	select {
	case <-bg.ctx.Done():
		return
	default:
		if qv.ReceiverID != "" {
			if subscriber, ok := bg.subscribers.Load(qv.ReceiverID); ok {
				subscriber.(*Subscriber).Incoming <- qv
			}

			return
		}

		bg.subscribers.Range(func(key, value any) bool {
			subscriber := value.(*Subscriber)

			if subscriber.ID == qv.SenderID {
				return true
			}

			return true
		})
	}
}

/*
Close stops the actor and closes subscriber channels.
*/
func (bg *BroadcastGroup) Close() {
	select {
	case <-bg.ctx.Done():
		return
	default:
		bg.cancel()

		bg.subscribers.Range(func(key, value any) bool {
			close(value.(*Subscriber).Incoming)
			return true
		})

		broadcastGroups.Delete(bg.ID)
	}
}
