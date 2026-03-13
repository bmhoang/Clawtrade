package engine

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Event represents a message passed through the event bus.
type Event struct {
	Type      string         `json:"type"`
	Data      map[string]any `json:"data,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

// EventHandler is a callback function that processes an event.
type EventHandler func(Event)

type subscription struct {
	id      uint64
	pattern string
	handler EventHandler
}

// EventBus provides publish/subscribe messaging with wildcard pattern support.
type EventBus struct {
	mu          sync.RWMutex
	subscribers []subscription
	nextID      atomic.Uint64
}

// NewEventBus creates a new EventBus instance.
func NewEventBus() *EventBus {
	return &EventBus{}
}

// Subscribe registers a handler for events matching the given pattern.
// Patterns support wildcard suffix matching (e.g. "price.*" matches "price.update").
// Returns a subscription ID that can be used to unsubscribe.
func (b *EventBus) Subscribe(pattern string, handler EventHandler) uint64 {
	id := b.nextID.Add(1)
	b.mu.Lock()
	b.subscribers = append(b.subscribers, subscription{id: id, pattern: pattern, handler: handler})
	b.mu.Unlock()
	return id
}

// Unsubscribe removes a subscription by its ID.
func (b *EventBus) Unsubscribe(id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, sub := range b.subscribers {
		if sub.id == id {
			b.subscribers = append(b.subscribers[:i], b.subscribers[i+1:]...)
			return
		}
	}
}

// Publish sends an event to all matching subscribers asynchronously.
// If the event has no timestamp, the current time is used.
func (b *EventBus) Publish(e Event) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	b.mu.RLock()
	var handlers []EventHandler
	for _, sub := range b.subscribers {
		if matchPattern(sub.pattern, e.Type) {
			handlers = append(handlers, sub.handler)
		}
	}
	b.mu.RUnlock()
	for _, h := range handlers {
		go h(e)
	}
}

// matchPattern checks if an event type matches a subscription pattern.
// Supports exact match, global wildcard "*", and prefix wildcard "prefix.*".
func matchPattern(pattern, eventType string) bool {
	if pattern == "*" || pattern == eventType {
		return true
	}
	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, ".*")
		return strings.HasPrefix(eventType, prefix+".")
	}
	return false
}
