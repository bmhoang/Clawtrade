package risk

import (
	"sync"
	"testing"
)

func TestMonitorAddRemovePosition(t *testing.T) {
	m := NewMonitor(DefaultMonitorConfig())

	m.AddPosition("BTC-USDT", "long", 1.0, 50000)

	pos, ok := m.GetPosition("BTC-USDT")
	if !ok {
		t.Fatal("expected position to exist")
	}
	if pos.Symbol != "BTC-USDT" {
		t.Errorf("expected symbol BTC-USDT, got %s", pos.Symbol)
	}
	if pos.EntryPrice != 50000 {
		t.Errorf("expected entry price 50000, got %f", pos.EntryPrice)
	}
	if pos.Side != "long" {
		t.Errorf("expected side long, got %s", pos.Side)
	}

	m.RemovePosition("BTC-USDT")
	_, ok = m.GetPosition("BTC-USDT")
	if ok {
		t.Error("expected position to be removed")
	}
}

func TestMonitorUpdatePriceLong(t *testing.T) {
	m := NewMonitor(DefaultMonitorConfig())
	m.AddPosition("BTC-USDT", "long", 1.0, 50000)

	// Price goes up
	m.UpdatePrice("BTC-USDT", 52000)
	pos, _ := m.GetPosition("BTC-USDT")

	expectedPnL := 2000.0
	if pos.UnrealizedPnL != expectedPnL {
		t.Errorf("expected PnL %f, got %f", expectedPnL, pos.UnrealizedPnL)
	}
	if pos.PeakPnL != expectedPnL {
		t.Errorf("expected peak PnL %f, got %f", expectedPnL, pos.PeakPnL)
	}
}

func TestMonitorUpdatePriceShort(t *testing.T) {
	m := NewMonitor(DefaultMonitorConfig())
	m.AddPosition("ETH-USDT", "short", 10.0, 3000)

	// Price goes down (profit for short)
	m.UpdatePrice("ETH-USDT", 2900)
	pos, _ := m.GetPosition("ETH-USDT")

	expectedPnL := 1000.0 // (3000 - 2900) * 10
	if pos.UnrealizedPnL != expectedPnL {
		t.Errorf("expected PnL %f, got %f", expectedPnL, pos.UnrealizedPnL)
	}
}

func TestMonitorDrawdownWarning(t *testing.T) {
	cfg := MonitorConfig{
		DrawdownWarning:  0.05,
		DrawdownCritical: 0.10,
		TrailingStopPct:  0,
	}
	m := NewMonitor(cfg)
	m.AddPosition("BTC-USDT", "long", 1.0, 50000)

	// Price up to set a peak
	m.UpdatePrice("BTC-USDT", 60000) // PnL = 10000
	_ = m.GetAlerts()                // clear

	// Drop 6% of peak PnL -> warning
	// Need PnL = 10000 * 0.94 = 9400, price = 50000 + 9400 = 59400
	m.UpdatePrice("BTC-USDT", 59400)

	alerts := m.GetAlerts()
	if len(alerts) == 0 {
		t.Fatal("expected a drawdown warning alert")
	}
	found := false
	for _, a := range alerts {
		if a.Type == AlertDrawdown && a.Severity == SeverityWarning {
			found = true
		}
	}
	if !found {
		t.Error("expected a WARNING drawdown alert")
	}
}

func TestMonitorDrawdownCritical(t *testing.T) {
	cfg := MonitorConfig{
		DrawdownWarning:  0.05,
		DrawdownCritical: 0.10,
		TrailingStopPct:  0,
	}
	m := NewMonitor(cfg)
	m.AddPosition("BTC-USDT", "long", 1.0, 50000)

	// Set peak
	m.UpdatePrice("BTC-USDT", 60000)
	_ = m.GetAlerts()

	// Drop 11% of peak PnL -> critical
	// PnL = 10000 * 0.89 = 8900, price = 58900
	m.UpdatePrice("BTC-USDT", 58900)

	alerts := m.GetAlerts()
	found := false
	for _, a := range alerts {
		if a.Type == AlertDrawdown && a.Severity == SeverityCritical {
			found = true
		}
	}
	if !found {
		t.Error("expected a CRITICAL drawdown alert")
	}
}

