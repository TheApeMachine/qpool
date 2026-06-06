package qpool

import (
	"hash/fnv"
	"sync/atomic"
)

const registryShardCount = 256

type registryShard struct {
	head atomic.Pointer[registryEntry]
}

type registryEntry struct {
	keyHash  uint64
	key      string
	value    atomic.Pointer[resultSlot]
	stored   atomic.Pointer[QValue[erasedAny]]
	children atomic.Pointer[depEdge]
	parents  atomic.Pointer[depEdge]
	next     atomic.Pointer[registryEntry]
}

type lockfreeRegistry struct {
	shards [registryShardCount]registryShard
}

func newLockfreeRegistry() *lockfreeRegistry {
	return &lockfreeRegistry{}
}

func registryShardIndex(key string) uint64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(key))

	return hasher.Sum64() % registryShardCount
}

func (registry *lockfreeRegistry) find(key string) *registryEntry {
	if registry == nil {
		return nil
	}

	shard := &registry.shards[registryShardIndex(key)]
	keyHash := fnvHash(key)

	for entry := shard.head.Load(); entry != nil; entry = entry.next.Load() {
		if entry.keyHash == keyHash && entry.key == key {
			return entry
		}
	}

	return nil
}

func (registry *lockfreeRegistry) getOrCreate(key string) *registryEntry {
	if registry == nil {
		return nil
	}

	if entry := registry.find(key); entry != nil {
		return entry
	}

	shard := &registry.shards[registryShardIndex(key)]
	keyHash := fnvHash(key)

	newEntry := &registryEntry{
		keyHash: keyHash,
		key:     key,
	}

	newEntry.value.Store(newResultSlot())

	for {
		head := shard.head.Load()
		newEntry.next.Store(head)

		if shard.head.CompareAndSwap(head, newEntry) {
			if existing := registry.find(key); existing != nil && existing != newEntry {
				return existing
			}

			return newEntry
		}

		if entry := registry.find(key); entry != nil {
			return entry
		}
	}
}

func (registry *lockfreeRegistry) removeExpired(key string) {
	if registry == nil {
		return
	}

	shard := &registry.shards[registryShardIndex(key)]
	keyHash := fnvHash(key)

	for {
		prev := (*registryEntry)(nil)
		entry := shard.head.Load()

		for entry != nil {
			next := entry.next.Load()

			if entry.keyHash == keyHash && entry.key == key {
				if prev == nil {
					if !shard.head.CompareAndSwap(entry, next) {
						break
					}

					return
				}

				prev.next.Store(next)

				return
			}

			prev = entry
			entry = next
		}

		return
	}
}

func (registry *lockfreeRegistry) closeAll() {
	if registry == nil {
		return
	}

	for shardIndex := range registry.shards {
		shard := &registry.shards[shardIndex]

		for entry := shard.head.Load(); entry != nil; entry = entry.next.Load() {
			if slot := entry.value.Load(); slot != nil {
				slot.Close()
			}
		}
	}
}

func fnvHash(key string) uint64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(key))

	return hasher.Sum64()
}
