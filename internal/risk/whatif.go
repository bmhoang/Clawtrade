// internal/risk/whatif.go
package risk

import (
	"math"
	"math/rand"
	"sort"
)

// Scenario represents a market scenario with a name, price change range, and probability weight.
type Scenario struct {
	Name        string  // e.g. "bull", "bear", "sideways"
	MinChangePct float64 // minimum price change per step as fraction (e.g. -0.05 = -5%)
	MaxChangePct float64 // maximum price change per step as fraction
	Weight       float64 // relative probability weight (will be normalised)
}

// SimulationConfig holds the parameters for a what-if simulation.
type SimulationConfig struct {
	EntryPrice   float64    // entry price of the position
	PositionSize float64    // quantity of the asset
	StopLoss     float64    // stop-loss price (must be below entry for long)
	TakeProfit   float64    // take-profit price (must be above entry for long)
	Iterations   int        // number of Monte Carlo paths
	Steps        int        // number of price steps per path (default 100)
	Scenarios    []Scenario // market scenarios to sample from
	Seed         int64      // random seed; 0 means use a random seed
}

// SimulationResult holds the aggregated results of a Monte Carlo simulation.
type SimulationResult struct {
	WinProbability float64            // fraction of paths that hit TP before SL
	LossProbability float64           // fraction of paths that hit SL before TP
	ExpectedPnL    float64            // mean PnL across all paths
	MaxDrawdown    float64            // worst drawdown observed across all paths (positive number)
	MedianPnL      float64            // 50th percentile PnL
	Percentile5    float64            // 5th percentile PnL (worst-case)
	Percentile25   float64            // 25th percentile PnL
	Percentile75   float64            // 75th percentile PnL
	Percentile95   float64            // 95th percentile PnL (best-case)
	AvgMaxDrawdown float64            // average max drawdown across paths
	ScenarioBreakdown map[string]float64 // win probability per scenario
}

// DefaultScenarios returns a standard set of bull / bear / sideways scenarios.
func DefaultScenarios() []Scenario {
	return []Scenario{
		{Name: "bull", MinChangePct: -0.01, MaxChangePct: 0.03, Weight: 1.0},
		{Name: "bear", MinChangePct: -0.03, MaxChangePct: 0.01, Weight: 1.0},
		{Name: "sideways", MinChangePct: -0.015, MaxChangePct: 0.015, Weight: 1.0},
	}
}

// WhatIfEngine runs Monte Carlo simulations for trade scenario analysis.
type WhatIfEngine struct{}

// NewWhatIfEngine creates a new WhatIfEngine.
func NewWhatIfEngine() *WhatIfEngine {
	return &WhatIfEngine{}
}

// Simulate runs a Monte Carlo simulation for the given config and returns aggregated results.
func (w *WhatIfEngine) Simulate(cfg SimulationConfig) SimulationResult {
	if cfg.Iterations <= 0 {
		cfg.Iterations = 1000
	}
	if cfg.Steps <= 0 {
		cfg.Steps = 100
	}
	if len(cfg.Scenarios) == 0 {
		cfg.Scenarios = DefaultScenarios()
	}

	// Normalise scenario weights into a cumulative distribution.
	totalWeight := 0.0
	for _, s := range cfg.Scenarios {
		totalWeight += s.Weight
	}
	cumWeights := make([]float64, len(cfg.Scenarios))
	running := 0.0
	for i, s := range cfg.Scenarios {
		running += s.Weight / totalWeight
		cumWeights[i] = running
	}

	rng := rand.New(rand.NewSource(cfg.Seed))
	if cfg.Seed == 0 {
		rng = rand.New(rand.NewSource(rand.Int63()))
	}

	pnls := make([]float64, cfg.Iterations)
	maxDrawdowns := make([]float64, cfg.Iterations)
	wins := 0
	losses := 0

	// Per-scenario tracking
	scenarioWins := make(map[string]int)
	scenarioCounts := make(map[string]int)

	for i := 0; i < cfg.Iterations; i++ {
		// Pick a scenario for this path.
		r := rng.Float64()
		scenarioIdx := 0
		for j, cw := range cumWeights {
			if r <= cw {
				scenarioIdx = j
				break
			}
		}
		sc := cfg.Scenarios[scenarioIdx]
		scenarioCounts[sc.Name]++

		price := cfg.EntryPrice
		peak := price
		pathMaxDrawdown := 0.0
		hitTP := false
		hitSL := false

		for step := 0; step < cfg.Steps; step++ {
			// Random walk: uniform change within the scenario range.
			changePct := sc.MinChangePct + rng.Float64()*(sc.MaxChangePct-sc.MinChangePct)
			price *= (1.0 + changePct)

			if price <= 0 {
				price = 0.001 // prevent negative prices
			}

			// Track drawdown from peak.
			if price > peak {
				peak = price
			}
			dd := (peak - price) / peak
			if dd > pathMaxDrawdown {
				pathMaxDrawdown = dd
			}

			// Check stop loss / take profit (long position assumed).
			if cfg.StopLoss > 0 && price <= cfg.StopLoss {
				price = cfg.StopLoss
				hitSL = true
				break
			}
			if cfg.TakeProfit > 0 && price >= cfg.TakeProfit {
				price = cfg.TakeProfit
				hitTP = true
				break
			}
		}

		pnl := (price - cfg.EntryPrice) * cfg.PositionSize
		pnls[i] = pnl
		maxDrawdowns[i] = pathMaxDrawdown

		if hitTP {
			wins++
			scenarioWins[sc.Name]++
		} else if hitSL {
			losses++
		} else {
			// Expired without hitting either level: count as win if profitable.
			if pnl > 0 {
				wins++
				scenarioWins[sc.Name]++
			} else {
				losses++
			}
		}
	}

	// Compute statistics.
	sort.Float64s(pnls)

	sumPnL := 0.0
	for _, p := range pnls {
		sumPnL += p
	}

	sumDD := 0.0
	globalMaxDD := 0.0
	for _, dd := range maxDrawdowns {
		sumDD += dd
		if dd > globalMaxDD {
			globalMaxDD = dd
		}
	}

	n := float64(cfg.Iterations)

	result := SimulationResult{
		WinProbability:  float64(wins) / n,
		LossProbability: float64(losses) / n,
		ExpectedPnL:     sumPnL / n,
		MaxDrawdown:     globalMaxDD,
		AvgMaxDrawdown:  sumDD / n,
		MedianPnL:       percentile(pnls, 0.50),
		Percentile5:     percentile(pnls, 0.05),
		Percentile25:    percentile(pnls, 0.25),
		Percentile75:    percentile(pnls, 0.75),
		Percentile95:    percentile(pnls, 0.95),
		ScenarioBreakdown: make(map[string]float64),
	}

	for name, count := range scenarioCounts {
		if count > 0 {
			result.ScenarioBreakdown[name] = float64(scenarioWins[name]) / float64(count)
		}
	}

	return result
}

// percentile returns the p-th percentile (0..1) from a sorted slice.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := p * float64(len(sorted)-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if lower == upper || upper >= len(sorted) {
		return sorted[lower]
	}
	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}
