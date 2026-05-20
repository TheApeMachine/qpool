package qpool

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

/*
QSpace stores job results, waits, broadcast groups, and dependency edges.
*/
type QSpace struct {
	valuesMu sync.RWMutex
	values   map[string]*QValue
	waiting  map[string][]chan *QValue

	groupsMu sync.RWMutex
	groups   map[string]*BroadcastGroup

	graphMu  sync.RWMutex
	children map[string]map[string]struct{}
	parents  map[string]map[string]struct{}

	cleanupInterval time.Duration
	shutdown        chan struct{}
	done            chan struct{}
	stopped         atomic.Bool
}

/*
NewQSpace starts the expiration loop.
*/
func NewQSpace() *QSpace {
	qspace := &QSpace{
		values:          make(map[string]*QValue),
		waiting:         make(map[string][]chan *QValue),
		groups:          make(map[string]*BroadcastGroup),
		children:        make(map[string]map[string]struct{}),
		parents:         make(map[string]map[string]struct{}),
		cleanupInterval: time.Minute,
		shutdown:        make(chan struct{}),
		done:            make(chan struct{}),
	}

	go qspace.loop()

	return qspace
}

func (qs *QSpace) loop() {
	defer close(qs.done)

	ticker := time.NewTicker(qs.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			qs.cleanup(time.Now())

		case <-qs.shutdown:
			qs.shutdownState()

			return
		}
	}
}

/*
Store persists a completed value and fulfills waiters.
*/
func (qs *QSpace) Store(id string, value interface{}, ttl time.Duration) {
	if qs.stopped.Load() {
		return
	}

	qvalue := NewQValue(value)
	qvalue.TTL = ttl

	qs.valuesMu.Lock()
	defer qs.valuesMu.Unlock()

	if qs.stopped.Load() {
		return
	}

	qs.values[id] = qvalue
	waiters := qs.waiting[id]
	delete(qs.waiting, id)

	qspaceDeliverWaiters(waiters, qvalue)
}

/*
Await returns a channel that receives exactly one *QValue for id.
*/
func (qs *QSpace) Await(id string) chan *QValue {
	if qs.stopped.Load() {
		return closedQValueChannel()
	}

	resultChannel := make(chan *QValue, 1)

	qs.valuesMu.Lock()
	defer qs.valuesMu.Unlock()

	if qs.stopped.Load() {
		close(resultChannel)

		return resultChannel
	}

	if qvalue, ok := qs.values[id]; ok {
		resultChannel <- qvalue
		close(resultChannel)

		return resultChannel
	}

	qs.waiting[id] = append(qs.waiting[id], resultChannel)

	return resultChannel
}

/*
PeekResult returns (nil, false) when id has no stored completion, when the entry was removed by TTL cleanup, or when the space is stopped.

When ok is true, it returns a shallow copy of the stored QValue. Fields such as QValue.Value and QValue.Error may still reference shared underlying objects, so callers must treat the returned value as read-only.
*/
func (qs *QSpace) PeekResult(id string) (*QValue, bool) {
	qs.valuesMu.RLock()
	defer qs.valuesMu.RUnlock()

	if qs.stopped.Load() {
		return nil, false
	}

	value, ok := qs.values[id]
	if !ok {
		return nil, false
	}

	copied := *value

	return &copied, true
}

/*
Exists reports whether id currently has a stored value.
*/
func (qs *QSpace) Exists(id string) bool {
	qs.valuesMu.RLock()
	defer qs.valuesMu.RUnlock()

	if qs.stopped.Load() {
		return false
	}

	_, ok := qs.values[id]

	return ok
}

/*
StoreError stores a terminal error result for id.
*/
func (qs *QSpace) StoreError(id string, err error, ttl time.Duration) {
	if qs.stopped.Load() {
		return
	}

	qvalue := NewQValue(nil)
	qvalue.Error = err
	qvalue.TTL = ttl

	qs.valuesMu.Lock()
	defer qs.valuesMu.Unlock()

	if qs.stopped.Load() {
		return
	}

	qs.values[id] = qvalue
	waiters := qs.waiting[id]
	delete(qs.waiting, id)

	qspaceDeliverWaiters(waiters, qvalue)
}

/*
AddRelationship records a dependency edge parent -> child.
*/
func (qs *QSpace) AddRelationship(parentID, childID string) error {
	if qs.stopped.Load() {
		return fmt.Errorf("qpool: space closed")
	}

	qs.graphMu.Lock()
	defer qs.graphMu.Unlock()

	if qs.stopped.Load() {
		return fmt.Errorf("qpool: space closed")
	}

	if qspaceWouldCreateCircle(qs.children, parentID, childID) {
		return fmt.Errorf("qpool: circular dependency detected")
	}

	qspaceAddEdge(qs.children, qs.parents, parentID, childID)

	return nil
}

