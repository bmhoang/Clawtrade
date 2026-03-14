package mql5

import (
	"testing"

	"github.com/clawtrade/clawtrade/internal/adapter"
)

// Compile-time check that MQL5Adapter implements TradingAdapter.
var _ adapter.TradingAdapter = (*MQL5Adapter)(nil)

func TestNewBridge(t *testing.T) {
	b := NewBridge("")
	if b == nil {
		t.Fatal("expected non-nil bridge")
	}
	if b.pending == nil {
		t.Fatal("expected pending map initialized")
	}
}

func TestBridgeNotConnectedByDefault(t *testing.T) {
	b := NewBridge("")
	if b.IsConnected() {
		t.Fatal("expected not connected by default")
	}
}

func TestBridgeCallNotConnected(t *testing.T) {
	b := NewBridge("")
	_, err := b.Call("get_price", map[string]string{"symbol": "EURUSD"})
	if err != ErrNotConnected {
		t.Fatalf("expected ErrNotConnected, got %v", err)
	}
}

func TestBridgeStopWhenNotStarted(t *testing.T) {
	b := NewBridge("")
	if err := b.Stop(); err != nil {
		t.Fatalf("Stop should not error when not started: %v", err)
	}
}

func TestBridgeSetCredentials(t *testing.T) {
	b := NewBridge("")
	b.SetCredentials("12345", "pass", "MetaQuotes-Demo")
	if b.login != "12345" {
		t.Fatalf("expected login 12345, got %s", b.login)
	}
	if b.password != "pass" {
		t.Fatalf("expected password pass, got %s", b.password)
	}
	if b.server != "MetaQuotes-Demo" {
		t.Fatalf("expected server MetaQuotes-Demo, got %s", b.server)
	}
}

func TestMQL5AdapterName(t *testing.T) {
	a := NewMQL5Adapter("")
	if a.Name() != "mt5" {
		t.Fatalf("expected name mt5, got %s", a.Name())
	}
}

func TestMQL5AdapterCapabilities(t *testing.T) {
	a := NewMQL5Adapter("")
	caps := a.Capabilities()

	if caps.Name != "mt5" {
		t.Fatalf("expected caps name mt5, got %s", caps.Name)
	}
	if caps.WebSocket {
		t.Fatal("expected websocket false")
	}
	if !caps.Margin {
		t.Fatal("expected margin true")
	}
	if caps.Futures {
		t.Fatal("expected futures false")
	}
	if len(caps.OrderTypes) != 3 {
		t.Fatalf("expected 3 order types, got %d", len(caps.OrderTypes))
	}
}

func TestMQL5AdapterNotConnectedByDefault(t *testing.T) {
	a := NewMQL5Adapter("")
	if a.IsConnected() {
		t.Fatal("expected not connected before Connect")
	}
}

func TestMQL5AdapterBridge(t *testing.T) {
	a := NewMQL5Adapter("")
	if a.Bridge() == nil {
		t.Fatal("expected non-nil bridge from adapter")
	}
}

func TestMQL5AdapterGetPriceNotConnected(t *testing.T) {
	a := NewMQL5Adapter("")
	_, err := a.GetPrice(nil, "EURUSD")
	if err != ErrNotConnected {
		t.Fatalf("expected ErrNotConnected, got %v", err)
	}
}

func TestMQL5AdapterGetCandlesNotConnected(t *testing.T) {
	a := NewMQL5Adapter("")
	_, err := a.GetCandles(nil, "EURUSD", "1h", 100)
	if err != ErrNotConnected {
		t.Fatalf("expected ErrNotConnected, got %v", err)
	}
}

func TestMQL5AdapterGetBalancesNotConnected(t *testing.T) {
	a := NewMQL5Adapter("")
	_, err := a.GetBalances(nil)
	if err != ErrNotConnected {
		t.Fatalf("expected ErrNotConnected, got %v", err)
	}
}

func TestMQL5AdapterGetPositionsNotConnected(t *testing.T) {
	a := NewMQL5Adapter("")
	_, err := a.GetPositions(nil)
	if err != ErrNotConnected {
		t.Fatalf("expected ErrNotConnected, got %v", err)
	}
}

func TestMQL5AdapterPlaceOrderNotConnected(t *testing.T) {
	a := NewMQL5Adapter("")
	_, err := a.PlaceOrder(nil, adapter.Order{Symbol: "EURUSD", Side: adapter.SideBuy, Size: 0.1})
	if err != ErrNotConnected {
		t.Fatalf("expected ErrNotConnected, got %v", err)
	}
}

func TestMQL5AdapterCancelOrderNotConnected(t *testing.T) {
	a := NewMQL5Adapter("")
	err := a.CancelOrder(nil, "12345")
	if err != ErrNotConnected {
		t.Fatalf("expected ErrNotConnected, got %v", err)
	}
}

func TestMQL5AdapterGetOpenOrdersNotConnected(t *testing.T) {
	a := NewMQL5Adapter("")
	_, err := a.GetOpenOrders(nil)
	if err != ErrNotConnected {
		t.Fatalf("expected ErrNotConnected, got %v", err)
	}
}

func TestMQL5AdapterOrderBookNotSupported(t *testing.T) {
	a := NewMQL5Adapter("")
	_, err := a.GetOrderBook(nil, "EURUSD", 10)
	if err == nil {
		t.Fatal("expected error for unsupported order book")
	}
}

func TestErrorMessage(t *testing.T) {
	if ErrNotConnected.Error() != "metatrader not connected" {
		t.Fatalf("unexpected error message: %s", ErrNotConnected.Error())
	}
}

func TestFindPython(t *testing.T) {
	python := findPython()
	if python == "" {
		t.Fatal("expected non-empty python path")
	}
}
