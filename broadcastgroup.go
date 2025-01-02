// broadcastgroup.go
package qpool

import (
	"math"
	"sync"
	"time"
)

/*
	FilterFunc defines a function type for filtering quantum values.

This function type is used to determine whether a quantum value should be
processed or ignored in the broadcast system.

Parameters:
  - *QValue: The quantum value to be filtered

Returns:
  - bool: True if the value should be processed, false if it should be filtered out
*/
type FilterFunc func(*QValue) bool

/*
	RoutingRule defines how messages should be routed to specific subscribers.

It combines subscriber identification with filtering logic and priority levels
to enable sophisticated message routing in the broadcast system.
*/
type RoutingRule struct {
	SubscriberID string
	Filter       FilterFunc
	Priority     int
}

/*
	BroadcastGroup implements a quantum-aware pub/sub system.

It provides a quantum-inspired broadcast mechanism that maintains quantum properties
such as entanglement and uncertainty while distributing messages to subscribers.

Key features:
  - Quantum state preservation
  - Filtered message routing
  - Subscriber management
  - Metrics collection
  - Entanglement support
*/
type BroadcastGroup struct {
	mu sync.RWMutex

	ID           string
	channels     []chan *QValue
	subscribers  map[string]chan *QValue
	filters      []FilterFunc
	routingRules map[string][]RoutingRule
	metrics      *BroadcastMetrics

	// Quantum properties
	entanglement *Entanglement
	uncertainty  UncertaintyLevel

	// Management
	TTL          time.Duration
	LastUsed     time.Time
	maxQueueSize int
}

/*
	BroadcastMetrics tracks performance and behavior of the broadcast group.

Collects and maintains statistical information about the broadcast group's
operation, including message counts, latency, and quantum uncertainty levels.
*/
type BroadcastMetrics struct {
	MessagesSent      int64
	MessagesDropped   int64
	AverageLatency    time.Duration
	ActiveSubscribers int
	UncertaintyLevel  UncertaintyLevel
	LastBroadcastTime time.Time
}

/*
	NewBroadcastGroup creates a new broadcast group with quantum properties.

Initializes a new broadcast group with specified parameters and default
quantum properties such as minimum uncertainty.

Parameters:
  - id: Unique identifier for the broadcast group
  - ttl: Time-to-live duration for the group
  - maxQueue: Maximum queue size for message buffering

Returns:
  - *BroadcastGroup: A new broadcast group instance
*/
func NewBroadcastGroup(id string, ttl time.Duration, maxQueue int) *BroadcastGroup {
	return &BroadcastGroup{
		ID:           id,
		subscribers:  make(map[string]chan *QValue),
		routingRules: make(map[string][]RoutingRule),
		TTL:          ttl,
		LastUsed:     time.Now(),
		maxQueueSize: maxQueue,
		metrics:      &BroadcastMetrics{},
		uncertainty:  MinUncertainty,
	}
}

/*
	Subscribe adds a new subscriber with optional filtering and routing rules.

Creates and registers a new subscriber channel with specified buffer size and
optional routing rules for message filtering.

Parameters:
  - subscriberID: Unique identifier for the subscriber
  - bufferSize: Size of the subscriber's message buffer
  - rules: Optional routing rules for message filtering

Returns:
  - chan *QValue: Channel for receiving broadcast messages

Thread-safe: This method uses mutual exclusion to ensure safe concurrent access.
*/
func (bg *BroadcastGroup) Subscribe(subscriberID string, bufferSize int, rules ...RoutingRule) chan *QValue {
	bg.mu.Lock()
	defer bg.mu.Unlock()

	ch := make(chan *QValue, bufferSize)
	bg.subscribers[subscriberID] = ch

	if len(rules) > 0 {
		bg.routingRules[subscriberID] = rules
	}

	bg.metrics.ActiveSubscribers++
	return ch
}

/*
	Unsubscribe removes a subscriber and cleans up associated resources.

Safely removes a subscriber from the broadcast group, closing their channel
and cleaning up any associated routing rules.

Parameters:
  - subscriberID: ID of the subscriber to remove

Thread-safe: This method uses mutual exclusion to ensure safe concurrent access.
*/
func (bg *BroadcastGroup) Unsubscribe(subscriberID string) {
	bg.mu.Lock()
	defer bg.mu.Unlock()

	if ch, exists := bg.subscribers[subscriberID]; exists {
		close(ch)
		delete(bg.subscribers, subscriberID)
		delete(bg.routingRules, subscriberID)
		bg.metrics.ActiveSubscribers--
	}
}

