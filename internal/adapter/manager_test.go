package adapter

import (
	"context"
	"testing"
)

// mockAdapter is a minimal TradingAdapter for testing the manager.
type mockAdapter struct {
	name      string
	connected bool
	caps      AdapterCaps
}

func newMock(name string, connected bool) *mockAdapter {
	return &mockAdapter{
		name:      name,
		connected: connected,
		caps: AdapterCaps{
			Name:       name,
			WebSocket:  true,
			OrderTypes: []OrderType{OrderTypeLimit, OrderTypeMarket},
		},
	}
}

func (m *mockAdapter) Name() string                        { return m.name }
func (m *mockAdapter) Capabilities() AdapterCaps           { return m.caps }
func (m *mockAdapter) IsConnected() bool                   { return m.connected }
func (m *mockAdapter) Connect(_ context.Context) error     { m.connected = true; return nil }
func (m *mockAdapter) Disconnect() error                   { m.connected = false; return nil }
func (m *mockAdapter) GetPrice(_ context.Context, _ string) (*Price, error) {
	return &Price{}, nil
}
func (m *mockAdapter) GetCandles(_ context.Context, _, _ string, _ int) ([]Candle, error) {
	return nil, nil
}
func (m *mockAdapter) GetOrderBook(_ context.Context, _ string, _ int) (*OrderBook, error) {
	return nil, nil
}
func (m *mockAdapter) PlaceOrder(_ context.Context, o Order) (*Order, error) { return &o, nil }
func (m *mockAdapter) CancelOrder(_ context.Context, _ string) error        { return nil }
func (m *mockAdapter) GetOpenOrders(_ context.Context) ([]Order, error)     { return nil, nil }
func (m *mockAdapter) GetBalances(_ context.Context) ([]Balance, error)     { return nil, nil }
func (m *mockAdapter) GetPositions(_ context.Context) ([]Position, error)   { return nil, nil }

func TestManagerRegisterAndGet(t *testing.T) {
	mgr := NewAdapterManager(0)

	a := newMock("binance", true)
	mgr.Register("binance", a)

	got, err := mgr.Get("binance")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.Name() != "binance" {
		t.Fatalf("expected binance, got %s", got.Name())
	}
}

func TestManagerGetNotFound(t *testing.T) {
	mgr := NewAdapterManager(0)

	_, err := mgr.Get("missing")
	if err != ErrAdapterNotFound {
		t.Fatalf("expected ErrAdapterNotFound, got %v", err)
	}
}

func TestManagerUnregister(t *testing.T) {
	mgr := NewAdapterManager(0)
	mgr.Register("a", newMock("a", true))
	mgr.Register("b", newMock("b", true))

	mgr.Unregister("a")
	if mgr.Len() != 1 {
		t.Fatalf("expected 1 adapter, got %d", mgr.Len())
	}
	_, err := mgr.Get("a")
	if err != ErrAdapterNotFound {
		t.Fatal("expected not found after unregister")
	}
}

func TestManagerPrimaryIsFirstRegistered(t *testing.T) {
	mgr := NewAdapterManager(0)
	mgr.Register("first", newMock("first", true))
	mgr.Register("second", newMock("second", true))

	got, err := mgr.GetPrimary()
	if err != nil {
		t.Fatalf("GetPrimary error: %v", err)
	}
	if got.Name() != "first" {
		t.Fatalf("expected first as primary, got %s", got.Name())
	}
}

func TestManagerSetPrimary(t *testing.T) {
	mgr := NewAdapterManager(0)
	mgr.Register("a", newMock("a", true))
	mgr.Register("b", newMock("b", true))

	mgr.SetPrimary("b")

	got, err := mgr.GetPrimary()
	if err != nil {
		t.Fatalf("GetPrimary error: %v", err)
	}
	if got.Name() != "b" {
		t.Fatalf("expected b as primary, got %s", got.Name())
	}
}

func TestManagerPrimaryFallback(t *testing.T) {
	mgr := NewAdapterManager(0)
	mgr.Register("primary", newMock("primary", false)) // unhealthy
	mgr.Register("fallback", newMock("fallback", true)) // healthy

	got, err := mgr.GetPrimary()
	if err != nil {
		t.Fatalf("GetPrimary error: %v", err)
	}
	if got.Name() != "fallback" {
		t.Fatalf("expected fallback, got %s", got.Name())
	}
}

func TestManagerPrimaryNoHealthy(t *testing.T) {
	mgr := NewAdapterManager(0)
	mgr.Register("a", newMock("a", false))

	_, err := mgr.GetPrimary()
	if err != ErrNoHealthy {
		t.Fatalf("expected ErrNoHealthy, got %v", err)
	}
}

func TestManagerList(t *testing.T) {
	mgr := NewAdapterManager(0)
	mgr.Register("alpha", newMock("alpha", true))
	mgr.Register("beta", newMock("beta", false))

	list := mgr.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(list))
	}

	// First registered is primary.
	if !list[0].Primary {
		t.Fatal("expected alpha to be primary")
	}
	if list[0].Name != "alpha" || !list[0].Healthy {
		t.Fatalf("unexpected info for alpha: %+v", list[0])
	}
	if list[1].Name != "beta" || list[1].Healthy {
		t.Fatalf("unexpected info for beta: %+v", list[1])
	}
}

func TestManagerHealthCheck(t *testing.T) {
	mgr := NewAdapterManager(0)
	mock := newMock("x", true)
	mgr.Register("x", mock)

	// Disconnect and run health check.
	mock.connected = false
	mgr.HealthCheck()

	_, err := mgr.GetPrimary()
	if err != ErrNoHealthy {
		t.Fatal("expected no healthy after health check")
	}

	// Reconnect and check again.
	mock.connected = true
	mgr.HealthCheck()

	got, err := mgr.GetPrimary()
	if err != nil {
		t.Fatalf("expected healthy adapter, got error: %v", err)
	}
	if got.Name() != "x" {
		t.Fatalf("expected x, got %s", got.Name())
	}
}

func TestManagerRateLimiter(t *testing.T) {
	mgr := NewAdapterManager(2) // 2 requests per second
	mgr.Register("r", newMock("r", true))

	// First two calls should succeed.
	for i := 0; i < 2; i++ {
		if _, err := mgr.Get("r"); err != nil {
			t.Fatalf("call %d should succeed: %v", i, err)
		}
	}
	// Third call should be rate limited.
	_, err := mgr.Get("r")
	if err != ErrRateLimited {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}

func TestManagerUnregisterPrimaryPromotes(t *testing.T) {
	mgr := NewAdapterManager(0)
	mgr.Register("a", newMock("a", true))
	mgr.Register("b", newMock("b", true))

	mgr.Unregister("a")

	got, err := mgr.GetPrimary()
	if err != nil {
		t.Fatalf("GetPrimary error: %v", err)
	}
	if got.Name() != "b" {
		t.Fatalf("expected b promoted to primary, got %s", got.Name())
	}
}

func TestManagerConcurrentAccess(t *testing.T) {
	mgr := NewAdapterManager(0)
	mgr.Register("c", newMock("c", true))

	done := make(chan struct{})
	for i := 0; i < 50; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			mgr.Get("c")
			mgr.List()
			mgr.GetPrimary()
			mgr.HealthCheck()
		}()
	}
	for i := 0; i < 50; i++ {
		<-done
	}
}
