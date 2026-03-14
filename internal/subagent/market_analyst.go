package subagent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/clawtrade/clawtrade/internal/adapter"
)

// MarketAnalystConfig holds the configuration for the Market Analyst sub-agent.
type MarketAnalystConfig struct {
	Strategies       []Strategy
	ActiveStrategies []string
	Weights          map[string]float64
	ScanInterval     time.Duration
	Timeframes       []string
	ExpertCaller     *LLMCaller
	SynthesisCaller  *LLMCaller
	MinConfluence    int
	Adapters         map[string]adapter.TradingAdapter
	Bus              *EventBus
	Watchlist        []string
}

// MarketSnapshot holds a point-in-time market data collection for a single symbol.
type MarketSnapshot struct {
	Symbol     string
	Candles    map[string][]adapter.Candle // timeframe -> candles
	OrderBook  *adapter.OrderBook
	Price      *adapter.Price
	Correlated map[string]*adapter.Price // other symbols' prices
}

// ExpertResult holds the output from one strategy expert LLM call.
type ExpertResult struct {
	Strategy string `json:"strategy"`
	Response string `json:"response"`
}

// MarketAnalyst is a sub-agent that scans the market for trading opportunities
// using multiple strategy experts and a synthesis LLM call.
type MarketAnalyst struct {
	cfg        MarketAnalystConfig
	running    bool
	lastRun    time.Time
	runCount   int
	errorCount int
	lastError  string
	cancel     context.CancelFunc
	mu         sync.RWMutex
}

// NewMarketAnalyst creates a new MarketAnalyst with the given configuration.
func NewMarketAnalyst(cfg MarketAnalystConfig) *MarketAnalyst {
	if cfg.ScanInterval == 0 {
		cfg.ScanInterval = 5 * time.Minute
	}
	if len(cfg.Timeframes) == 0 {
		cfg.Timeframes = []string{"1h", "4h"}
	}
	return &MarketAnalyst{
		cfg: cfg,
	}
}

// Name returns the sub-agent name.
func (ma *MarketAnalyst) Name() string {
	return "market-analyst"
}

// Start begins the market analyst scan loop. It blocks until the context
// is canceled or Stop is called.
func (ma *MarketAnalyst) Start(ctx context.Context) error {
	childCtx, cancel := context.WithCancel(ctx)

	ma.mu.Lock()
	ma.cancel = cancel
	ma.running = true
	ma.mu.Unlock()

	defer func() {
		ma.mu.Lock()
		ma.running = false
		ma.mu.Unlock()
	}()

	ticker := time.NewTicker(ma.cfg.ScanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-childCtx.Done():
			return nil
		case <-ticker.C:
			ma.runScan(childCtx)
		}
	}
}

// Stop cancels the scan loop and marks the agent as not running.
func (ma *MarketAnalyst) Stop() error {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	if ma.cancel != nil {
		ma.cancel()
	}
	ma.running = false
	return nil
}

// Status returns the current status of the market analyst.
func (ma *MarketAnalyst) Status() SubAgentStatus {
	ma.mu.RLock()
	defer ma.mu.RUnlock()
	return SubAgentStatus{
		Name:       "market-analyst",
		Running:    ma.running,
		LastRun:    ma.lastRun,
		RunCount:   ma.runCount,
		ErrorCount: ma.errorCount,
		LastError:  ma.lastError,
	}
}