/*
	Send broadcasts a quantum value to all applicable subscribers.

Distributes a quantum value to subscribers according to routing rules while
maintaining quantum properties and updating metrics.

Parameters:
  - qv: The quantum value to broadcast

Thread-safe: This method uses mutual exclusion to ensure safe concurrent access.
*/
func (bg *BroadcastGroup) Send(qv *QValue) {
	bg.mu.Lock()
	defer bg.mu.Unlock()

	startTime := time.Now()
	bg.LastUsed = startTime

	// Apply global filters
	for _, filter := range bg.filters {
		if !filter(qv) {
			bg.metrics.MessagesDropped++
			return
		}
	}

	// Track message through entanglement if configured
	if bg.entanglement != nil {
		bg.entanglement.UpdateState("broadcast", qv)
	}

	// Apply routing rules and send to subscribers
	for subID, ch := range bg.subscribers {
		if rules, hasRules := bg.routingRules[subID]; hasRules {
			// Check if message passes any routing rules
			shouldSend := false
			for _, rule := range rules {
				if rule.Filter(qv) {
					shouldSend = true
					break
				}
			}
			if !shouldSend {
				continue
			}
		}

		// Attempt to send with non-blocking write
		select {
		case ch <- qv:
			bg.metrics.MessagesSent++
		default:
			// Channel full - message dropped
			bg.metrics.MessagesDropped++
		}
	}

	// Update metrics
	bg.metrics.LastBroadcastTime = startTime
	bg.metrics.AverageLatency = time.Since(startTime)
	bg.updateUncertainty()
}

/*
	AddFilter adds a global filter to the broadcast group.

Registers a new filter function that will be applied to all messages
before broadcasting.

Parameters:
  - filter: The filter function to add

Thread-safe: This method uses mutual exclusion to ensure safe concurrent access.
*/
func (bg *BroadcastGroup) AddFilter(filter FilterFunc) {
	bg.mu.Lock()
	defer bg.mu.Unlock()
	bg.filters = append(bg.filters, filter)
}

/*
	AddRoutingRule adds a routing rule for a specific subscriber.

Adds a new routing rule that determines how messages should be filtered
for a specific subscriber.

Parameters:
  - subscriberID: ID of the subscriber to add the rule for
  - rule: The routing rule to add

Thread-safe: This method uses mutual exclusion to ensure safe concurrent access.
*/
func (bg *BroadcastGroup) AddRoutingRule(subscriberID string, rule RoutingRule) {
	bg.mu.Lock()
	defer bg.mu.Unlock()
	bg.routingRules[subscriberID] = append(bg.routingRules[subscriberID], rule)
}

/*
	SetEntanglement connects the broadcast group to an entanglement.

Associates the broadcast group with a quantum entanglement, allowing it to
participate in quantum-like state synchronization.

Parameters:
  - e: The entanglement to connect to

Thread-safe: This method uses mutual exclusion to ensure safe concurrent access.
*/
func (bg *BroadcastGroup) SetEntanglement(e *Entanglement) {
	bg.mu.Lock()
	defer bg.mu.Unlock()
	bg.entanglement = e
}

/*
	updateUncertainty adjusts uncertainty based on broadcast patterns.

Updates the uncertainty level of the broadcast group based on the time
elapsed since the last broadcast, implementing quantum-inspired uncertainty
principles.

Thread-safe: Called within Send which provides mutex protection.
*/
func (bg *BroadcastGroup) updateUncertainty() {
	timeSinceLastBroadcast := time.Since(bg.metrics.LastBroadcastTime)

	// Uncertainty increases with time since last broadcast
	uncertaintyFactor := float64(timeSinceLastBroadcast) / float64(time.Second)
	newUncertainty := UncertaintyLevel(math.Min(
		float64(bg.uncertainty)+(uncertaintyFactor*0.01),
		float64(MaxUncertainty),
	))

	bg.uncertainty = newUncertainty
	bg.metrics.UncertaintyLevel = newUncertainty
}

/*
	GetMetrics returns current broadcast metrics.

Provides access to the current operational metrics of the broadcast group.

Returns:
  - BroadcastMetrics: Copy of the current metrics

Thread-safe: This method uses read-lock to ensure safe concurrent access.
*/
func (bg *BroadcastGroup) GetMetrics() BroadcastMetrics {
	bg.mu.RLock()
	defer bg.mu.RUnlock()
	return *bg.metrics
}

/*
	Close shuts down the broadcast group and cleans up resources.

Performs graceful shutdown of the broadcast group, closing all subscriber
channels and cleaning up internal resources.

Thread-safe: This method uses mutual exclusion to ensure safe concurrent access.
*/
func (bg *BroadcastGroup) Close() {
	bg.mu.Lock()
	defer bg.mu.Unlock()

	// Close all subscriber channels
	for _, ch := range bg.subscribers {
		close(ch)
	}

	// Clear maps and slices
	bg.subscribers = nil
	bg.routingRules = nil
	bg.filters = nil
	bg.entanglement = nil
}
