// internal/streaming/portfolio_poller_test.go
package streaming

import (
	"context"
	"testing"
	"time"

	"github.com/clawtrade/clawtrade/internal/adapter"
	"github.com/clawtrade/clawtrade/internal/engine"
)

func TestPortfolioPoller_PublishesOnChange(t *testing.T) {
	bus := engine.NewEventBus()
	adp := &mockAdapter{name: "test", connected: true}

	pp := NewPortfolioPoller(PortfolioPollerConfig{
		Adapters:     map[string]adapter.TradingAdapter{"test": adp},
		Bus:          bus,
		PollInterval: 100 * time.Millisecond,
	})

	received := make(chan engine.Event, 1)
	bus.Subscribe("portfolio.update", func(e engine.Event) {
		received <- e
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go pp.Start(ctx)

	select {
	case ev := <-received:
		if ev.Type != "portfolio.update" {
			t.Errorf("expected type 'portfolio.update', got %q", ev.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for portfolio update")
	}
}

func TestPortfolioPoller_HashChanges(t *testing.T) {
	pp := &PortfolioPoller{}
	h1 := pp.hashState([]adapter.Balance{{Asset: "USDT", Total: 1000}}, nil)
	h2 := pp.hashState([]adapter.Balance{{Asset: "USDT", Total: 1000}}, nil)
	h3 := pp.hashState([]adapter.Balance{{Asset: "USDT", Total: 1001}}, nil)

	if h1 != h2 {
		t.Error("same state should produce same hash")
	}
	if h1 == h3 {
		t.Error("different state should produce different hash")
	}
}
