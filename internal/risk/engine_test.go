package risk

import (
	"testing"
)

func TestEngine_AllowValidTrade(t *testing.T) {
	e := NewEngine(DefaultLimits())

	result := e.Check(
		TradeProposal{Symbol: "BTC", Side: "buy", Size: 0.01, Price: 70000, StopLoss: 69000},
		PortfolioState{Balance: 100000, OpenPositions: 0, TotalExposure: 0},
	)

	if !result.Allowed {
		t.Errorf("expected trade to be allowed, reasons: %v", result.Reasons)
	}
}

func TestEngine_RejectOversizedPosition(t *testing.T) {
	e := NewEngine(DefaultLimits())

	result := e.Check(
		TradeProposal{Symbol: "BTC", Side: "buy", Size: 1.0, Price: 70000},
		PortfolioState{Balance: 100000, OpenPositions: 0, TotalExposure: 0},
	)

	if result.Allowed {
		t.Error("expected oversized position to be rejected")
	}
}

func TestEngine_RejectMaxPositions(t *testing.T) {
	e := NewEngine(DefaultLimits())

	result := e.Check(
		TradeProposal{Symbol: "BTC", Side: "buy", Size: 0.001, Price: 70000},
		PortfolioState{Balance: 100000, OpenPositions: 5, TotalExposure: 10000},
	)

	if result.Allowed {
		t.Error("expected rejection when max positions reached")
	}
}

func TestEngine_RejectExcessiveRisk(t *testing.T) {
	e := NewEngine(DefaultLimits())

	// 0.1 BTC with stop loss 5000 away = $500 risk on $10000 balance = 5%
	result := e.Check(
		TradeProposal{Symbol: "BTC", Side: "buy", Size: 0.1, Price: 70000, StopLoss: 65000},
		PortfolioState{Balance: 10000, OpenPositions: 0, TotalExposure: 0},
	)

	if result.Allowed {
		t.Error("expected excessive risk to be rejected")
	}
}

func TestEngine_RejectDailyLossLimit(t *testing.T) {
	e := NewEngine(DefaultLimits())

	result := e.Check(
		TradeProposal{Symbol: "BTC", Side: "buy", Size: 0.001, Price: 70000},
		PortfolioState{Balance: 100000, OpenPositions: 0, TotalExposure: 0, DailyPnL: -6000},
	)

	if result.Allowed {
		t.Error("expected rejection when daily loss limit hit")
	}
}

func TestEngine_CalculatePositionSize(t *testing.T) {
	e := NewEngine(DefaultLimits())

	size := e.CalculatePositionSize(10000, 70000, 69000)
	if size <= 0 {
		t.Error("expected positive position size")
	}

	// Risk = size * (70000-69000) should be <= 2% of 10000 = 200
	risk := size * 1000
	if risk > 200.01 { // small float tolerance
		t.Errorf("risk $%.2f exceeds 2%% of balance ($200)", risk)
	}
}

func TestEngine_UpdateLimits(t *testing.T) {
	e := NewEngine(DefaultLimits())

	custom := RiskLimits{
		MaxPositionSizePct:  0.20,
		MaxTotalExposurePct: 0.80,
		MaxRiskPerTradePct:  0.05,
		MaxOpenPositions:    10,
		MaxDailyLossPct:     0.10,
		MaxOrderSize:        50000,
	}
	e.UpdateLimits(custom)

	got := e.GetLimits()
	if got.MaxPositionSizePct != 0.20 {
		t.Errorf("expected 0.20, got %f", got.MaxPositionSizePct)
	}
}
