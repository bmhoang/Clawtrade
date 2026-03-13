package risk

import (
	"math"
	"testing"
)

func TestWhatIfBasicSimulation(t *testing.T) {
	engine := NewWhatIfEngine()

	cfg := SimulationConfig{
		EntryPrice:   100.0,
		PositionSize: 10.0,
		StopLoss:     90.0,
		TakeProfit:   110.0,
		Iterations:   5000,
		Steps:        50,
		Seed:         42,
	}

	result := engine.Simulate(cfg)

	if result.WinProbability < 0 || result.WinProbability > 1 {
		t.Errorf("win probability out of range: %f", result.WinProbability)
	}
	if result.LossProbability < 0 || result.LossProbability > 1 {
		t.Errorf("loss probability out of range: %f", result.LossProbability)
	}
	if result.MaxDrawdown < 0 {
		t.Errorf("max drawdown should be non-negative, got %f", result.MaxDrawdown)
	}

	t.Logf("Win prob: %.2f%%, Loss prob: %.2f%%, ExpPnL: %.2f, MaxDD: %.4f",
		result.WinProbability*100, result.LossProbability*100,
		result.ExpectedPnL, result.MaxDrawdown)
	t.Logf("Percentiles: P5=%.2f P25=%.2f P50=%.2f P75=%.2f P95=%.2f",
		result.Percentile5, result.Percentile25, result.MedianPnL,
		result.Percentile75, result.Percentile95)
}

func TestWhatIfTightVsWideSL(t *testing.T) {
	engine := NewWhatIfEngine()

	// Tight stop loss: 2% below entry.
	tightCfg := SimulationConfig{
		EntryPrice:   100.0,
		PositionSize: 10.0,
		StopLoss:     98.0,
		TakeProfit:   110.0,
		Iterations:   10000,
		Steps:        100,
		Seed:         123,
	}

	// Wide stop loss: 20% below entry.
	wideCfg := SimulationConfig{
		EntryPrice:   100.0,
		PositionSize: 10.0,
		StopLoss:     80.0,
		TakeProfit:   110.0,
		Iterations:   10000,
		Steps:        100,
		Seed:         123,
	}

	tightResult := engine.Simulate(tightCfg)
	wideResult := engine.Simulate(wideCfg)

	// With a tight SL, the loss probability should be higher than with a wide SL.
	if tightResult.LossProbability <= wideResult.LossProbability {
		t.Errorf("tight SL should have higher loss probability (%.4f) than wide SL (%.4f)",
			tightResult.LossProbability, wideResult.LossProbability)
	}

	// Correspondingly, the win probability with a wide SL should be higher.
	if wideResult.WinProbability <= tightResult.WinProbability {
		t.Errorf("wide SL should have higher win probability (%.4f) than tight SL (%.4f)",
			wideResult.WinProbability, tightResult.WinProbability)
	}

	t.Logf("Tight SL: WinP=%.2f%% LossP=%.2f%% ExpPnL=%.2f",
		tightResult.WinProbability*100, tightResult.LossProbability*100, tightResult.ExpectedPnL)
	t.Logf("Wide  SL: WinP=%.2f%% LossP=%.2f%% ExpPnL=%.2f",
		wideResult.WinProbability*100, wideResult.LossProbability*100, wideResult.ExpectedPnL)
}

func TestWhatIfBullVsBear(t *testing.T) {
	engine := NewWhatIfEngine()

	bullOnly := SimulationConfig{
		EntryPrice:   100.0,
		PositionSize: 10.0,
		StopLoss:     90.0,
		TakeProfit:   115.0,
		Iterations:   10000,
		Steps:        100,
		Seed:         99,
		Scenarios: []Scenario{
			{Name: "bull", MinChangePct: 0.0, MaxChangePct: 0.03, Weight: 1.0},
		},
	}

	bearOnly := SimulationConfig{
		EntryPrice:   100.0,
		PositionSize: 10.0,
		StopLoss:     90.0,
		TakeProfit:   115.0,
		Iterations:   10000,
		Steps:        100,
		Seed:         99,
		Scenarios: []Scenario{
			{Name: "bear", MinChangePct: -0.03, MaxChangePct: 0.0, Weight: 1.0},
		},
	}

	bullResult := engine.Simulate(bullOnly)
	bearResult := engine.Simulate(bearOnly)

	if bullResult.WinProbability <= bearResult.WinProbability {
		t.Errorf("bull scenario should win more often (%.4f) than bear (%.4f)",
			bullResult.WinProbability, bearResult.WinProbability)
	}

	if bullResult.ExpectedPnL <= bearResult.ExpectedPnL {
		t.Errorf("bull expected PnL (%.2f) should exceed bear (%.2f)",
			bullResult.ExpectedPnL, bearResult.ExpectedPnL)
	}

	t.Logf("Bull: WinP=%.2f%% ExpPnL=%.2f", bullResult.WinProbability*100, bullResult.ExpectedPnL)
	t.Logf("Bear: WinP=%.2f%% ExpPnL=%.2f", bearResult.WinProbability*100, bearResult.ExpectedPnL)
}

