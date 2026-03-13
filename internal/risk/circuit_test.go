package risk

import (
	"sync"
	"testing"
	"time"
)

func defaultCircuitConfig() CircuitConfig {
	return CircuitConfig{
		MaxDailyLossPct:      0.05, // 5%
		MaxConsecutiveLosses: 3,
		CooldownMinutes:      0, // no cooldown for most tests
		StartingBalance:      10000,
	}
}

func TestCircuitBreaker_InitialState(t *testing.T) {
	cb := NewCircuitBreaker(defaultCircuitConfig())

	if cb.IsHalted() {
		t.Fatal("new circuit breaker should not be halted")
	}
	st := cb.GetState()
	if st.State != CircuitActive {
		t.Fatalf("expected ACTIVE, got %s", st.State)
	}
	if st.TradeCount != 0 {
		t.Fatalf("expected 0 trades, got %d", st.TradeCount)
	}
}

func TestCircuitBreaker_DailyLossHalt(t *testing.T) {
	cfg := defaultCircuitConfig()
	cfg.MaxConsecutiveLosses = 0 // disable consecutive check for this test
	cb := NewCircuitBreaker(cfg)

	// Record losses totaling 5% of 10000 = 500
	cb.RecordTrade(-200)
	if cb.IsHalted() {
		t.Fatal("should not be halted after -200 (2% loss)")
	}

	cb.RecordTrade(-150)
	if cb.IsHalted() {
		t.Fatal("should not be halted after -350 (3.5% loss)")
	}

	cb.RecordTrade(-150) // total = -500 = 5%
	if !cb.IsHalted() {
		t.Fatal("should be halted after reaching 5% daily loss")
	}

	st := cb.GetState()
	if st.State != CircuitHaltedDailyLoss {
		t.Fatalf("expected HALTED_DAILY_LOSS, got %s", st.State)
	}
	if st.DailyPnL != -500 {
		t.Fatalf("expected daily PnL -500, got %f", st.DailyPnL)
	}
}

func TestCircuitBreaker_ConsecutiveLossHalt(t *testing.T) {
	cb := NewCircuitBreaker(defaultCircuitConfig())

	// 3 consecutive losses with small amounts (don't trigger daily limit)
	cb.RecordTrade(-10)
	cb.RecordTrade(-10)
	if cb.IsHalted() {
		t.Fatal("should not be halted after 2 consecutive losses")
	}

	cb.RecordTrade(-10) // 3rd consecutive loss
	if !cb.IsHalted() {
		t.Fatal("should be halted after 3 consecutive losses")
	}

	st := cb.GetState()
	if st.State != CircuitHaltedConsecutive {
		t.Fatalf("expected HALTED_CONSECUTIVE, got %s", st.State)
	}
	if st.ConsecutiveLosses != 3 {
		t.Fatalf("expected 3 consecutive losses, got %d", st.ConsecutiveLosses)
	}
}

func TestCircuitBreaker_ConsecutiveLossResetOnWin(t *testing.T) {
	cb := NewCircuitBreaker(defaultCircuitConfig())

	cb.RecordTrade(-10)
	cb.RecordTrade(-10)
	// A winning trade resets the counter
	cb.RecordTrade(50)
	cb.RecordTrade(-10)
	cb.RecordTrade(-10)

	if cb.IsHalted() {
		t.Fatal("should not be halted; consecutive count was reset by a win")
	}

	st := cb.GetState()
	if st.ConsecutiveLosses != 2 {
		t.Fatalf("expected 2 consecutive losses, got %d", st.ConsecutiveLosses)
	}
}

func TestCircuitBreaker_PanicButton(t *testing.T) {
	cb := NewCircuitBreaker(defaultCircuitConfig())

	cb.Panic()
	if !cb.IsHalted() {
		t.Fatal("should be halted after panic")
	}

	st := cb.GetState()
	if st.State != CircuitHaltedPanic {
		t.Fatalf("expected HALTED_PANIC, got %s", st.State)
	}

	// Trades should not be recorded during panic
	cb.RecordTrade(-10)
	st = cb.GetState()
	if st.TradeCount != 0 {
		t.Fatal("trades should not be recorded during panic")
	}
}

