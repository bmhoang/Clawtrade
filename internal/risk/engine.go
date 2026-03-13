// internal/risk/engine.go
package risk

import (
	"fmt"
	"sync"
)

// RiskLimits defines the risk parameters
type RiskLimits struct {
	MaxPositionSizePct  float64 // max % of balance per position (e.g., 0.1 = 10%)
	MaxTotalExposurePct float64 // max % of balance in all positions (e.g., 0.5 = 50%)
	MaxRiskPerTradePct  float64 // max % of balance risked per trade (e.g., 0.02 = 2%)
	MaxOpenPositions    int     // max number of concurrent positions
	MaxDailyLossPct     float64 // max daily loss before halt (e.g., 0.05 = 5%)
	MaxOrderSize        float64 // absolute max order size in quote currency
}

// DefaultLimits returns conservative risk limits
func DefaultLimits() RiskLimits {
	return RiskLimits{
		MaxPositionSizePct:  0.10,
		MaxTotalExposurePct: 0.50,
		MaxRiskPerTradePct:  0.02,
		MaxOpenPositions:    5,
		MaxDailyLossPct:     0.05,
		MaxOrderSize:        10000,
	}
}

// TradeProposal represents a trade to be validated
type TradeProposal struct {
	Symbol   string
	Side     string  // "buy" or "sell"
	Size     float64 // quantity
	Price    float64 // entry price
	StopLoss float64 // stop loss price (0 if none)
}

// CheckResult holds the result of a risk check
type CheckResult struct {
	Allowed bool     `json:"allowed"`
	Reasons []string `json:"reasons,omitempty"`
	RiskPct float64  `json:"risk_pct"` // risk as % of balance
	SizePct float64  `json:"size_pct"` // position size as % of balance
}

// PortfolioState represents current portfolio for risk calculations
type PortfolioState struct {
	Balance       float64
	OpenPositions int
	TotalExposure float64 // total value of open positions
	DailyPnL      float64 // realized PnL today
}

// Engine performs pre-trade risk checks
type Engine struct {
	mu     sync.RWMutex
	limits RiskLimits
}

// NewEngine creates a new risk engine with the given limits
func NewEngine(limits RiskLimits) *Engine {
	return &Engine{limits: limits}
}

// UpdateLimits updates the risk limits
func (e *Engine) UpdateLimits(limits RiskLimits) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.limits = limits
}

// GetLimits returns current risk limits
func (e *Engine) GetLimits() RiskLimits {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.limits
}

// Check validates a trade proposal against risk limits
func (e *Engine) Check(proposal TradeProposal, portfolio PortfolioState) CheckResult {
	e.mu.RLock()
	limits := e.limits
	e.mu.RUnlock()

	result := CheckResult{Allowed: true}

	if portfolio.Balance <= 0 {
		result.Allowed = false
		result.Reasons = append(result.Reasons, "zero or negative balance")
		return result
	}

	orderValue := proposal.Size * proposal.Price
	result.SizePct = orderValue / portfolio.Balance

	// Check 1: Position size
	if result.SizePct > limits.MaxPositionSizePct {
		result.Allowed = false
		result.Reasons = append(result.Reasons,
			fmt.Sprintf("position size %.1f%% exceeds limit %.1f%%",
				result.SizePct*100, limits.MaxPositionSizePct*100))
	}

	// Check 2: Absolute max order size
	if limits.MaxOrderSize > 0 && orderValue > limits.MaxOrderSize {
		result.Allowed = false
		result.Reasons = append(result.Reasons,
			fmt.Sprintf("order value $%.2f exceeds max $%.2f", orderValue, limits.MaxOrderSize))
	}

	// Check 3: Total exposure
	newExposure := (portfolio.TotalExposure + orderValue) / portfolio.Balance
	if newExposure > limits.MaxTotalExposurePct {
		result.Allowed = false
		result.Reasons = append(result.Reasons,
			fmt.Sprintf("total exposure %.1f%% would exceed limit %.1f%%",
				newExposure*100, limits.MaxTotalExposurePct*100))
	}

	// Check 4: Open positions count
	if portfolio.OpenPositions >= limits.MaxOpenPositions {
		result.Allowed = false
		result.Reasons = append(result.Reasons,
			fmt.Sprintf("max open positions %d reached", limits.MaxOpenPositions))
	}

	// Check 5: Risk per trade (using stop loss)
	if proposal.StopLoss > 0 {
		var riskAmount float64
		if proposal.Side == "buy" {
			riskAmount = (proposal.Price - proposal.StopLoss) * proposal.Size
		} else {
			riskAmount = (proposal.StopLoss - proposal.Price) * proposal.Size
		}
		if riskAmount > 0 {
			result.RiskPct = riskAmount / portfolio.Balance
			if result.RiskPct > limits.MaxRiskPerTradePct {
				result.Allowed = false
				result.Reasons = append(result.Reasons,
					fmt.Sprintf("trade risk %.2f%% exceeds max %.2f%%",
						result.RiskPct*100, limits.MaxRiskPerTradePct*100))
			}
		}
	}

	// Check 6: Daily loss limit
	if portfolio.DailyPnL < 0 {
		dailyLossPct := -portfolio.DailyPnL / portfolio.Balance
		if dailyLossPct >= limits.MaxDailyLossPct {
			result.Allowed = false
			result.Reasons = append(result.Reasons,
				fmt.Sprintf("daily loss %.2f%% reached limit %.2f%%",
					dailyLossPct*100, limits.MaxDailyLossPct*100))
		}
	}

	return result
}

// CalculatePositionSize returns the recommended position size based on risk limits
func (e *Engine) CalculatePositionSize(balance, entryPrice, stopLoss float64) float64 {
	e.mu.RLock()
	limits := e.limits
	e.mu.RUnlock()

	if entryPrice <= 0 || stopLoss <= 0 {
		return 0
	}

	riskPerUnit := entryPrice - stopLoss
	if riskPerUnit < 0 {
		riskPerUnit = -riskPerUnit
	}
	if riskPerUnit == 0 {
		return 0
	}

	// Max risk amount in currency
	maxRisk := balance * limits.MaxRiskPerTradePct

	// Position size from risk
	sizeFromRisk := maxRisk / riskPerUnit

	// Also check position size limit
	maxPositionValue := balance * limits.MaxPositionSizePct
	sizeFromPosition := maxPositionValue / entryPrice

	// Take the smaller
	size := sizeFromRisk
	if sizeFromPosition < size {
		size = sizeFromPosition
	}

	return size
}