func TestMonitorTrailingStop(t *testing.T) {
	cfg := MonitorConfig{
		DrawdownWarning:  1.0, // disable drawdown alerts
		DrawdownCritical: 1.0,
		TrailingStopPct:  0.03,
	}
	m := NewMonitor(cfg)
	m.AddPosition("BTC-USDT", "long", 1.0, 50000)

	// Set peak
	m.UpdatePrice("BTC-USDT", 60000) // PnL = 10000
	_ = m.GetAlerts()

	// Drop 4% of peak PnL -> trailing stop triggers (threshold = 3%)
	// PnL = 10000 * 0.96 = 9600, price = 59600
	m.UpdatePrice("BTC-USDT", 59600)

	alerts := m.GetAlerts()
	found := false
	for _, a := range alerts {
		if a.Type == AlertTrailingStop {
			found = true
		}
	}
	if !found {
		t.Error("expected a trailing stop alert")
	}
}

func TestMonitorUpdateTrailingStop(t *testing.T) {
	cfg := DefaultMonitorConfig()
	m := NewMonitor(cfg)

	m.UpdateTrailingStop(0.05)
	// Verify by exercising: add a position, set peak, test the new threshold
	m.AddPosition("BTC-USDT", "long", 1.0, 50000)
	m.UpdatePrice("BTC-USDT", 60000) // peak PnL = 10000
	_ = m.GetAlerts()

	// 4% drop - should NOT trigger with 5% trailing stop
	m.UpdatePrice("BTC-USDT", 59600) // PnL = 9600, drop = 4%
	alerts := m.GetAlerts()
	for _, a := range alerts {
		if a.Type == AlertTrailingStop {
			t.Error("trailing stop should not have triggered at 4% with 5% threshold")
		}
	}

	// 6% drop - SHOULD trigger
	m.UpdatePrice("BTC-USDT", 59400) // PnL = 9400, drop = 6%
	alerts = m.GetAlerts()
	found := false
	for _, a := range alerts {
		if a.Type == AlertTrailingStop {
			found = true
		}
	}
	if !found {
		t.Error("expected trailing stop at 6% drop with 5% threshold")
	}
}

func TestMonitorNoAlertWhenNoPeak(t *testing.T) {
	m := NewMonitor(DefaultMonitorConfig())
	m.AddPosition("BTC-USDT", "long", 1.0, 50000)

	// Price goes down immediately - no peak to draw down from
	m.UpdatePrice("BTC-USDT", 49000)
	alerts := m.GetAlerts()
	if len(alerts) != 0 {
		t.Errorf("expected no alerts when price drops from entry (no peak), got %d", len(alerts))
	}
}

func TestMonitorPositions(t *testing.T) {
	m := NewMonitor(DefaultMonitorConfig())
	m.AddPosition("BTC-USDT", "long", 1.0, 50000)
	m.AddPosition("ETH-USDT", "short", 10.0, 3000)

	positions := m.Positions()
	if len(positions) != 2 {
		t.Errorf("expected 2 positions, got %d", len(positions))
	}
}

func TestMonitorGetAlertsClears(t *testing.T) {
	cfg := MonitorConfig{
		DrawdownWarning:  0.01,
		DrawdownCritical: 1.0,
		TrailingStopPct:  0,
	}
	m := NewMonitor(cfg)
	m.AddPosition("BTC-USDT", "long", 1.0, 50000)
	m.UpdatePrice("BTC-USDT", 60000) // peak
	_ = m.GetAlerts()
	m.UpdatePrice("BTC-USDT", 59700) // 3% drop from peak -> warning

	alerts1 := m.GetAlerts()
	if len(alerts1) == 0 {
		t.Fatal("expected alerts")
	}

	// Second call should be empty
	alerts2 := m.GetAlerts()
	if len(alerts2) != 0 {
		t.Errorf("expected alerts to be cleared, got %d", len(alerts2))
	}
}

func TestMonitorUpdatePriceUnknownSymbol(t *testing.T) {
	m := NewMonitor(DefaultMonitorConfig())
	// Should not panic
	m.UpdatePrice("UNKNOWN", 100)
	alerts := m.GetAlerts()
	if len(alerts) != 0 {
		t.Error("expected no alerts for unknown symbol")
	}
}

func TestMonitorConcurrency(t *testing.T) {
	m := NewMonitor(DefaultMonitorConfig())
	m.AddPosition("BTC-USDT", "long", 1.0, 50000)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(price float64) {
			defer wg.Done()
			m.UpdatePrice("BTC-USDT", price)
		}(50000 + float64(i)*100)
	}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.GetAlerts()
		}()
	}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.Positions()
		}()
	}
	wg.Wait()
}