func TestCircuitBreaker_PanicNotClearableByCooldown(t *testing.T) {
	cfg := defaultCircuitConfig()
	cfg.CooldownMinutes = 1
	cb := NewCircuitBreaker(cfg)

	cb.Panic()
	if !cb.IsHalted() {
		t.Fatal("should be halted after panic")
	}

	// Panic state should not have cooldown
	st := cb.GetState()
	if st.State != CircuitHaltedPanic {
		t.Fatalf("expected HALTED_PANIC, got %s", st.State)
	}
	if !st.CooldownEndsAt.IsZero() {
		t.Fatal("panic should not set a cooldown end time")
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(defaultCircuitConfig())

	cb.RecordTrade(-10)
	cb.RecordTrade(-10)
	cb.RecordTrade(-10) // triggers consecutive halt

	if !cb.IsHalted() {
		t.Fatal("should be halted")
	}

	cb.Reset()
	if cb.IsHalted() {
		t.Fatal("should not be halted after reset")
	}

	st := cb.GetState()
	if st.State != CircuitActive {
		t.Fatalf("expected ACTIVE after reset, got %s", st.State)
	}
	if st.DailyPnL != 0 || st.ConsecutiveLosses != 0 || st.TradeCount != 0 {
		t.Fatal("reset should clear all counters")
	}
}

func TestCircuitBreaker_ResetFromPanic(t *testing.T) {
	cb := NewCircuitBreaker(defaultCircuitConfig())

	cb.Panic()
	if !cb.IsHalted() {
		t.Fatal("should be halted after panic")
	}

	cb.Reset()
	if cb.IsHalted() {
		t.Fatal("reset should clear panic state")
	}
}

func TestCircuitBreaker_Cooldown(t *testing.T) {
	cfg := defaultCircuitConfig()
	cfg.CooldownMinutes = 5
	cb := NewCircuitBreaker(cfg)

	fakeNow := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cb.now = func() time.Time { return fakeNow }

	// Trigger consecutive loss halt
	cb.RecordTrade(-10)
	cb.RecordTrade(-10)
	cb.RecordTrade(-10)

	if !cb.IsHalted() {
		t.Fatal("should be halted")
	}

	st := cb.GetState()
	if st.State != CircuitCooldown {
		t.Fatalf("expected COOLDOWN, got %s", st.State)
	}

	// Still halted at 4 minutes
	fakeNow = fakeNow.Add(4 * time.Minute)
	if !cb.IsHalted() {
		t.Fatal("should still be halted during cooldown")
	}

	// Not halted at 6 minutes
	fakeNow = fakeNow.Add(2 * time.Minute) // total 6 minutes
	if cb.IsHalted() {
		t.Fatal("should not be halted after cooldown expires")
	}

	st = cb.GetState()
	if st.State != CircuitActive {
		t.Fatalf("expected ACTIVE after cooldown, got %s", st.State)
	}
}

func TestCircuitBreaker_CooldownResumesAndCanHaltAgain(t *testing.T) {
	cfg := defaultCircuitConfig()
	cfg.CooldownMinutes = 1
	cb := NewCircuitBreaker(cfg)

	fakeNow := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cb.now = func() time.Time { return fakeNow }

	// Trigger halt
	cb.RecordTrade(-10)
	cb.RecordTrade(-10)
	cb.RecordTrade(-10)
	if !cb.IsHalted() {
		t.Fatal("should be halted")
	}

	// Advance past cooldown
	fakeNow = fakeNow.Add(2 * time.Minute)

	// Should be able to trade again
	if cb.IsHalted() {
		t.Fatal("should be active after cooldown")
	}

	// New trade resumes, another set of losses should halt again
	cb.RecordTrade(-10)
	cb.RecordTrade(-10)
	cb.RecordTrade(-10)
	if !cb.IsHalted() {
		t.Fatal("should be halted again after more consecutive losses")
	}
}

func TestCircuitBreaker_TradesIgnoredWhileHalted(t *testing.T) {
	cb := NewCircuitBreaker(defaultCircuitConfig())

	cb.RecordTrade(-10)
	cb.RecordTrade(-10)
	cb.RecordTrade(-10) // halt

	cb.RecordTrade(-1000) // should be ignored
	st := cb.GetState()
	if st.TradeCount != 3 {
		t.Fatalf("expected 3 trades, got %d (trade during halt should be ignored)", st.TradeCount)
	}
	if st.DailyPnL != -30 {
		t.Fatalf("expected PnL -30, got %f (trade during halt should be ignored)", st.DailyPnL)
	}
}

func TestCircuitBreaker_DailyLossWithMixedTrades(t *testing.T) {
	cb := NewCircuitBreaker(defaultCircuitConfig())

	cb.RecordTrade(100)   // +100
	cb.RecordTrade(-300)  // -200 net
	cb.RecordTrade(50)    // -150 net
	cb.RecordTrade(-400)  // -550 net = 5.5% of 10000

	if !cb.IsHalted() {
		t.Fatal("should be halted; net loss is 5.5%")
	}

	st := cb.GetState()
	if st.State != CircuitHaltedDailyLoss {
		t.Fatalf("expected HALTED_DAILY_LOSS, got %s", st.State)
	}
}

func TestCircuitBreaker_NoDailyLossHaltWhenProfitable(t *testing.T) {
	cb := NewCircuitBreaker(defaultCircuitConfig())

	cb.RecordTrade(500)
	cb.RecordTrade(-200)
	// net +300, should not halt for daily loss
	if cb.IsHalted() {
		t.Fatal("should not be halted when net profitable")
	}
}

func TestCircuitBreaker_UpdateConfig(t *testing.T) {
	cb := NewCircuitBreaker(defaultCircuitConfig())

	newCfg := CircuitConfig{
		MaxDailyLossPct:      0.10,
		MaxConsecutiveLosses: 5,
		CooldownMinutes:      10,
		StartingBalance:      20000,
	}
	cb.UpdateConfig(newCfg)

	// With new config, 3 consecutive losses should not halt
	cb.RecordTrade(-10)
	cb.RecordTrade(-10)
	cb.RecordTrade(-10)
	if cb.IsHalted() {
		t.Fatal("should not be halted with new config (max 5 consecutive)")
	}
}

func TestCircuitBreaker_ThreadSafety(t *testing.T) {
	cb := NewCircuitBreaker(CircuitConfig{
		MaxDailyLossPct:      0.50, // high threshold so it doesn't halt quickly
		MaxConsecutiveLosses: 1000,
		CooldownMinutes:      0,
		StartingBalance:      100000,
	})

	var wg sync.WaitGroup
	iterations := 100

	// Concurrent RecordTrade calls
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				cb.RecordTrade(-1)
			}
		}()
	}

	// Concurrent IsHalted reads
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = cb.IsHalted()
			}
		}()
	}

	// Concurrent GetState reads
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = cb.GetState()
			}
		}()
	}

	wg.Wait()

	st := cb.GetState()
	if st.TradeCount == 0 {
		t.Fatal("expected some trades to be recorded")
	}
}

