package binance

import (
	"testing"

	"github.com/clawtrade/clawtrade/internal/adapter"
)

// Compile-time interface compliance check.
var _ adapter.TradingAdapter = (*Adapter)(nil)

func TestNew(t *testing.T) {
	a := New("key", "secret")
	if a == nil {
		t.Fatal("expected non-nil adapter")
	}
}

func TestName(t *testing.T) {
	a := New("key", "secret")
	if a.Name() != "binance" {
		t.Fatalf("expected name %q, got %q", "binance", a.Name())
	}
}

func TestCapabilities(t *testing.T) {
	a := New("key", "secret")
	caps := a.Capabilities()

	if caps.Name != "binance" {
		t.Fatalf("expected caps name %q, got %q", "binance", caps.Name)
	}
	if !caps.WebSocket {
		t.Fatal("expected WebSocket to be true")
	}
	if !caps.Margin {
		t.Fatal("expected Margin to be true")
	}
	if !caps.Futures {
		t.Fatal("expected Futures to be true")
	}
	if len(caps.OrderTypes) != 3 {
		t.Fatalf("expected 3 order types, got %d", len(caps.OrderTypes))
	}
}
