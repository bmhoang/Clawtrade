package backtest

import (
	"testing"
	"time"

	"github.com/clawtrade/clawtrade/internal/adapter"
	"github.com/clawtrade/clawtrade/internal/strategy"
)

// makeCandles creates adapter candles from close prices for testing.
func makeCandles(closes []float64) []adapter.Candle {
	candles := make([]adapter.Candle, len(closes))
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, c := range closes {
		candles[i] = adapter.Candle{
			Open:      c - 1,
			High:      c + 2,
			Low:       c - 2,
			Close:     c,
			Volume:    1000,
			Timestamp: base.Add(time.Duration(i) * time.Hour),
		}
	}
	return candles
}

func TestCodeStrategy(t *testing.T) {
	arena := strategy.NewArena()

	// Register a simple strategy: buy if last price > first price, sell otherwise.
	err := arena.Register("test_strat", "test", func(symbol string, prices []float64) *strategy.Signal {
		if len(prices) < 2 {
			return nil
		}
		if prices[len(prices)-1] > prices[0] {
			return &strategy.Signal{Symbol: symbol, Side: "buy", Strength: 0.8, Reason: "rising"}
		}
		return &strategy.Signal{Symbol: symbol, Side: "sell", Strength: 0.6, Reason: "falling"}
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	cs := &CodeStrategy{Arena: arena, StrategyName: "test_strat"}

	// Rising prices → buy
	rising := makeCandles([]float64{10, 11, 12, 13, 14, 15})
	sig := cs.Evaluate("BTCUSD", rising)
	if sig == nil {
		t.Fatal("expected signal, got nil")
	}
	if sig.Action != ActionBuy {
		t.Errorf("expected buy, got %s", sig.Action)
	}
	if sig.Strength != 0.8 {
		t.Errorf("expected strength 0.8, got %f", sig.Strength)
	}

	// Falling prices → sell
	falling := makeCandles([]float64{20, 19, 18, 17, 16, 15})
	sig = cs.Evaluate("BTCUSD", falling)
	if sig == nil {
		t.Fatal("expected signal, got nil")
	}
	if sig.Action != ActionSell {
		t.Errorf("expected sell, got %s", sig.Action)
	}
}

func TestConfigStrategy(t *testing.T) {
	// Generate enough candles (40+) so RSI can be computed (needs > 14 candles).
	// Use a pattern that should produce RSI < 35 at the end: steady decline.
	closes := make([]float64, 40)
	for i := range closes {
		closes[i] = 100 - float64(i)*1.5 // declining
	}
	candles := makeCandles(closes)

	cs := &ConfigStrategy{
		BuyWhen:  "rsi < 35",
		SellWhen: "rsi > 70",
	}

	sig := cs.Evaluate("ETHUSD", candles)
	if sig == nil {
		t.Fatal("expected signal, got nil")
	}
	// With declining prices, RSI should be low → buy or at least not panic
	t.Logf("signal: action=%s strength=%f reason=%s", sig.Action, sig.Strength, sig.Reason)
}

func TestGetBuiltinStrategy(t *testing.T) {
	for _, name := range []string{"momentum", "meanrevert", "macd", "unknown"} {
		s := GetBuiltinStrategy(name)
		if s == nil {
			t.Errorf("GetBuiltinStrategy(%q) returned nil", name)
		}
	}
}
