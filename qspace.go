package qpool

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/datura"
)

/*
QSpace stores job results, waits, broadcast groups, and dependency edges.
*/
type QSpace struct {
	ID              string
	ctx             context.Context
	cancel          context.CancelFunc
	entries         Registry
	groups          sync.Map
	stopped         atomic.Bool
	cleanupInterval time.Duration
	maintDone       atomic.Bool
}

/*
NewQSpace starts the expiration loop.
*/
func NewQSpace(ctx context.Context) *QSpace {
	ctx, cancel := context.WithCancel(context.Background())

	qspace := &QSpace{
		ID:              uuid.New().String(),
		ctx:             ctx,
		cancel:          cancel,
		cleanupInterval: time.Minute,
		entries:         *NewRegistry(),
	}

	go qspace.loop()
	return qspace
}

func (qspace *QSpace) loop() {
	defer qspace.maintDone.Store(true)

	for !qspace.stopped.Load() {
		deadline := time.Now().Add(qspace.cleanupInterval)

		for time.Now().Before(deadline) && !qspace.stopped.Load() {
			time.Sleep(10 * time.Millisecond)
		}

		if qspace.stopped.Load() {
			return
		}

		qspace.cleanup(time.Now())
	}
}

/*
Store persists a completed value and fulfills waiters.
*/
func (qspace *QSpace) Store(id string, value any, ttl time.Duration) {
	if qspace.stopped.Load() {
		return
	}

	artifact, err := newResultArtifact(id, value, ttl)

	if err != nil {
		return
	}

	entry := qspace.entries.getOrCreate(id)

	if entry == nil || qspace.stopped.Load() {
		return
	}

	entry.stored.Store(artifact)

	if slot := entry.value.Load(); slot != nil {
		slot.Deliver(artifact)
	}
}

/*
Await registers a lock-free waiter for id.
*/
func (qspace *QSpace) Await(id string) *ResultWait[erasedAny] {
	if qspace.stopped.Load() {
		return errorResultWait[erasedAny](errResultClosed)
	}

	entry := qspace.entries.getOrCreate(id)

	if entry == nil || qspace.stopped.Load() {
		return errorResultWait[erasedAny](errResultClosed)
	}

	if stored := entry.stored.Load(); stored != nil {
		return readyResultWait[erasedAny](stored)
	}

	slot := entry.value.Load()

	if slot == nil {
		return errorResultWait[erasedAny](errResultClosed)
	}

	if slot.state.Load() == slotReady {
		if stored := entry.stored.Load(); stored != nil {
			return readyResultWait[erasedAny](stored)
		}
	}

	return pendingResultWait[erasedAny](slot)
}

/*
PeekResult returns (nil, false) when id has no stored completion, when
the entry was removed by TTL cleanup, or when the space is stopped.
*/
func (qspace *QSpace) PeekResult(id string) (*datura.Artifact, bool) {
	if qspace.stopped.Load() {
		return nil, false
	}

	entry := qspace.entries.find(id)

	if entry == nil {
		return nil, false
	}

	value := entry.stored.Load()

	if value == nil {
		return nil, false
	}

	return cloneArtifact(value), true
}

/*
Exists reports whether id currently has a stored value.
*/
func (qspace *QSpace) Exists(id string) bool {
	if qspace.stopped.Load() {
		return false
	}

	entry := qspace.entries.find(id)

	return entry != nil && entry.stored.Load() != nil
}

/*
StoreError stores a terminal error result for id.
*/
func (qspace *QSpace) StoreError(id string, terminalErr error, ttl time.Duration) {
	if qspace.stopped.Load() {
		return
	}

	artifact, err := newErrorArtifact(id, terminalErr, ttl)

	if err != nil {
		return
	}

	entry := qspace.entries.getOrCreate(id)

	if entry == nil || qspace.stopped.Load() {
		return
	}

	entry.stored.Store(artifact)

	if slot := entry.value.Load(); slot != nil {
		slot.Deliver(artifact)
	}
}

/*
AddRelationship records a dependency edge parent -> child.
*/
func (qspace *QSpace) AddRelationship(parentID, childID string) error {
	if qspace.stopped.Load() {
		return fmt.Errorf("qpool: space closed")
	}

	return qspace.entries.addRelationship(parentID, childID)
}

/*
RegisterDependent records that jobID waits on depID when dependency polling fails.
*/
func (qspace *QSpace) RegisterDependent(depID, jobID string) {
	if qspace.stopped.Load() {
		return
	}

	qspace.entries.registerDependent(depID, jobID)
}

/*
CreateBroadcastGroup registers a pub/sub group owned by this space.
*/
func (qspace *QSpace) CreateBroadcastGroup(id string) *BroadcastGroup {
	stored, _ := qspace.groups.LoadOrStore(
		id, NewBroadcastGroup(qspace.ctx, id, time.Minute),
	)

	return stored.(*BroadcastGroup)
}

/*
Subscribe attaches to a broadcast group by id.
*/
func (qspace *QSpace) Subscribe(
	groupID string, callback func(*datura.Artifact) error,
) (consumer *BroadcastConsumer) {
	return qspace.CreateBroadcastGroup(groupID).Acquire(
		uuid.New().String(), callback,
	)
}

/*
Close stops maintenance and releases waiters.
*/
func (qspace *QSpace) Close() {
	if qspace.stopped.Swap(true) {
		return
	}

	for !qspace.maintDone.Load() {
		time.Sleep(time.Millisecond)
	}

	qspace.entries.closeAll()

	qspace.groups.Range(func(key, value any) bool {
		value.(*BroadcastGroup).Close()
		return true
	})
}

func (qspace *QSpace) cleanup(now time.Time) {
	for shardIndex := range qspace.entries.shards {
		qspace.entries.shards[shardIndex].entries.Walk(func(
			entry *RegistryEntry,
		) {
			value := entry.stored.Load()

			if value == nil {
				return
			}

			ttl := artifactTTL(value)

			if ttl <= 0 {
				return
			}

			if now.Sub(
				time.Unix(0, value.Timestamp()),
			) <= ttl || !entry.stored.CompareAndSwap(value, nil) {
				return
			}

			qspace.entries.removeExpired(entry.key)
			qspace.entries.pruneDependencyEdges(entry.key)
		})
	}
}
