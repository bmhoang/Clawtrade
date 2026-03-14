package subagent

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/clawtrade/clawtrade/internal/adapter"
)

func TestCorrelationAgent_Name(t *testing.T) {
	ca := NewCorrelationAgent(CorrelationConfig{})
	if ca.Name() != "correlation" {
		t.Errorf("expected 'correlation', got %q", ca.Name())
	}
}

func TestCorrelationAgent_CalcCorrelation(t *testing.T) {
	// Perfect positive correlation
	a := []float64{1, 2, 3, 4, 5}
	b := []float64{2, 4, 6, 8, 10}
	corr := calcCorrelation(a, b)
	if corr < 0.99 {
		t.Errorf("expected ~1.0, got %f", corr)
	}

	// Perfect negative correlation
	c := []float64{5, 4, 3, 2, 1}
	corr2 := calcCorrelation(a, c)
	if corr2 > -0.99 {
		t.Errorf("expected ~-1.0, got %f", corr2)
	}
}

func TestCorrelationAgent_CalcCorrelation_ZeroVariance(t *testing.T) {
	a := []float64{5, 5, 5, 5, 5}
	b := []float64{1, 2, 3, 4, 5}
	corr := calcCorrelation(a, b)
	if corr != 0 {
		t.Errorf("expected 0 for zero variance, got %f", corr)
	}
}

func TestCorrelationAgent_CalcCorrelation_UnequalLength(t *testing.T) {
	a := []float64{1, 2, 3}
	b := []float64{1, 2}
	corr := calcCorrelation(a, b)
	if corr != 0 {
		t.Errorf("expected 0 for unequal length, got %f", corr)
	}
}

func TestCorrelationAgent_CalcCorrelation_NoCorrelation(t *testing.T) {
	// Roughly uncorrelated data
	a := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	b := []float64{5, 3, 7, 1, 9, 2, 8, 4, 6, 10}
	corr := calcCorrelation(a, b)
	if math.Abs(corr) > 0.5 {
		t.Errorf("expected weak correlation, got %f", corr)
	}
}

func TestCorrelationAgent_FormatCorrelationData(t *testing.T) {
	ca := NewCorrelationAgent(CorrelationConfig{})

	correlations := map[string]float64{
		"BTC/USDT vs ETH/USDT": 0.85,
	}
	prices := map[string]float64{
		"BTC/USDT": 64000.00,
		"ETH/USDT": 3400.00,
	}

	result := ca.formatCorrelationData(correlations, prices)

	if !strings.Contains(result, "Cross-Asset Correlations") {
		t.Error("expected correlation header in output")
	}
	if !strings.Contains(result, "BTC/USDT vs ETH/USDT") {
		t.Error("expected correlation pair in output")
	}
	if !strings.Contains(result, "0.85") {
		t.Error("expected correlation value in output")
	}
	if !strings.Contains(result, "Current Prices") {
		t.Error("expected prices header in output")
	}
	if !strings.Contains(result, "$64,000.00") {
		t.Error("expected BTC price in output")
	}
	if !strings.Contains(result, "$3,400.00") {
		t.Error("expected ETH price in output")
	}
}

func TestCorrelationAgent_Status(t *testing.T) {
	ca := NewCorrelationAgent(CorrelationConfig{})
	status := ca.Status()
	if status.Name != "correlation" {
		t.Errorf("expected name 'correlation', got %q", status.Name)
	}
	if status.Running {
		t.Error("expected not running initially")
	}
}

func TestCorrelationAgent_StartStop(t *testing.T) {
	ca := NewCorrelationAgent(CorrelationConfig{
		ScanInterval: 100 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- ca.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	status := ca.Status()
	if !status.Running {
		t.Error("expected running after Start")
	}

	if err := ca.Stop(); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start() returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start() did not return after Stop()")
	}

	status = ca.Status()
	if status.Running {
		t.Error("expected not running after Stop")
	}
}

func TestCorrelationAgent_RunScan_PublishesEvent(t *testing.T) {
	bus := NewEventBus()
	events := bus.Subscribe("correlation")

	mockAdapter := &mockCorrelationAdapter{
		prices: map[string]float64{
			"BTC/USDT": 64000.0,
			"ETH/USDT": 3400.0,
		},
	}

	ca := NewCorrelationAgent(CorrelationConfig{
		Bus:          bus,
		Adapters:     map[string]adapter.TradingAdapter{"mock": mockAdapter},
		Watchlist:    []string{"BTC/USDT", "ETH/USDT"},
		ScanInterval: time.Minute,
	})

	// Pre-fill price history so we have enough data points
	for i := 0; i < 6; i++ {
		ca.priceHistory["BTC/USDT"] = append(ca.priceHistory["BTC/USDT"], 60000.0+float64(i)*1000)
		ca.priceHistory["ETH/USDT"] = append(ca.priceHistory["ETH/USDT"], 3000.0+float64(i)*100)
	}

	ca.runScan(context.Background())

	select {
	case e := <-events:
		if e.Type != "correlation" {
			t.Errorf("expected event type 'correlation', got %q", e.Type)
		}
		if e.Source != "correlation" {
			t.Errorf("expected source 'correlation', got %q", e.Source)
		}
	case <-time.After(time.Second):
		t.Fatal("expected correlation event to be published")
	}
}

func TestCorrelationAgent_PriceHistoryRolling(t *testing.T) {
	mockAdapter := &mockCorrelationAdapter{
		prices: map[string]float64{
			"BTC/USDT": 64000.0,
		},
	}

	ca := NewCorrelationAgent(CorrelationConfig{
		Adapters:     map[string]adapter.TradingAdapter{"mock": mockAdapter},
		Watchlist:    []string{"BTC/USDT"},
		ScanInterval: time.Minute,
	})

	// Pre-fill with 30 data points
	for i := 0; i < 30; i++ {
		ca.priceHistory["BTC/USDT"] = append(ca.priceHistory["BTC/USDT"], float64(i))
	}

	ca.runScan(context.Background())

	history := ca.priceHistory["BTC/USDT"]
	if len(history) > 30 {
		t.Errorf("expected max 30 data points, got %d", len(history))
	}
}

func TestCorrelationAgent_DefaultScanInterval(t *testing.T) {
	ca := NewCorrelationAgent(CorrelationConfig{})
	if ca.cfg.ScanInterval != 10*time.Minute {
		t.Errorf("expected default scan interval 10m, got %v", ca.cfg.ScanInterval)
	}
}

// mockCorrelationAdapter is a minimal mock implementing adapter.TradingAdapter for correlation tests.
type mockCorrelationAdapter struct {
	adapter.TradingAdapter
	prices map[string]float64
}

func (m *mockCorrelationAdapter) IsConnected() bool { return true }
func (m *mockCorrelationAdapter) Name() string       { return "mock" }
func (m *mockCorrelationAdapter) GetPrice(ctx context.Context, symbol string) (*adapter.Price, error) {
	p, ok := m.prices[symbol]
	if !ok {
		p = 100.0
	}
	return &adapter.Price{Last: p, Bid: p - 1, Ask: p + 1}, nil
}