/*
RegisterDependent records that jobID waits on depID when dependency polling fails.
*/
func (qs *QSpace) RegisterDependent(depID, jobID string) {
	if qs.stopped.Load() {
		return
	}

	qs.graphMu.Lock()
	defer qs.graphMu.Unlock()

	if qs.stopped.Load() {
		return
	}

	qspaceAddEdge(qs.children, qs.parents, depID, jobID)
}

/*
CreateBroadcastGroup registers a pub/sub group owned by this space.
*/
func (qs *QSpace) CreateBroadcastGroup(id string, ttl time.Duration) *BroadcastGroup {
	group := NewBroadcastGroup(id, ttl, 100)

	if qs.stopped.Load() {
		group.Close()

		return group
	}

	qs.groupsMu.Lock()
	defer qs.groupsMu.Unlock()

	if qs.stopped.Load() {
		group.Close()

		return group
	}

	qs.groups[id] = group

	return group
}

/*
Subscribe attaches to a broadcast group by id.
*/
func (qs *QSpace) Subscribe(groupID string) chan *QValue {
	if qs.stopped.Load() {
		return closedQValueChannel()
	}

	qs.groupsMu.RLock()
	group := qs.groups[groupID]
	qs.groupsMu.RUnlock()

	if group == nil {
		return closedQValueChannel()
	}

	resultChannel := group.Subscribe("", 10)
	if resultChannel == nil {
		return closedQValueChannel()
	}

	return resultChannel
}

/*
Close stops maintenance and releases channels.
*/
func (qs *QSpace) Close() {
	if qs.stopped.Swap(true) {
		return
	}

	close(qs.shutdown)
	<-qs.done
}

func (qs *QSpace) cleanup(now time.Time) {
	expiredValues := qs.cleanupValues(now)
	qs.cleanupBroadcastGroups(now)

	if len(expiredValues) == 0 {
		return
	}

	qs.graphMu.Lock()
	defer qs.graphMu.Unlock()

	for _, id := range expiredValues {
		qspacePruneEdges(qs.children, qs.parents, id)
	}
}

func (qs *QSpace) cleanupValues(now time.Time) []string {
	qs.valuesMu.Lock()
	defer qs.valuesMu.Unlock()

	if qs.stopped.Load() {
		return nil
	}

	expired := make([]string, 0)

	for id, qvalue := range qs.values {
		if qvalue.TTL <= 0 {
			continue
		}

		if now.Sub(qvalue.CreatedAt) <= qvalue.TTL {
			continue
		}

		delete(qs.values, id)
		expired = append(expired, id)
	}

	return expired
}

func (qs *QSpace) cleanupBroadcastGroups(now time.Time) {
	qs.groupsMu.Lock()
	defer qs.groupsMu.Unlock()

	if qs.stopped.Load() {
		return
	}

	for id, group := range qs.groups {
		if group == nil {
			continue
		}

		if !group.Expired(now) {
			continue
		}

		group.Close()
		delete(qs.groups, id)
	}
}

func (qs *QSpace) shutdownState() {
	qs.valuesMu.Lock()

	for _, waiters := range qs.waiting {
		for _, resultChannel := range waiters {
			close(resultChannel)
		}
	}

	qs.waiting = nil
	qs.values = nil
	qs.valuesMu.Unlock()

	qs.groupsMu.Lock()

	for _, group := range qs.groups {
		if group != nil {
			group.Close()
		}
	}

	qs.groups = nil
	qs.groupsMu.Unlock()

	qs.graphMu.Lock()
	qs.children = nil
	qs.parents = nil
	qs.graphMu.Unlock()
}

func closedQValueChannel() chan *QValue {
	resultChannel := make(chan *QValue)

	close(resultChannel)

	return resultChannel
}

func qspaceDeliverWaiters(waiters []chan *QValue, qvalue *QValue) {
	for _, resultChannel := range waiters {
		select {
		case resultChannel <- qvalue:
		default:
		}

		close(resultChannel)
	}
}

func qspaceAddEdge(
	children map[string]map[string]struct{},
	parents map[string]map[string]struct{},
	parentID string,
	childID string,
) {
	if children[parentID] == nil {
		children[parentID] = make(map[string]struct{})
	}

	children[parentID][childID] = struct{}{}

	if parents[childID] == nil {
		parents[childID] = make(map[string]struct{})
	}

	parents[childID][parentID] = struct{}{}
}

func qspacePruneEdges(
	children map[string]map[string]struct{},
	parents map[string]map[string]struct{},
	id string,
) {
	delete(children, id)

	for parentID, childSet := range children {
		delete(childSet, id)

		if len(childSet) == 0 {
			delete(children, parentID)
		}
	}

	delete(parents, id)

	for childID, parentSet := range parents {
		delete(parentSet, id)

		if len(parentSet) == 0 {
			delete(parents, childID)
		}
	}
}

func qspaceWouldCreateCircle(
	children map[string]map[string]struct{},
	parentID string,
	childID string,
) bool {
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

		for nextChildID := range children[currentID] {
			stack = append(stack, nextChildID)
		}
	}

	return false
}
