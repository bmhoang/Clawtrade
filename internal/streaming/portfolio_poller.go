// internal/streaming/portfolio_poller.go
package streaming

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/clawtrade/clawtrade/internal/adapter"
	"github.com/clawtrade/clawtrade/internal/engine"
)

// PortfolioPollerConfig holds configuration for the PortfolioPoller.
type PortfolioPollerConfig struct {
	Adapters     map[string]adapter.TradingAdapter
	Bus          *engine.EventBus
	PollInterval time.Duration
}

// PortfolioPoller polls connected adapters for portfolio state (balances and
// positions) and publishes portfolio.update events when data changes.
// It uses hash-based change detection to avoid publishing unchanged data.
type PortfolioPoller struct {
	config   PortfolioPollerConfig
	lastHash string
}

// NewPortfolioPoller creates a new PortfolioPoller with the given configuration.
func NewPortfolioPoller(cfg PortfolioPollerConfig) *PortfolioPoller {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 30 * time.Second
	}
	return &PortfolioPoller{
		config: cfg,
	}
}

// Start begins polling for portfolio updates. It blocks until the context is cancelled.
func (pp *PortfolioPoller) Start(ctx context.Context) {
	ticker := time.NewTicker(pp.config.PollInterval)
	defer ticker.Stop()

	// Poll immediately on start.
	pp.poll(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pp.poll(ctx)
		}
	}
}

// poll fetches balances and positions from ALL connected adapters,
// aggregates results, and publishes an event if the state has changed.
func (pp *PortfolioPoller) poll(ctx context.Context) {
	var allBalances []adapter.Balance
	var allPositions []adapter.Position
	exchanges := map[string]any{}

	for name, adp := range pp.config.Adapters {
		if !adp.IsConnected() {
			continue
		}

		balances, err := adp.GetBalances(ctx)
		if err != nil {
			continue
		}

		positions, err := adp.GetPositions(ctx)
		if err != nil {
			continue
		}

		allBalances = append(allBalances, balances...)
		allPositions = append(allPositions, positions...)

		var exchTotal float64
		for _, b := range balances {
			exchTotal += b.Total
		}
		exchanges[name] = map[string]any{
			"total":     exchTotal,
			"balances":  balances,
			"positions": positions,
		}
	}

	if len(exchanges) == 0 {
		return
	}

	hash := pp.hashState(allBalances, allPositions)
	if hash == pp.lastHash {
		return
	}
	pp.lastHash = hash

	var totalPnL float64
	for _, pos := range allPositions {
		totalPnL += pos.PnL
	}

	pp.config.Bus.Publish(engine.Event{
		Type: "portfolio.update",
		Data: map[string]any{
			"balances":  allBalances,
			"positions": allPositions,
			"total_pnl": totalPnL,
			"exchanges": exchanges,
		},
	})
}

// hashState produces a SHA-256 hex digest of the JSON-serialized balances and
// positions, used for change detection.
func (pp *PortfolioPoller) hashState(balances []adapter.Balance, positions []adapter.Position) string {
	state := struct {
		Balances  []adapter.Balance  `json:"balances"`
		Positions []adapter.Position `json:"positions"`
	}{
		Balances:  balances,
		Positions: positions,
	}
	data, _ := json.Marshal(state)
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}