func TestWhatIfPercentileOrdering(t *testing.T) {
	engine := NewWhatIfEngine()

	cfg := SimulationConfig{
		EntryPrice:   100.0,
		PositionSize: 10.0,
		StopLoss:     85.0,
		TakeProfit:   120.0,
		Iterations:   10000,
		Steps:        100,
		Seed:         7,
	}

	result := engine.Simulate(cfg)

	// Percentiles must be in non-decreasing order.
	if result.Percentile5 > result.Percentile25 {
		t.Errorf("P5 (%.2f) > P25 (%.2f)", result.Percentile5, result.Percentile25)
	}
	if result.Percentile25 > result.MedianPnL {
		t.Errorf("P25 (%.2f) > P50 (%.2f)", result.Percentile25, result.MedianPnL)
	}
	if result.MedianPnL > result.Percentile75 {
		t.Errorf("P50 (%.2f) > P75 (%.2f)", result.MedianPnL, result.Percentile75)
	}
	if result.Percentile75 > result.Percentile95 {
		t.Errorf("P75 (%.2f) > P95 (%.2f)", result.Percentile75, result.Percentile95)
	}
}

func TestWhatIfScenarioBreakdown(t *testing.T) {
	engine := NewWhatIfEngine()

	cfg := SimulationConfig{
		EntryPrice:   100.0,
		PositionSize: 10.0,
		StopLoss:     90.0,
		TakeProfit:   110.0,
		Iterations:   10000,
		Steps:        50,
		Seed:         55,
		Scenarios:    DefaultScenarios(),
	}

	result := engine.Simulate(cfg)

	if len(result.ScenarioBreakdown) == 0 {
		t.Fatal("scenario breakdown should not be empty")
	}

	for name, winP := range result.ScenarioBreakdown {
		if winP < 0 || winP > 1 {
			t.Errorf("scenario %s win probability out of range: %f", name, winP)
		}
		t.Logf("Scenario %s: WinP=%.2f%%", name, winP*100)
	}

	// Bull scenario should have higher win probability than bear.
	bullWin, bullOk := result.ScenarioBreakdown["bull"]
	bearWin, bearOk := result.ScenarioBreakdown["bear"]
	if bullOk && bearOk && bullWin <= bearWin {
		t.Errorf("bull win rate (%.4f) should be higher than bear (%.4f)", bullWin, bearWin)
	}
}

func TestWhatIfDeterministic(t *testing.T) {
	engine := NewWhatIfEngine()

	cfg := SimulationConfig{
		EntryPrice:   100.0,
		PositionSize: 10.0,
		StopLoss:     90.0,
		TakeProfit:   110.0,
		Iterations:   1000,
		Steps:        50,
		Seed:         42,
	}

	r1 := engine.Simulate(cfg)
	r2 := engine.Simulate(cfg)

	if math.Abs(r1.WinProbability-r2.WinProbability) > 1e-9 {
		t.Errorf("same seed should produce identical results: %f vs %f",
			r1.WinProbability, r2.WinProbability)
	}
	if math.Abs(r1.ExpectedPnL-r2.ExpectedPnL) > 1e-9 {
		t.Errorf("same seed should produce identical ExpectedPnL: %f vs %f",
			r1.ExpectedPnL, r2.ExpectedPnL)
	}
}

func TestWhatIfDefaults(t *testing.T) {
	engine := NewWhatIfEngine()

	// Minimal config: should use defaults for iterations, steps, scenarios.
	cfg := SimulationConfig{
		EntryPrice:   50.0,
		PositionSize: 5.0,
		StopLoss:     45.0,
		TakeProfit:   55.0,
		Seed:         1,
	}

	result := engine.Simulate(cfg)

	if result.WinProbability == 0 && result.LossProbability == 0 {
		t.Error("simulation with defaults should produce non-trivial results")
	}
}

func TestWhatIfMaxDrawdownPositive(t *testing.T) {
	engine := NewWhatIfEngine()

	cfg := SimulationConfig{
		EntryPrice:   100.0,
		PositionSize: 10.0,
		StopLoss:     50.0,
		TakeProfit:   200.0,
		Iterations:   5000,
		Steps:        200,
		Seed:         77,
	}

	result := engine.Simulate(cfg)

	if result.MaxDrawdown <= 0 {
		t.Errorf("max drawdown should be positive with volatile paths, got %f", result.MaxDrawdown)
	}
	if result.AvgMaxDrawdown <= 0 {
		t.Errorf("avg max drawdown should be positive, got %f", result.AvgMaxDrawdown)
	}
	if result.AvgMaxDrawdown > result.MaxDrawdown {
		t.Errorf("avg max drawdown (%.4f) should not exceed global max drawdown (%.4f)",
			result.AvgMaxDrawdown, result.MaxDrawdown)
	}

	t.Logf("MaxDD: %.4f, AvgMaxDD: %.4f", result.MaxDrawdown, result.AvgMaxDrawdown)
}
