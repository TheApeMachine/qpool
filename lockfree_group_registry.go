package qpool

import (
	"hash/fnv"
	"sync/atomic"
)

type groupRegistryEntry struct {
	keyHash uint64
	key     string
	group   atomic.Pointer[BroadcastGroup]
	next    atomic.Pointer[groupRegistryEntry]
}

type lockfreeGroupRegistry struct {
	shards [registryShardCount]struct {
		head atomic.Pointer[groupRegistryEntry]
	}
}

func (registry *lockfreeGroupRegistry) store(key string, group *BroadcastGroup) {
	if registry == nil || group == nil {
		return
	}

	shard := &registry.shards[groupShardIndex(key)]
	keyHash := fnvHash(key)

	entry := &groupRegistryEntry{
		keyHash: keyHash,
		key:     key,
	}

	entry.group.Store(group)

	for {
		head := shard.head.Load()
		entry.next.Store(head)

		if shard.head.CompareAndSwap(head, entry) {
			return
		}
	}
}

func (registry *lockfreeGroupRegistry) load(key string) *BroadcastGroup {
	if registry == nil {
		return nil
	}

	shard := &registry.shards[groupShardIndex(key)]
	keyHash := fnvHash(key)

	for entry := shard.head.Load(); entry != nil; entry = entry.next.Load() {
		if entry.keyHash == keyHash && entry.key == key {
			return entry.group.Load()
		}
	}

	return nil
}

func (registry *lockfreeGroupRegistry) closeAll() {
	if registry == nil {
		return
	}

	for shardIndex := range registry.shards {
		shard := &registry.shards[shardIndex]

		for entry := shard.head.Load(); entry != nil; entry = entry.next.Load() {
			if group := entry.group.Load(); group != nil {
				group.Close()
			}
		}
	}
}

func groupShardIndex(key string) uint64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(key))

	return hasher.Sum64() % registryShardCount
}
