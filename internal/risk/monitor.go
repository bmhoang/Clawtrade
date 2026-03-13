// internal/risk/monitor.go
package risk

import (
	"fmt"
	"sync"
	"time"
)

// AlertSeverity indicates the urgency of an alert.
type AlertSeverity int

const (
	SeverityInfo AlertSeverity = iota
	SeverityWarning
	SeverityCritical
)

func (s AlertSeverity) String() string {
	switch s {
	case SeverityInfo:
		return "INFO"
	case SeverityWarning:
		return "WARNING"
	case SeverityCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// AlertType classifies the kind of alert.
type AlertType string

const (
	AlertDrawdown     AlertType = "DRAWDOWN"
	AlertTrailingStop AlertType = "TRAILING_STOP"
	AlertPnLThreshold AlertType = "PNL_THRESHOLD"
)

// Alert represents a triggered risk alert.
type Alert struct {
	Type      AlertType
	Symbol    string
	Message   string
	Severity  AlertSeverity
	Timestamp time.Time
}

// TrackedPosition holds live position state for monitoring.
type TrackedPosition struct {
	Symbol     string
	Side       string  // "long" or "short"
	Size       float64
	EntryPrice float64
	CurrentPrice float64
	PeakPnL    float64
	Drawdown   float64 // drawdown from peak as a positive fraction (0.0 - 1.0)
	UnrealizedPnL float64
}

// computePnL returns the unrealized PnL for the position.
func (tp *TrackedPosition) computePnL() float64 {
	if tp.Side == "long" {
		return (tp.CurrentPrice - tp.EntryPrice) * tp.Size
	}
	return (tp.EntryPrice - tp.CurrentPrice) * tp.Size
}

// MonitorConfig holds configurable thresholds for the monitor.
type MonitorConfig struct {
	DrawdownWarning  float64 // e.g., 0.05 = 5% drawdown from peak triggers warning
	DrawdownCritical float64 // e.g., 0.10 = 10% drawdown triggers critical alert
	TrailingStopPct  float64 // e.g., 0.03 = 3% trailing stop distance
}

// DefaultMonitorConfig returns sensible defaults.
func DefaultMonitorConfig() MonitorConfig {
	return MonitorConfig{
		DrawdownWarning:  0.05,
		DrawdownCritical: 0.10,
		TrailingStopPct:  0.03,
	}
}

// Monitor tracks open positions with real-time PnL, drawdown, and alerts.
type Monitor struct {
	mu        sync.RWMutex
	positions map[string]*TrackedPosition
	alerts    []Alert
	config    MonitorConfig
}

// NewMonitor creates a new position monitor with the given config.
func NewMonitor(cfg MonitorConfig) *Monitor {
	return &Monitor{
		positions: make(map[string]*TrackedPosition),
		config:    cfg,
	}
}

// AddPosition adds a new position to be tracked.
func (m *Monitor) AddPosition(symbol, side string, size, entryPrice float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	tp := &TrackedPosition{
		Symbol:       symbol,
		Side:         side,
		Size:         size,
		EntryPrice:   entryPrice,
		CurrentPrice: entryPrice,
		PeakPnL:      0,
		Drawdown:     0,
		UnrealizedPnL: 0,
	}
	m.positions[symbol] = tp
}

// RemovePosition removes a tracked position by symbol.
func (m *Monitor) RemovePosition(symbol string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.positions, symbol)
}

// UpdatePrice updates the current price for a symbol, recalculates PnL and
// drawdown, and checks alert thresholds.
func (m *Monitor) UpdatePrice(symbol string, price float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	tp, ok := m.positions[symbol]
	if !ok {
		return
	}

	tp.CurrentPrice = price
	pnl := tp.computePnL()
	tp.UnrealizedPnL = pnl

	// Update peak PnL
	if pnl > tp.PeakPnL {
		tp.PeakPnL = pnl
	}

	// Calculate drawdown from peak (only meaningful when peak > 0)
	if tp.PeakPnL > 0 {
		tp.Drawdown = (tp.PeakPnL - pnl) / tp.PeakPnL
	} else {
		tp.Drawdown = 0
	}

	// Check drawdown alerts
	if tp.Drawdown >= m.config.DrawdownCritical {
		m.alerts = append(m.alerts, Alert{
			Type:      AlertDrawdown,
			Symbol:    symbol,
			Message:   fmt.Sprintf("CRITICAL drawdown %.2f%% from peak on %s (PnL: %.4f, Peak: %.4f)", tp.Drawdown*100, symbol, pnl, tp.PeakPnL),
			Severity:  SeverityCritical,
			Timestamp: time.Now(),
		})
	} else if tp.Drawdown >= m.config.DrawdownWarning {
		m.alerts = append(m.alerts, Alert{
			Type:      AlertDrawdown,
			Symbol:    symbol,
			Message:   fmt.Sprintf("WARNING drawdown %.2f%% from peak on %s (PnL: %.4f, Peak: %.4f)", tp.Drawdown*100, symbol, pnl, tp.PeakPnL),
			Severity:  SeverityWarning,
			Timestamp: time.Now(),
		})
	}

	// Check trailing stop
	if m.config.TrailingStopPct > 0 && tp.PeakPnL > 0 {
		trailingThreshold := tp.PeakPnL * (1 - m.config.TrailingStopPct)
		if pnl <= trailingThreshold {
			m.alerts = append(m.alerts, Alert{
				Type:      AlertTrailingStop,
				Symbol:    symbol,
				Message:   fmt.Sprintf("Trailing stop triggered on %s: PnL %.4f fell below threshold %.4f (peak: %.4f)", symbol, pnl, trailingThreshold, tp.PeakPnL),
				Severity:  SeverityWarning,
				Timestamp: time.Now(),
			})
		}
	}
}

// GetAlerts returns all triggered alerts and clears the internal list.
func (m *Monitor) GetAlerts() []Alert {
	m.mu.Lock()
	defer m.mu.Unlock()

	alerts := make([]Alert, len(m.alerts))
	copy(alerts, m.alerts)
	m.alerts = m.alerts[:0]
	return alerts
}

// GetPosition returns a snapshot of a tracked position.
func (m *Monitor) GetPosition(symbol string) (TrackedPosition, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tp, ok := m.positions[symbol]
	if !ok {
		return TrackedPosition{}, false
	}
	return *tp, true
}

// Positions returns a snapshot of all tracked positions.
func (m *Monitor) Positions() []TrackedPosition {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]TrackedPosition, 0, len(m.positions))
	for _, tp := range m.positions {
		result = append(result, *tp)
	}
	return result
}

// UpdateTrailingStop updates the trailing stop percentage for the monitor.
func (m *Monitor) UpdateTrailingStop(pct float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config.TrailingStopPct = pct
}
