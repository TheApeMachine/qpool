package qpool

import (
	"log"
	"sync"
	"time"
)

// BroadcastGroup handles pub/sub
type BroadcastGroup struct {
	ID       string
	channels []chan QuantumValue
	TTL      time.Duration
	LastUsed time.Time
}

// QuantumValue wraps a value with metadata
type QuantumValue struct {
	Value     any
	Error     error
	CreatedAt time.Time
	TTL       time.Duration
}

// QuantumSpace handles value storage and messaging
type QuantumSpace struct {
	mu       sync.RWMutex
	values   map[string]QuantumValue
	waiting  map[string][]chan QuantumValue
	groups   map[string]*BroadcastGroup
	errors   map[string]error
	children map[string][]string
	wg       sync.WaitGroup
}

func newQuantumSpace() *QuantumSpace {
	qs := &QuantumSpace{
		values:  make(map[string]QuantumValue),
		waiting: make(map[string][]chan QuantumValue),
		groups:  make(map[string]*BroadcastGroup),
		mu:      sync.RWMutex{},
	}

	// Start cleanup goroutine
	qs.wg.Add(1)
	go func() {
		defer qs.wg.Done()
		qs.cleanup()
	}()

	return qs
}

// Store stores a value with its metadata
func (qs *QuantumSpace) Store(id string, value any, err error, ttl time.Duration) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	qv := QuantumValue{
		Value:     value,
		Error:     err,
		CreatedAt: time.Now(),
		TTL:       ttl,
	}
	qs.values[id] = qv
	log.Printf("Storing value for job %s: value=%v, err=%v", id, value, err)

	// Notify any waiting channels
	if channels, ok := qs.waiting[id]; ok {
		log.Printf("Found %d waiting channels for job %s", len(channels), id)
		for i, ch := range channels {
			select {
			case ch <- qv:
				log.Printf("Successfully sent result to channel %d for job %s", i, id)
				close(ch)
			default:
				log.Printf("Failed to send result to channel %d for job %s (channel full or closed)", i, id)
			}
		}
		delete(qs.waiting, id)
		log.Printf("Cleared waiting channels for job %s", id)
	} else {
		log.Printf("No waiting channels found for job %s", id)
	}
}

// Await returns a channel that will receive the value when it's available
func (qs *QuantumSpace) Await(id string) chan QuantumValue {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	ch := make(chan QuantumValue, 1)
	log.Printf("Created await channel for job %s", id)

	// Check if value already exists
	if qv, ok := qs.values[id]; ok {
		log.Printf("Value already exists for job %s, sending immediately", id)
		ch <- qv
		close(ch)
		return ch
	}

	// Add to waiting list
	if qs.waiting == nil {
		qs.waiting = make(map[string][]chan QuantumValue)
	}
	qs.waiting[id] = append(qs.waiting[id], ch)
	log.Printf("Added channel to waiting list for job %s (total waiting: %d)",
		id, len(qs.waiting[id]))

	return ch
}

func (qs *QuantumSpace) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		qs.mu.Lock()
		qs.cleanupExpiredValues()
		qs.cleanupExpiredGroups()
		qs.mu.Unlock()
	}
}

func (qs *QuantumSpace) cleanupExpiredValues() {
	now := time.Now()
	for id, qv := range qs.values {
		if qv.TTL > 0 && now.Sub(qv.CreatedAt) > qv.TTL {
			delete(qs.values, id)
		}
	}
}

func (qs *QuantumSpace) cleanupExpiredGroups() {
	now := time.Now()
	for id, group := range qs.groups {
		if group.TTL > 0 && now.Sub(group.LastUsed) > group.TTL {
			for _, ch := range group.channels {
				close(ch)
			}
			delete(qs.groups, id)
		}
	}
}

func (qs *QuantumSpace) CreateBroadcastGroup(id string, ttl time.Duration) *BroadcastGroup {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	group := &BroadcastGroup{
		ID:       id,
		channels: make([]chan QuantumValue, 0),
		TTL:      ttl,
		LastUsed: time.Now(),
	}
	qs.groups[id] = group
	return group
}

func (bg *BroadcastGroup) Send(qv QuantumValue) {
	bg.LastUsed = time.Now()
	for _, ch := range bg.channels {
		ch <- qv
	}
}

func (qs *QuantumSpace) Subscribe(groupID string) chan QuantumValue {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	ch := make(chan QuantumValue, 10)
	if group, ok := qs.groups[groupID]; ok {
		group.channels = append(group.channels, ch)
	}
	return ch
}

func (qs *QuantumSpace) Close() {
	qs.CleanUp()
	// Add any additional cleanup if necessary
}

func (qs *QuantumSpace) CleanUp() {
	qs.mu.Lock()
	defer qs.mu.Unlock()
	now := time.Now()
	for id, qv := range qs.values {
		if now.Sub(qv.CreatedAt) > qv.TTL {
			delete(qs.values, id)
		}
	}
}

func (qs *QuantumSpace) StoreError(id string, err error) {
	qs.mu.Lock()
	defer qs.mu.Unlock()
	if qs.errors == nil {
		qs.errors = make(map[string]error)
	}
	qs.errors[id] = err
}

func (qs *QuantumSpace) AddChild(parentID, childID string) {
	qs.mu.Lock()
	defer qs.mu.Unlock()
	if qs.children == nil {
		qs.children = make(map[string][]string)
	}
	qs.children[parentID] = append(qs.children[parentID], childID)
}

func (qs *QuantumSpace) GetChildren(parentID string) []string {
	qs.mu.RLock()
	defer qs.mu.RUnlock()
	return qs.children[parentID]
}
