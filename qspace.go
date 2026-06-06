package qpool

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

type depEdge struct {
	id   string
	next atomic.Pointer[depEdge]
}

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
	}

	go qspace.loop()

	return qspace
}

func (qs *QSpace) loop() {
	defer qs.maintDone.Store(true)

	for !qs.stopped.Load() {
		deadline := time.Now().Add(qs.cleanupInterval)

		for time.Now().Before(deadline) && !qs.stopped.Load() {
			time.Sleep(10 * time.Millisecond)
		}

		if qs.stopped.Load() {
			return
		}

		qs.cleanup(time.Now())
	}
}

/*
Store persists a completed value and fulfills waiters.
*/
func (qs *QSpace) Store(id string, value interface{}, ttl time.Duration) {
	if qs.stopped.Load() {
		return
	}

	qvalue, err := NewQValue("", "", value, ttl)
	if err != nil {
		return
	}

	qvalue.TTL = ttl

	entry := qs.entries.getOrCreate(id)
	if entry == nil || qs.stopped.Load() {
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
func (qs *QSpace) Await(id string) *ResultWait[erasedAny] {
	if qs.stopped.Load() {
		return errorResultWait[erasedAny](errResultClosed)
	}

	entry := qs.entries.getOrCreate(id)
	if entry == nil || qs.stopped.Load() {
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
PeekResult returns (nil, false) when id has no stored completion, when the entry was removed by TTL cleanup, or when the space is stopped.
*/
func (qs *QSpace) PeekResult(id string) (*QValue[erasedAny], bool) {
	if qs.stopped.Load() {
		return nil, false
	}

	entry := qs.entries.find(id)
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
func (qs *QSpace) Exists(id string) bool {
	if qs.stopped.Load() {
		return false
	}

	entry := qs.entries.find(id)

	return entry != nil && entry.stored.Load() != nil
}

/*
StoreError stores a terminal error result for id.
*/
func (qs *QSpace) StoreError(id string, err error, ttl time.Duration) {
	if qs.stopped.Load() {
		return
	}

	qvalue, qvalueErr := NewQValue[erasedAny]("", "", nil, 0)
	if qvalueErr != nil {
		return
	}

	qvalue.Error = err
	qvalue.TTL = ttl

	entry := qs.entries.getOrCreate(id)
	if entry == nil || qs.stopped.Load() {
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
func (qs *QSpace) AddRelationship(parentID, childID string) error {
	if qs.stopped.Load() {
		return fmt.Errorf("qpool: space closed")
	}

	if wouldCreateCircle(&qs.entries, parentID, childID) {
		return fmt.Errorf("qpool: circular dependency detected")
	}

	addEdge(&qs.entries, parentID, childID)

	return nil
}

/*
RegisterDependent records that jobID waits on depID when dependency polling fails.
*/
func (qs *QSpace) RegisterDependent(depID, jobID string) {
	if qs.stopped.Load() {
		return
	}

	addEdge(&qs.entries, depID, jobID)
}

/*
CreateBroadcastGroup registers a pub/sub group owned by this space.
*/
func (qs *QSpace) CreateBroadcastGroup(id string, ttl time.Duration) *BroadcastGroup {
	group, err := NewBroadcastGroup(context.Background(), id, ttl)
	if err != nil {
		return nil
	}

	if qs.stopped.Load() {
		group.Close()

		return group
	}

	qs.groups.store(id, group)

	return group
}

/*
Subscribe attaches to a broadcast group by id.
*/
func (qs *QSpace) Subscribe(groupID string) *BroadcastConsumer {
	if qs.stopped.Load() {
		return nil
	}

	group := qs.groups.load(groupID)
	if group == nil {
		return nil
	}

	return group.Subscribe("", 10)
}

/*
Close stops maintenance and releases waiters.
*/
func (qs *QSpace) Close() {
	if qs.stopped.Swap(true) {
		return
	}

	for !qs.maintDone.Load() {
		time.Sleep(time.Millisecond)
	}

	qs.entries.closeAll()
	qs.groups.closeAll()
}

func (qs *QSpace) cleanup(now time.Time) {
	for shardIndex := range qs.entries.shards {
		shard := &qs.entries.shards[shardIndex]

		for entry := shard.head.Load(); entry != nil; entry = entry.next.Load() {
			value := entry.stored.Load()
			if value == nil {
				continue
			}

			if value.TTL <= 0 {
				continue
			}

			if now.Sub(time.Unix(0, value.CreatedAt)) <= value.TTL {
				continue
			}

			entry.stored.Store(nil)
			qs.entries.removeExpired(entry.key)
			pruneEdges(&qs.entries, entry.key)
		}
	}
}

func addEdge(registry *Registry, parentID, childID string) {
	parent := registry.getOrCreate(parentID)
	child := registry.getOrCreate(childID)

	if parent == nil || child == nil {
		return
	}

	depPush(&parent.children, childID)
	depPush(&child.parents, parentID)
}

func depPush(head *atomic.Pointer[depEdge], id string) {
	edge := &depEdge{id: id}

	for {
		current := head.Load()
		edge.next.Store(current)

		if head.CompareAndSwap(current, edge) {
			return
		}
	}
}

func pruneEdges(registry *Registry, id string) {
	entry := registry.find(id)
	if entry == nil {
		return
	}

	for edge := entry.children.Load(); edge != nil; edge = edge.next.Load() {
		if child := registry.find(edge.id); child != nil {
			depRemove(&child.parents, id)
		}
	}

	for edge := entry.parents.Load(); edge != nil; edge = edge.next.Load() {
		if parent := registry.find(edge.id); parent != nil {
			depRemove(&parent.children, id)
		}
	}

	entry.children.Store(nil)
	entry.parents.Store(nil)
}

func depRemove(head *atomic.Pointer[depEdge], id string) {
	for {
		prev := (*depEdge)(nil)
		current := head.Load()

		for current != nil {
			next := current.next.Load()

			if current.id == id {
				if prev == nil {
					if head.CompareAndSwap(current, next) {
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

func wouldCreateCircle(registry *Registry, parentID, childID string) bool {
	visited := make(map[string]struct{})
	stack := []string{childID}

	for len(stack) > 0 {
		lastIndex := len(stack) - 1
		currentID := stack[lastIndex]
		stack = stack[:lastIndex]

		if currentID == parentID {
			return true
		}

		if _, ok := visited[currentID]; ok {
			continue
		}

		visited[currentID] = struct{}{}

		entry := registry.find(currentID)
		if entry == nil {
			continue
		}

		for edge := entry.children.Load(); edge != nil; edge = edge.next.Load() {
			stack = append(stack, edge.id)
		}
	}

	return false
}
