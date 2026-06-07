package qpool

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

/*
QSpace stores job results, waits, broadcast groups, and dependency edges.
*/
type QSpace struct {
	entries         Registry
	groups          GroupRegistry
	stopped         atomic.Bool
	cleanupInterval time.Duration
	maintDone       atomic.Bool
}

/*
NewQSpace starts the expiration loop.
*/
func NewQSpace() *QSpace {
	qspace := &QSpace{
		cleanupInterval: time.Minute,
		entries:         *NewRegistry(),
	}

	qspace.groups.init()

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
func (qspace *QSpace) Store(id string, value interface{}, ttl time.Duration) {
	if qspace.stopped.Load() {
		return
	}

	qvalue, err := NewQValue("", "", value, ttl)

	if err != nil {
		return
	}

	qvalue.TTL = ttl

	entry := qspace.entries.getOrCreate(id)

	if entry == nil || qspace.stopped.Load() {
		return
	}

	entry.stored.Store(qvalue)

	if slot := entry.value.Load(); slot != nil {
		slot.Deliver(qvalue)
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
func (qspace *QSpace) PeekResult(id string) (*QValue[erasedAny], bool) {
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

	copied := *value

	return &copied, true
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
func (qspace *QSpace) StoreError(id string, err error, ttl time.Duration) {
	if qspace.stopped.Load() {
		return
	}

	qvalue, qvalueErr := NewQValue[erasedAny]("", "", nil, 0)
	if qvalueErr != nil {
		return
	}

	qvalue.Error = err
	qvalue.TTL = ttl

	entry := qspace.entries.getOrCreate(id)
	if entry == nil || qspace.stopped.Load() {
		return
	}

	entry.stored.Store(qvalue)

	if slot := entry.value.Load(); slot != nil {
		slot.Deliver(qvalue)
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
func (qspace *QSpace) CreateBroadcastGroup(id string, ttl time.Duration) *BroadcastGroup {
	if existing := qspace.groups.load(id); existing != nil {
		return existing
	}

	group, err := NewBroadcastGroup(context.Background(), id, ttl)

	if err != nil {
		return nil
	}

	if qspace.stopped.Load() {
		group.Close()

		return nil
	}

	qspace.groups.store(id, group)

	return group
}

/*
Subscribe attaches to a broadcast group by id.
*/
func (qspace *QSpace) Subscribe(groupID string) *BroadcastConsumer {
	if qspace.stopped.Load() {
		return nil
	}

	group := qspace.groups.load(groupID)

	if group == nil {
		return nil
	}

	return group.Subscribe("", 10)
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
	qspace.groups.closeAll()
}

func (qspace *QSpace) cleanup(now time.Time) {
	for shardIndex := range qspace.entries.shards {
		qspace.entries.shards[shardIndex].entries.Walk(func(entry *RegistryEntry) {
			value := entry.stored.Load()

			if value == nil || value.TTL <= 0 || now.Sub(time.Unix(0, value.CreatedAt)) <= value.TTL {
				return
			}

			if !entry.stored.CompareAndSwap(value, nil) {
				return
			}

			qspace.entries.removeExpired(entry.key)
			qspace.entries.pruneDependencyEdges(entry.key)
		})
	}
}
