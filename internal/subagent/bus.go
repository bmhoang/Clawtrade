package subagent

import (
	"sync"
	"time"
)

type EventBus struct {
	subscribers map[string][]chan Event
	mu          sync.RWMutex
	recent      []Event
	maxRecent   int
}

func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[string][]chan Event),
		recent:      make([]Event, 0, 100),
		maxRecent:   100,
	}
}

func (b *EventBus) Subscribe(eventType string) <-chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan Event, 64)
	b.subscribers[eventType] = append(b.subscribers[eventType], ch)
	return ch
}

func (b *EventBus) Unsubscribe(eventType string, ch <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	subs := b.subscribers[eventType]
	for i, s := range subs {
		if s == ch {
			b.subscribers[eventType] = append(subs[:i], subs[i+1:]...)
			return
		}
	}
}

func (b *EventBus) Publish(ev Event) {
	if ev.Time.IsZero() {
		ev.Time = time.Now()
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers[ev.Type] {
		select {
		case ch <- ev:
		default:
			// drop if subscriber is full
		}
	}
	b.recent = append(b.recent, ev)
	if len(b.recent) > b.maxRecent {
		b.recent = b.recent[1:]
	}
}

// RecentEvents returns the most recent events, newest first.
func (b *EventBus) RecentEvents(limit int) []Event {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if limit <= 0 || limit > len(b.recent) {
		limit = len(b.recent)
	}
	result := make([]Event, limit)
	for i, j := 0, len(b.recent)-1; i < limit; i, j = i+1, j-1 {
		result[i] = b.recent[j]
	}
	return result
}
