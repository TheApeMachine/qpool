package qpool

import "sync/atomic"

type GroupRegistryEntry struct {
	keyHash uint64
	key     string
	group   atomic.Pointer[BroadcastGroup]
	next    atomic.Pointer[GroupRegistryEntry]
}

type groupRegistryShard struct {
	entries IntrusiveList[GroupRegistryEntry]
}

type GroupRegistry struct {
	shards [registryShardCount]groupRegistryShard
}

func (registry *GroupRegistry) init() {
	for shardIndex := range registry.shards {
		registry.shards[shardIndex].entries.bind(
			func(entry *GroupRegistryEntry) *GroupRegistryEntry {
				return entry.next.Load()
			},
			func(entry, next *GroupRegistryEntry) {
				entry.next.Store(next)
			},
			func(prev, current, next *GroupRegistryEntry) bool {
				return prev.next.CompareAndSwap(current, next)
			},
		)
	}
}

func (registry *GroupRegistry) store(key string, group *BroadcastGroup) {
	if registry == nil || group == nil {
		return
	}

	keyHash := keyIndexer{}.hash(key)
	shard := &registry.shards[keyIndexer{}.shardFromHash(keyHash)]

	entry := &GroupRegistryEntry{
		keyHash: keyHash,
		key:     key,
	}

	entry.group.Store(group)

	shard.entries.Prepend(entry)
}

func (registry *GroupRegistry) load(key string) *BroadcastGroup {
	if registry == nil {
		return nil
	}

	keyHash := keyIndexer{}.hash(key)
	shard := &registry.shards[keyIndexer{}.shardFromHash(keyHash)]

	entry := shard.entries.Find(func(entry *GroupRegistryEntry) bool {
		return entry.keyHash == keyHash && entry.key == key
	})

	if entry == nil {
		return nil
	}

	return entry.group.Load()
}

func (registry *GroupRegistry) closeAll() {
	if registry == nil {
		return
	}

	for shardIndex := range registry.shards {
		registry.shards[shardIndex].entries.Walk(func(entry *GroupRegistryEntry) {
			if group := entry.group.Load(); group != nil {
				group.Close()
			}
		})
	}
}
