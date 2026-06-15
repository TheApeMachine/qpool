package qpool

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

/*
BroadcastGroup routes publisher messages
to subscribers without mutexes or channels.
*/
type BroadcastGroup struct {
	ctx              context.Context
	cancel           context.CancelFunc
	err              error
	ttl              time.Duration
	ID               string
	nextSubscriberID atomic.Uint64
	dropOldestOnFull bool
	consumers        *sync.Map
}

/*
NewBroadcastGroup starts a group that owns subscriber registrations.
*/
func NewBroadcastGroup(
	ctx context.Context, id string, ttl time.Duration,
) *BroadcastGroup {
	ctx, cancel := context.WithCancel(ctx)

	bg := &BroadcastGroup{
		ctx:              ctx,
		cancel:           cancel,
		ttl:              ttl,
		ID:               id,
		dropOldestOnFull: true,
		consumers:        &sync.Map{},
	}

	if err := errnie.Error(errnie.Require(
		map[string]any{
			"ctx":              bg.ctx,
			"cancel":           bg.cancel,
			"ttl":              bg.ttl,
			"id":               bg.ID,
			"dropOldestOnFull": bg.dropOldestOnFull,
			"consumers":        bg.consumers,
		},
	)); err != nil {
		return nil
	}

	return bg
}

/*
Acquire registers a consumer ring sized with bufferSize.
*/
func (bg *BroadcastGroup) Acquire(
	subscriberID string, callback func(*datura.Artifact) error,
) *BroadcastConsumer {
	select {
	case <-bg.ctx.Done():
		return nil
	default:
	}

	var (
		existing any
		ok       bool
	)

	if existing, ok = bg.consumers.LoadOrStore(
		subscriberID,
		NewBroadcastConsumer(NewSPSCRing[datura.Artifact](
			128, bg.dropOldestOnFull,
		), callback),
	); ok {
		errnie.Error(errnie.Err(
			errnie.Conflict,
			"subscriber already exists",
			nil,
		))

		return nil
	}

	return existing.(*BroadcastConsumer)
}

/*
Release releases a consumer ring.
*/
func (bg *BroadcastGroup) Release(subscriberID string) error {
	var (
		existing any
		ok       bool
	)

	if existing, ok = bg.consumers.LoadAndDelete(subscriberID); !ok {
		return errnie.Err(
			errnie.NotFound,
			"subscriber not found",
			nil,
		)
	}

	return errnie.Error(existing.(*BroadcastConsumer).ring.Close())

}

/*
Poll returns the next artifact when one is available.
*/
func (bg *BroadcastGroup) Poll(subscriberID string) (*datura.Artifact, bool) {
	var (
		existing any
		ok       bool
	)

	if existing, ok = bg.consumers.Load(subscriberID); !ok {
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"subscriber not found",
			nil,
		))

		return nil, false
	}

	value := existing.(*BroadcastConsumer).Poll()
	return value, value != nil
}

/*
Send delivers artifact to consumers honoring filters and routing rules.
*/
func (bg *BroadcastGroup) Send(artifact *datura.Artifact) error {
	if artifact == nil {
		return errnie.Err(
			errnie.Validation,
			"artifact is nil",
			nil,
		)
	}

	select {
	case <-bg.ctx.Done():
		return errnie.Err(
			errnie.IO,
			"broadcast group context is done",
			nil,
		)
	default:
	}

	var (
		destination string
		err         error
		existing    any
		ok          bool
	)

	if destination, err = artifact.Destination(); err != nil || destination == "" {
		if existing, ok = bg.consumers.Load(destination); !ok {
			return errnie.Err(
				errnie.NotFound,
				"subscriber not found",
				nil,
			)
		}

		consumer := existing.(*BroadcastConsumer)

		if consumer.callback == nil {
			consumer.ring.Push(artifact)
			consumer.wake()
			return nil
		}

		consumer.callback(artifact)
		return nil
	}

	bg.consumers.Range(func(key, value any) bool {
		consumer := value.(*BroadcastConsumer)

		if consumer.callback == nil {
			consumer.ring.Push(artifact)
			consumer.wake()
			return true
		}

		if err := consumer.callback(artifact); err != nil {
			errnie.Error(err)
			return true
		}

		return true
	})

	return nil
}

/*
Close stops the group and releases subscriber rings.
*/
func (bg *BroadcastGroup) Close() error {
	select {
	case <-bg.ctx.Done():
		return errnie.Err(
			errnie.IO,
			"broadcast group context is done",
			nil,
		)
	default:
	}

	bg.cancel()

	bg.consumers.Range(func(key, value any) bool {
		value.(*BroadcastConsumer).ring.Close()
		return true
	})

	return nil
}
