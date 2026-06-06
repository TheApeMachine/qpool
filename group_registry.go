package qpool

import (
	"hash/fnv"
	"sync/atomic"
)

type GroupRegistryEntry struct {
	keyHash uint64
	key     string
	group   atomic.Pointer[BroadcastGroup]
	next    atomic.Pointer[GroupRegistryEntry]
}

type GroupRegistry struct {
	shards [registryShardCount]struct {
		head atomic.Pointer[GroupRegistryEntry]
	}
}

func (registry *GroupRegistry) store(key string, group *BroadcastGroup) {
	if registry == nil || group == nil {
		return
	}

	shard := &registry.shards[groupShardIndex(key)]
	keyHash := fnvHash(key)

	entry := &GroupRegistryEntry{
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

func (registry *GroupRegistry) load(key string) *BroadcastGroup {
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

func (registry *GroupRegistry) closeAll() {
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
