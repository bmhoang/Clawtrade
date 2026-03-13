package engine

import (
	"sync"
	"testing"
	"time"
)

func TestEventBus_PublishSubscribe(t *testing.T) {
	bus := NewEventBus()

	var received []Event
	var mu sync.Mutex

	bus.Subscribe("price.update", func(e Event) {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	})

	bus.Publish(Event{
		Type: "price.update",
		Data: map[string]any{"symbol": "BTC/USDT", "price": 67180.0},
	})

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	if received[0].Data["symbol"] != "BTC/USDT" {
		t.Errorf("unexpected symbol: %v", received[0].Data["symbol"])
	}
}

func TestEventBus_WildcardSubscribe(t *testing.T) {
	bus := NewEventBus()

	var count int
	var mu sync.Mutex

	bus.Subscribe("price.*", func(e Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	bus.Publish(Event{Type: "price.update"})
	bus.Publish(Event{Type: "price.alert"})
	bus.Publish(Event{Type: "trade.filled"})

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if count != 2 {
		t.Errorf("expected 2 events, got %d", count)
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	bus := NewEventBus()

	var count int
	var mu sync.Mutex

	id := bus.Subscribe("test", func(e Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	bus.Publish(Event{Type: "test"})
	time.Sleep(50 * time.Millisecond)

	bus.Unsubscribe(id)

	bus.Publish(Event{Type: "test"})
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if count != 1 {
		t.Errorf("expected 1 event after unsub, got %d", count)
	}
}
