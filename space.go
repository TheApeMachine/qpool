package qpool

import (
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
	values   map[string]*QuantumValue
	waiting  map[string][]chan QuantumValue
	errors   map[string]error
	children map[string][]string
	groups   map[string]*BroadcastGroup
}

func newQuantumSpace() *QuantumSpace {
	qs := &QuantumSpace{
		values:   make(map[string]*QuantumValue),
		waiting:  make(map[string][]chan QuantumValue),
		errors:   make(map[string]error),
		children: make(map[string][]string),
		groups:   make(map[string]*BroadcastGroup),
	}

	go qs.cleanup()
	return qs
}

func (qs *QuantumSpace) Store(id string, value any, err error, ttl time.Duration) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	qv := &QuantumValue{
		Value:     value,
		Error:     err,
		CreatedAt: time.Now(),
		TTL:       ttl,
	}
	qs.values[id] = qv

	// Notify waiters
	if channels, ok := qs.waiting[id]; ok {
		for _, ch := range channels {
			ch <- *qv
			close(ch)
		}
		delete(qs.waiting, id)
	}
}

func (qs *QuantumSpace) Await(id string) chan QuantumValue {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	ch := make(chan QuantumValue, 1)
	if qv, ok := qs.values[id]; ok {
		ch <- *qv
		close(ch)
		return ch
	}

	qs.waiting[id] = append(qs.waiting[id], ch)
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
	qs.mu.Lock()
	defer qs.mu.Unlock()

	// Create a set of channels to close
	channelsToClose := make(map[chan QuantumValue]struct{})

	// Collect all channels
	for _, channels := range qs.waiting {
		for _, ch := range channels {
			channelsToClose[ch] = struct{}{}
		}
	}
	for _, group := range qs.groups {
		for _, ch := range group.channels {
			channelsToClose[ch] = struct{}{}
		}
	}

	// Close each channel exactly once with a timeout
	for ch := range channelsToClose {
		select {
		case <-ch: // Try to drain with timeout
		case <-time.After(100 * time.Millisecond):
		}
		close(ch)
	}

	// Clear all maps
	qs.values = make(map[string]*QuantumValue)
	qs.waiting = make(map[string][]chan QuantumValue)
	qs.groups = make(map[string]*BroadcastGroup)
}