// runScan iterates over the watchlist and performs analysis for each symbol.
func (ma *MarketAnalyst) runScan(ctx context.Context) {
	ma.mu.Lock()
	ma.runCount++
	ma.lastRun = time.Now()
	ma.mu.Unlock()

	for _, symbol := range ma.cfg.Watchlist {
		select {
		case <-ctx.Done():
			return
		default:
		}

		snap, err := ma.collectSnapshot(ctx, symbol)
		if err != nil {
			ma.mu.Lock()
			ma.errorCount++
			ma.lastError = fmt.Sprintf("collect %s: %v", symbol, err)
			ma.mu.Unlock()
			continue
		}

		formatted := ma.formatForLLM(snap)

		expertResults := ma.runExperts(ctx, formatted)

		synthesis, err := ma.synthesize(ctx, expertResults, symbol)
		if err != nil {
			ma.mu.Lock()
			ma.errorCount++
			ma.lastError = fmt.Sprintf("synthesize %s: %v", symbol, err)
			ma.mu.Unlock()
			continue
		}

		if ma.cfg.Bus != nil {
			ma.cfg.Bus.Publish(Event{
				Type:   "analysis",
				Source: "market-analyst",
				Symbol: symbol,
				Data: map[string]any{
					"synthesis":      synthesis,
					"expert_count":   len(expertResults),
					"expert_results": expertResults,
				},
				Time: time.Now(),
			})
		}
	}
}

// collectSnapshot fetches market data for a symbol from the first connected adapter.
func (ma *MarketAnalyst) collectSnapshot(ctx context.Context, symbol string) (*MarketSnapshot, error) {
	adp := ma.findConnectedAdapter()
	if adp == nil {
		return nil, fmt.Errorf("no connected adapter available")
	}

	snap := &MarketSnapshot{
		Symbol:     symbol,
		Candles:    make(map[string][]adapter.Candle),
		Correlated: make(map[string]*adapter.Price),
	}

	// Get current price
	price, err := adp.GetPrice(ctx, symbol)
	if err != nil {
		return nil, fmt.Errorf("get price %s: %w", symbol, err)
	}
	snap.Price = price

	// Get candles for each configured timeframe
	for _, tf := range ma.cfg.Timeframes {
		candles, err := adp.GetCandles(ctx, symbol, tf, 50)
		if err != nil {
			return nil, fmt.Errorf("get candles %s %s: %w", symbol, tf, err)
		}
		snap.Candles[tf] = candles
	}

	// Get orderbook
	ob, err := adp.GetOrderBook(ctx, symbol, 20)
	if err != nil {
		return nil, fmt.Errorf("get orderbook %s: %w", symbol, err)
	}
	snap.OrderBook = ob

	// Get correlated asset prices
	correlatedSymbols := []string{"BTC/USDT", "ETH/USDT"}
	for _, cs := range correlatedSymbols {
		if cs == symbol {
			continue
		}
		cp, err := adp.GetPrice(ctx, cs)
		if err == nil {
			snap.Correlated[cs] = cp
		}
	}

	return snap, nil
}

// findConnectedAdapter returns the first connected adapter from the config.
func (ma *MarketAnalyst) findConnectedAdapter() adapter.TradingAdapter {
	for _, adp := range ma.cfg.Adapters {
		if adp.IsConnected() {
			return adp
		}
	}
	return nil
}

