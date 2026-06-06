package qpool

import (
	"fmt"
	"hash/fnv"
	"sync/atomic"
)

const registryShardCount = 256

type keyIndexer struct{}

func (indexer keyIndexer) hash(key string) uint64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(key))

	return hasher.Sum64()
}

func (indexer keyIndexer) shard(key string) uint64 {
	return indexer.hash(key) % registryShardCount
}

type RegistryShard struct {
	entries IntrusiveList[RegistryEntry]
}

type depEdge struct {
	id   string
	next atomic.Pointer[depEdge]
}

type depEdgeList struct {
	edges IntrusiveList[depEdge]
}

func newDepEdgeList() depEdgeList {
	list := depEdgeList{}
	list.edges.bind(
		func(edge *depEdge) *depEdge {
			return edge.next.Load()
		},
		func(edge, next *depEdge) {
			edge.next.Store(next)
		},
	)

	return list
}

func (list *depEdgeList) Push(id string) {
	list.edges.Prepend(&depEdge{id: id})
}

func (list *depEdgeList) Remove(id string) {
	list.edges.Remove(func(edge *depEdge) bool {
		return edge.id == id
	})
}

func (list *depEdgeList) Clear() {
	list.edges.Clear()
}

func (list *depEdgeList) Walk(visitor func(string)) {
	if visitor == nil {
		return
	}

	list.edges.Walk(func(edge *depEdge) {
		if edge != nil {
			visitor(edge.id)
		}
	})
}

type RegistryEntry struct {
	keyHash  uint64
	key      string
	value    atomic.Pointer[resultSlot]
	stored   atomic.Pointer[QValue[erasedAny]]
	children depEdgeList
	parents  depEdgeList
	next     atomic.Pointer[RegistryEntry]
}

type Registry struct {
	shards [registryShardCount]RegistryShard
}

func NewRegistry() *Registry {
	registry := &Registry{}

	for shardIndex := range registry.shards {
		registry.shards[shardIndex].entries.bind(
			func(entry *RegistryEntry) *RegistryEntry {
				return entry.next.Load()
			},
			func(entry, next *RegistryEntry) {
				entry.next.Store(next)
			},
		)
	}

	return registry
}

func (registry *Registry) find(key string) *RegistryEntry {
	if registry == nil {
		return nil
	}

	shard := &registry.shards[keyIndexer{}.shard(key)]
	keyHash := keyIndexer{}.hash(key)

	return shard.entries.Find(func(entry *RegistryEntry) bool {
		return entry.keyHash == keyHash && entry.key == key
	})
}

func (registry *Registry) getOrCreate(key string) *RegistryEntry {
	if registry == nil {
		return nil
	}

	if entry := registry.find(key); entry != nil {
		return entry
	}

	shard := &registry.shards[keyIndexer{}.shard(key)]
	keyHash := keyIndexer{}.hash(key)

	newEntry := &RegistryEntry{
		keyHash:  keyHash,
		key:      key,
		children: newDepEdgeList(),
		parents:  newDepEdgeList(),
	}

	newEntry.value.Store(newResultSlot())

	for {
		if entry := registry.find(key); entry != nil {
			return entry
		}

		if shard.entries.prependOnce(newEntry) {
			if existing := registry.find(key); existing != nil && existing != newEntry {
				return existing
			}

			return newEntry
		}
	}
}

func (registry *Registry) removeExpired(key string) {
	if registry == nil {
		return
	}

	shard := &registry.shards[keyIndexer{}.shard(key)]
	keyHash := keyIndexer{}.hash(key)

	shard.entries.Remove(func(entry *RegistryEntry) bool {
		return entry.keyHash == keyHash && entry.key == key
	})
}

func (registry *Registry) closeAll() {
	if registry == nil {
		return
	}

	for shardIndex := range registry.shards {
		registry.shards[shardIndex].entries.Walk(func(entry *RegistryEntry) {
			if slot := entry.value.Load(); slot != nil {
				slot.Close()
			}
		})
	}
}

func (registry *Registry) addRelationship(parentID, childID string) error {
	if registry.wouldCreateCircle(parentID, childID) {
		return fmt.Errorf("qpool: circular dependency detected")
	}

	parent := registry.getOrCreate(parentID)
	child := registry.getOrCreate(childID)

	if parent == nil || child == nil {
		return nil
	}

	parent.children.Push(childID)
	child.parents.Push(parentID)

	return nil
}

func (registry *Registry) registerDependent(depID, jobID string) {
	parent := registry.getOrCreate(depID)
	child := registry.getOrCreate(jobID)

	if parent == nil || child == nil {
		return
	}

	parent.children.Push(jobID)
	child.parents.Push(depID)
}

func (registry *Registry) pruneDependencyEdges(id string) {
	entry := registry.find(id)
	if entry == nil {
		return
	}

	entry.children.Walk(func(childID string) {
		if child := registry.find(childID); child != nil {
			child.parents.Remove(id)
		}
	})

	entry.parents.Walk(func(parentID string) {
		if parent := registry.find(parentID); parent != nil {
			parent.children.Remove(id)
		}
	})

	entry.children.Clear()
	entry.parents.Clear()
}

func (registry *Registry) wouldCreateCircle(parentID, childID string) bool {
	visited := make(map[string]struct{})
	stack := []string{childID}

	for len(stack) > 0 {
		lastIndex := len(stack) - 1
		currentID := stack[lastIndex]
		stack = stack[:lastIndex]

		if currentID == parentID {
			return true
		}

		if _, seen := visited[currentID]; seen {
			continue
		}

		visited[currentID] = struct{}{}

		entry := registry.find(currentID)
		if entry == nil {
			continue
		}

		entry.children.Walk(func(nextID string) {
			stack = append(stack, nextID)
		})
	}

	return false
}
