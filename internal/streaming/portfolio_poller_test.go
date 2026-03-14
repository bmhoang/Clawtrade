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

func TestPortfolioPoller_MultiAdapterAggregation(t *testing.T) {
	bus := engine.NewEventBus()

	adp1 := &mockAdapterWithData{
		mockAdapter: mockAdapter{name: "binance", connected: true},
		balances:    []adapter.Balance{{Asset: "USDT", Total: 1000}},
		positions:   []adapter.Position{{Symbol: "BTC/USDT", PnL: 50}},
	}
	adp2 := &mockAdapterWithData{
		mockAdapter: mockAdapter{name: "bybit", connected: true},
		balances:    []adapter.Balance{{Asset: "USDT", Total: 2000}},
		positions:   []adapter.Position{{Symbol: "ETH/USDT", PnL: -20}},
	}

	pp := NewPortfolioPoller(PortfolioPollerConfig{
		Adapters: map[string]adapter.TradingAdapter{
			"binance": adp1,
			"bybit":   adp2,
		},
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
		balances := ev.Data["balances"].([]adapter.Balance)
		if len(balances) != 2 {
			t.Errorf("expected 2 balances, got %d", len(balances))
		}
		positions := ev.Data["positions"].([]adapter.Position)
		if len(positions) != 2 {
			t.Errorf("expected 2 positions, got %d", len(positions))
		}
		totalPnL := ev.Data["total_pnl"].(float64)
		if totalPnL != 30 {
			t.Errorf("expected total PnL 30, got %f", totalPnL)
		}
		exchanges := ev.Data["exchanges"].(map[string]any)
		if len(exchanges) != 2 {
			t.Errorf("expected 2 exchanges, got %d", len(exchanges))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for portfolio update")
	}
}

// mockAdapterWithData extends mockAdapter with balance/position data.
type mockAdapterWithData struct {
	mockAdapter
	balances  []adapter.Balance
	positions []adapter.Position
}

func (m *mockAdapterWithData) GetBalances(ctx context.Context) ([]adapter.Balance, error) {
	return m.balances, nil
}

func (m *mockAdapterWithData) GetPositions(ctx context.Context) ([]adapter.Position, error) {
	return m.positions, nil
}
