package backtest

import (
	"math"
	"testing"
	"time"
)

const floatTol = 1e-6

func almostEqual(a, b, tol float64) bool {
	return math.Abs(a-b) < tol
}

func TestPortfolio_OpenAndClose(t *testing.T) {
	// capital=10000, makerFee=0.1%, takerFee=0.2%, slippage=0.05%
	p := NewPortfolio(10000, 0.001, 0.002, 0.0005)

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// Open a long position: 1 BTC at $5000
	err := p.OpenPosition("BTC", SideLong, 1.0, 5000.0, now)
	if err != nil {
		t.Fatalf("unexpected error opening position: %v", err)
	}

	// Verify cash deduction:
	// notional = 1 * 5000 = 5000
	// slip = 5000 * 0.0005 = 2.5
	// fee = (5000 + 2.5) * 0.002 = 10.005
	// totalCost = 5000 + 2.5 + 10.005 = 5012.505
	expectedCash := 10000.0 - 5012.505
	if !almostEqual(p.Cash, expectedCash, floatTol) {
		t.Errorf("after open: cash = %f, want %f", p.Cash, expectedCash)
	}

	if len(p.Positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(p.Positions))
	}

	// Close at $6000
	closeTime := now.Add(time.Hour)
	pnl := p.ClosePosition(0, 6000.0, closeTime)

	// Close calculation:
	// notional = 1 * 6000 = 6000
	// slip = 6000 * 0.0005 = 3.0
	// fee = (6000 - 3.0) * 0.002 = 11.994
	// netReceived = 6000 - 3.0 - 11.994 = 5985.006
	// PnL = netReceived - entryCost = 5985.006 - 5012.505 = 972.501
	expectedPnL := 5985.006 - 5012.505
	if !almostEqual(pnl, expectedPnL, floatTol) {
		t.Errorf("PnL = %f, want %f", pnl, expectedPnL)
	}

	// Cash should be initialCash - entryCost + netReceived
	expectedCashAfter := expectedCash + 5985.006
	if !almostEqual(p.Cash, expectedCashAfter, floatTol) {
		t.Errorf("after close: cash = %f, want %f", p.Cash, expectedCashAfter)
	}

	if len(p.Positions) != 0 {
		t.Errorf("expected 0 positions after close, got %d", len(p.Positions))
	}

	if len(p.Trades) != 1 {
		t.Fatalf("expected 1 trade record, got %d", len(p.Trades))
	}

	tr := p.Trades[0]
	if tr.Symbol != "BTC" || tr.Side != SideLong {
		t.Errorf("trade record symbol/side mismatch: %s/%s", tr.Symbol, tr.Side)
	}
	if !almostEqual(tr.PnL, expectedPnL, floatTol) {
		t.Errorf("trade record PnL = %f, want %f", tr.PnL, expectedPnL)
	}
}

func TestPortfolio_StopLoss(t *testing.T) {
	p := NewPortfolio(10000, 0.001, 0.002, 0.0005)
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	err := p.OpenPosition("ETH", SideLong, 2.0, 3000.0, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Set stop-loss at 2800
	p.Positions[0].StopLoss = 2800.0

	// Tick at price 2750 (below SL)
	tickTime := now.Add(30 * time.Minute)
	closed := p.Tick(2750.0, tickTime)

	if len(closed) != 1 {
		t.Fatalf("expected 1 closed trade from SL, got %d", len(closed))
	}

	// Should have closed at the SL price (2800), not the tick price
	tr := closed[0]
	if tr.ExitPrice != 2800.0 {
		t.Errorf("SL exit price = %f, want 2800", tr.ExitPrice)
	}

	// PnL should be negative (bought at 3000, sold at 2800)
	if tr.PnL >= 0 {
		t.Errorf("expected negative PnL for SL hit, got %f", tr.PnL)
	}

	if len(p.Positions) != 0 {
		t.Errorf("expected 0 positions after SL, got %d", len(p.Positions))
	}
}

func TestPortfolio_TakeProfit(t *testing.T) {
	p := NewPortfolio(10000, 0.001, 0.002, 0.0005)
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	err := p.OpenPosition("ETH", SideLong, 2.0, 3000.0, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Set take-profit at 3500
	p.Positions[0].TakeProfit = 3500.0

	// Tick at price 3600 (above TP)
	tickTime := now.Add(time.Hour)
	closed := p.Tick(3600.0, tickTime)

	if len(closed) != 1 {
		t.Fatalf("expected 1 closed trade from TP, got %d", len(closed))
	}

	tr := closed[0]
	if tr.ExitPrice != 3500.0 {
		t.Errorf("TP exit price = %f, want 3500", tr.ExitPrice)
	}

	// PnL should be positive (bought at 3000, sold at 3500)
	if tr.PnL <= 0 {
		t.Errorf("expected positive PnL for TP hit, got %f", tr.PnL)
	}

	if len(p.Positions) != 0 {
		t.Errorf("expected 0 positions after TP, got %d", len(p.Positions))
	}
}

func TestPortfolio_Equity(t *testing.T) {
	p := NewPortfolio(10000, 0.001, 0.002, 0.0005)
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// With no positions, equity = cash = initial capital
	eq := p.Equity(5000.0)
	if !almostEqual(eq, 10000.0, floatTol) {
		t.Errorf("equity with no positions = %f, want 10000", eq)
	}

	// Open a long position: 1 unit at 5000
	err := p.OpenPosition("BTC", SideLong, 1.0, 5000.0, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Equity at current price 5500:
	// cash (after open) + size * currentPrice
	// The position is worth size * currentPrice = 1 * 5500 = 5500
	expectedEquity := p.Cash + 1.0*5500.0
	eq = p.Equity(5500.0)
	if !almostEqual(eq, expectedEquity, floatTol) {
		t.Errorf("equity = %f, want %f", eq, expectedEquity)
	}
}
