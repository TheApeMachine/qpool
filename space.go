package qpool

import "time"

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

	// Close waiting channels
	for id, channels := range qs.waiting {
		for _, ch := range channels {
			close(ch)
		}
		delete(qs.waiting, id)
	}

	// Close BroadcastGroup channels
	for id, group := range qs.groups {
		for _, ch := range group.channels {
			close(ch)
		}
		delete(qs.groups, id)
	}

	// Clear values
	qs.values = nil
}