// formatForLLM converts a MarketSnapshot into a structured text representation
// suitable for sending to an LLM.
func (ma *MarketAnalyst) formatForLLM(snap *MarketSnapshot) string {
	var b strings.Builder

	fmt.Fprintf(&b, "## %s Raw Market Data\n\n", snap.Symbol)

	// Current Price
	if snap.Price != nil {
		fmt.Fprintf(&b, "### Current Price\n")
		fmt.Fprintf(&b, "Last: $%.2f | Bid: $%.2f | Ask: $%.2f | Volume 24h: %.2f\n\n",
			snap.Price.Last, snap.Price.Bid, snap.Price.Ask, snap.Price.Volume24h)
	}

	// Candles by timeframe (sorted for deterministic output)
	timeframes := make([]string, 0, len(snap.Candles))
	for tf := range snap.Candles {
		timeframes = append(timeframes, tf)
	}
	sort.Strings(timeframes)

	for _, tf := range timeframes {
		candles := snap.Candles[tf]
		fmt.Fprintf(&b, "### %s Candles (last %d)\n", tf, len(candles))
		fmt.Fprintf(&b, "Time | Open | High | Low | Close | Volume\n")
		for _, c := range candles {
			fmt.Fprintf(&b, "%s | %.2f | %.2f | %.2f | %.2f | %.2f\n",
				c.Timestamp.Format("2006-01-02 15:04"), c.Open, c.High, c.Low, c.Close, c.Volume)
		}
		b.WriteString("\n")
	}

	// Orderbook
	if snap.OrderBook != nil {
		fmt.Fprintf(&b, "### Orderbook (top %d)\n", maxInt(len(snap.OrderBook.Bids), len(snap.OrderBook.Asks)))

		b.WriteString("Bids: ")
		for i, bid := range snap.OrderBook.Bids {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%.2f (%.1f)", bid.Price, bid.Amount)
			if i >= 19 {
				break
			}
		}
		b.WriteString("\n")

		b.WriteString("Asks: ")
		for i, ask := range snap.OrderBook.Asks {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%.2f (%.1f)", ask.Price, ask.Amount)
			if i >= 19 {
				break
			}
		}
		b.WriteString("\n\n")
	}

	// Correlated assets
	if len(snap.Correlated) > 0 {
		b.WriteString("### Correlated Assets\n")
		symbols := make([]string, 0, len(snap.Correlated))
		for s := range snap.Correlated {
			symbols = append(symbols, s)
		}
		sort.Strings(symbols)
		parts := make([]string, 0, len(symbols))
		for _, s := range symbols {
			p := snap.Correlated[s]
			parts = append(parts, fmt.Sprintf("%s: $%.2f", s, p.Last))
		}
		b.WriteString(strings.Join(parts, " | "))
		b.WriteString("\n")
	}

	return b.String()
}

// runExperts calls each active strategy expert concurrently via the LLM.
func (ma *MarketAnalyst) runExperts(ctx context.Context, formattedData string) []ExpertResult {
	if ma.cfg.ExpertCaller == nil {
		return nil
	}

	activeSet := make(map[string]bool, len(ma.cfg.ActiveStrategies))
	for _, slug := range ma.cfg.ActiveStrategies {
		activeSet[slug] = true
	}

	var activeStrategies []Strategy
	for _, s := range ma.cfg.Strategies {
		if activeSet[s.Slug] {
			activeStrategies = append(activeStrategies, s)
		}
	}

	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		results []ExpertResult
	)

	for _, strat := range activeStrategies {
		wg.Add(1)
		go func(s Strategy) {
			defer wg.Done()

			resp, err := ma.cfg.ExpertCaller.Call(ctx, s.Prompt, formattedData)
			if err != nil {
				mu.Lock()
				ma.errorCount++
				ma.lastError = fmt.Sprintf("expert %s: %v", s.Slug, err)
				mu.Unlock()
				return
			}

			mu.Lock()
			results = append(results, ExpertResult{
				Strategy: s.Name,
				Response: resp,
			})
			mu.Unlock()
		}(strat)
	}

	wg.Wait()
	return results
}

// synthesize makes a final LLM call combining all expert outputs into a unified analysis.
func (ma *MarketAnalyst) synthesize(ctx context.Context, expertResults []ExpertResult, symbol string) (string, error) {
	if ma.cfg.SynthesisCaller == nil {
		return "", fmt.Errorf("no synthesis caller configured")
	}

	if len(expertResults) == 0 {
		return "", fmt.Errorf("no expert results to synthesize")
	}

	var userMsg strings.Builder
	for _, er := range expertResults {
		fmt.Fprintf(&userMsg, "## %s Analysis\n%s\n\n", er.Strategy, er.Response)
	}

	systemPrompt := fmt.Sprintf(`You are a Trading Synthesis Agent. Below are analyses from multiple expert perspectives on %s.

Your task:
1. Find where experts AGREE (confluence zones)
2. Find where experts DISAGREE (conflicts)
3. Rate overall conviction (0-100%%)
4. Generate a unified trade thesis
5. Recommend: "strong_buy", "buy", "neutral", "sell", "strong_sell"

Output as JSON: {"bias":"...", "confidence":N, "thesis":"...", "confluence":["..."], "conflicts":["..."], "recommendation":"..."}`, symbol)

	return ma.cfg.SynthesisCaller.Call(ctx, systemPrompt, userMsg.String())
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
