// internal/risk/circuit.go
package risk

import (
	"sync"
	"time"
)

// CircuitState represents the current state of the circuit breaker.
type CircuitState string

const (
	CircuitActive             CircuitState = "ACTIVE"
	CircuitHaltedDailyLoss    CircuitState = "HALTED_DAILY_LOSS"
	CircuitHaltedConsecutive  CircuitState = "HALTED_CONSECUTIVE"
	CircuitHaltedPanic        CircuitState = "HALTED_PANIC"
	CircuitCooldown           CircuitState = "COOLDOWN"
)

// CircuitConfig holds configuration for the circuit breaker.
type CircuitConfig struct {
	MaxDailyLossPct      float64 // max daily loss as fraction (e.g., 0.05 = 5%)
	MaxConsecutiveLosses int     // max consecutive losing trades before halt
	CooldownMinutes      int     // minutes to wait before auto-resuming
	StartingBalance      float64 // balance at start of day for daily loss calculation
}

// CircuitStatus describes the current circuit breaker state with context.
type CircuitStatus struct {
	State              CircuitState `json:"state"`
	Reason             string       `json:"reason"`
	DailyPnL           float64      `json:"daily_pnl"`
	DailyLossPct       float64      `json:"daily_loss_pct"`
	ConsecutiveLosses  int          `json:"consecutive_losses"`
	TradeCount         int          `json:"trade_count"`
	HaltedAt           time.Time    `json:"halted_at,omitempty"`
	CooldownEndsAt     time.Time    `json:"cooldown_ends_at,omitempty"`
}

// CircuitBreaker monitors trade outcomes and halts trading when thresholds
// are breached. It is safe for concurrent use.
type CircuitBreaker struct {
	mu     sync.RWMutex
	config CircuitConfig

	state             CircuitState
	reason            string
	dailyPnL          float64
	consecutiveLosses int
	tradeCount        int
	haltedAt          time.Time
	cooldownEndsAt    time.Time

	// now is a function that returns the current time; overridable for testing.
	now func() time.Time
}

// NewCircuitBreaker creates a circuit breaker with the given configuration.
func NewCircuitBreaker(cfg CircuitConfig) *CircuitBreaker {
	return &CircuitBreaker{
		config: cfg,
		state:  CircuitActive,
		now:    time.Now,
	}
}

// RecordTrade records a trade outcome (positive = profit, negative = loss)
// and checks whether any halt thresholds have been breached.
func (cb *CircuitBreaker) RecordTrade(pnl float64) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// If in panic state, reject all trades silently.
	if cb.state == CircuitHaltedPanic {
		return
	}

	// If in cooldown, check whether the cooldown has expired.
	if cb.state == CircuitCooldown {
		if cb.now().Before(cb.cooldownEndsAt) {
			return
		}
		// Cooldown expired, resume.
		cb.state = CircuitActive
		cb.reason = ""
	}

	// If halted (non-panic, non-cooldown), don't record.
	if cb.state != CircuitActive {
		return
	}

	cb.tradeCount++
	cb.dailyPnL += pnl

	// Track consecutive losses.
	if pnl < 0 {
		cb.consecutiveLosses++
	} else {
		cb.consecutiveLosses = 0
	}

	// Check consecutive loss threshold.
	if cb.config.MaxConsecutiveLosses > 0 && cb.consecutiveLosses >= cb.config.MaxConsecutiveLosses {
		cb.halt(CircuitHaltedConsecutive, "consecutive loss limit reached")
		return
	}

	// Check daily loss threshold.
	if cb.config.StartingBalance > 0 && cb.config.MaxDailyLossPct > 0 && cb.dailyPnL < 0 {
		lossPct := -cb.dailyPnL / cb.config.StartingBalance
		if lossPct >= cb.config.MaxDailyLossPct {
			cb.halt(CircuitHaltedDailyLoss, "daily loss limit reached")
			return
		}
	}
}

// halt transitions to a halted state with an optional cooldown.
func (cb *CircuitBreaker) halt(state CircuitState, reason string) {
	cb.haltedAt = cb.now()
	if cb.config.CooldownMinutes > 0 && state != CircuitHaltedPanic {
		cb.state = CircuitCooldown
		cb.reason = reason + " (cooldown)"
		cb.cooldownEndsAt = cb.haltedAt.Add(time.Duration(cb.config.CooldownMinutes) * time.Minute)
	} else {
		cb.state = state
		cb.reason = reason
	}
}

// IsHalted returns true if trading should be stopped.
func (cb *CircuitBreaker) IsHalted() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case CircuitHaltedDailyLoss, CircuitHaltedConsecutive, CircuitHaltedPanic:
		return true
	case CircuitCooldown:
		if cb.now().Before(cb.cooldownEndsAt) {
			return true
		}
		// Cooldown has expired; will be set to active on next write operation.
		// For the read path, report as not halted.
		return false
	default:
		return false
	}
}

// GetState returns the current circuit breaker status.
func (cb *CircuitBreaker) GetState() CircuitStatus {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	state := cb.state
	reason := cb.reason

	// If cooldown expired, report as active.
	if state == CircuitCooldown && !cb.now().Before(cb.cooldownEndsAt) {
		state = CircuitActive
		reason = ""
	}

	var dailyLossPct float64
	if cb.config.StartingBalance > 0 && cb.dailyPnL < 0 {
		dailyLossPct = -cb.dailyPnL / cb.config.StartingBalance
	}

	return CircuitStatus{
		State:             state,
		Reason:            reason,
		DailyPnL:          cb.dailyPnL,
		DailyLossPct:      dailyLossPct,
		ConsecutiveLosses: cb.consecutiveLosses,
		TradeCount:        cb.tradeCount,
		HaltedAt:          cb.haltedAt,
		CooldownEndsAt:    cb.cooldownEndsAt,
	}
}

// Reset manually resets the circuit breaker to the active state.
// This clears all counters including panic state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = CircuitActive
	cb.reason = ""
	cb.dailyPnL = 0
	cb.consecutiveLosses = 0
	cb.tradeCount = 0
	cb.haltedAt = time.Time{}
	cb.cooldownEndsAt = time.Time{}
}

// Panic triggers an emergency halt that can only be cleared by Reset().
func (cb *CircuitBreaker) Panic() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = CircuitHaltedPanic
	cb.reason = "emergency panic halt"
	cb.haltedAt = cb.now()
	cb.cooldownEndsAt = time.Time{} // no auto-recovery
}

// UpdateConfig replaces the circuit breaker configuration.
// This does not reset counters or state.
func (cb *CircuitBreaker) UpdateConfig(cfg CircuitConfig) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.config = cfg
}