func TestCircuitBreaker_ZeroConsecutiveLossesConfig(t *testing.T) {
	cfg := defaultCircuitConfig()
	cfg.MaxConsecutiveLosses = 0 // disabled
	cb := NewCircuitBreaker(cfg)

	cb.RecordTrade(-10)
	cb.RecordTrade(-10)
	cb.RecordTrade(-10)
	cb.RecordTrade(-10)

	// Should not halt for consecutive losses (only daily loss can trigger)
	if cb.IsHalted() {
		st := cb.GetState()
		if st.State == CircuitHaltedConsecutive {
			t.Fatal("consecutive loss check should be disabled when MaxConsecutiveLosses is 0")
		}
	}
}

func TestCircuitBreaker_ZeroPnlTrade(t *testing.T) {
	cb := NewCircuitBreaker(defaultCircuitConfig())

	cb.RecordTrade(-10)
	cb.RecordTrade(-10)
	cb.RecordTrade(0) // breakeven resets consecutive counter
	cb.RecordTrade(-10)
	cb.RecordTrade(-10)

	if cb.IsHalted() {
		st := cb.GetState()
		if st.State == CircuitHaltedConsecutive {
			t.Fatal("breakeven trade should reset consecutive loss counter")
		}
	}

	st := cb.GetState()
	if st.ConsecutiveLosses != 2 {
		t.Fatalf("expected 2 consecutive losses after breakeven reset, got %d", st.ConsecutiveLosses)
	}
}

func TestCircuitBreaker_GetStateDailyLossPct(t *testing.T) {
	cb := NewCircuitBreaker(defaultCircuitConfig())

	cb.RecordTrade(-300) // 3% loss
	st := cb.GetState()

	expectedPct := 0.03
	if st.DailyLossPct < expectedPct-0.001 || st.DailyLossPct > expectedPct+0.001 {
		t.Fatalf("expected daily loss pct ~0.03, got %f", st.DailyLossPct)
	}
}
