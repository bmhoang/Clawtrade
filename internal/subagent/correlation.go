package subagent

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/clawtrade/clawtrade/internal/adapter"
)

// CorrelationConfig holds the configuration for the Correlation Agent sub-agent.
type CorrelationConfig struct {
	LLM          *LLMCaller
	Bus          *EventBus
	Adapters     map[string]adapter.TradingAdapter
	Watchlist    []string
	ScanInterval time.Duration
}

// CorrelationAgent is a sub-agent that tracks cross-asset correlations and
// publishes regime analysis based on Pearson correlation coefficients.
type CorrelationAgent struct {
	cfg          CorrelationConfig
	running      bool
	lastRun      time.Time
	runCount     int
	errorCount   int
	lastError    string
	cancel       context.CancelFunc
	mu           sync.RWMutex
	priceHistory map[string][]float64
}

// NewCorrelationAgent creates a new CorrelationAgent with the given configuration.
func NewCorrelationAgent(cfg CorrelationConfig) *CorrelationAgent {
	if cfg.ScanInterval == 0 {
		cfg.ScanInterval = 10 * time.Minute
	}
	return &CorrelationAgent{
		cfg:          cfg,
		priceHistory: make(map[string][]float64),
	}
}

// Name returns the sub-agent name.
func (ca *CorrelationAgent) Name() string {
	return "correlation"
}

// Start begins the correlation scan loop. It blocks until the context is
// canceled or Stop is called.
func (ca *CorrelationAgent) Start(ctx context.Context) error {
	childCtx, cancel := context.WithCancel(ctx)

	ca.mu.Lock()
	ca.cancel = cancel
	ca.running = true
	ca.mu.Unlock()

	defer func() {
		ca.mu.Lock()
		ca.running = false
		ca.mu.Unlock()
	}()

	ticker := time.NewTicker(ca.cfg.ScanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-childCtx.Done():
			return nil
		case <-ticker.C:
			ca.runScan(childCtx)
		}
	}
}

// Stop cancels the scan loop and marks the agent as not running.
func (ca *CorrelationAgent) Stop() error {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	if ca.cancel != nil {
		ca.cancel()
	}
	ca.running = false
	return nil
}

// Status returns the current status of the correlation agent.
func (ca *CorrelationAgent) Status() SubAgentStatus {
	ca.mu.RLock()
	defer ca.mu.RUnlock()
	return SubAgentStatus{
		Name:       "correlation",
		Running:    ca.running,
		LastRun:    ca.lastRun,
		RunCount:   ca.runCount,
		ErrorCount: ca.errorCount,
		LastError:  ca.lastError,
	}
}

// runScan fetches current prices, updates price history, calculates pairwise
// correlations, optionally calls the LLM for regime analysis, and publishes
// a correlation event.
func (ca *CorrelationAgent) runScan(ctx context.Context) {
	ca.mu.Lock()
	ca.runCount++
	ca.lastRun = time.Now()
	ca.mu.Unlock()

	// Step 1: Fetch current prices for all watchlist symbols.
	prices := make(map[string]float64)
	adp := ca.findConnectedAdapter()
	if adp == nil {
		ca.mu.Lock()
		ca.errorCount++
		ca.lastError = "no connected adapter available"
		ca.mu.Unlock()
		return
	}

	for _, symbol := range ca.cfg.Watchlist {
		select {
		case <-ctx.Done():
			return
		default:
		}

		price, err := adp.GetPrice(ctx, symbol)
		if err != nil {
			ca.mu.Lock()
			ca.errorCount++
			ca.lastError = fmt.Sprintf("get price %s: %v", symbol, err)
			ca.mu.Unlock()
			continue
		}
		prices[symbol] = price.Last
	}

	// Step 2: Append to priceHistory (keep last 30 points).
	ca.mu.Lock()
	for symbol, p := range prices {
		ca.priceHistory[symbol] = append(ca.priceHistory[symbol], p)
		if len(ca.priceHistory[symbol]) > 30 {
			ca.priceHistory[symbol] = ca.priceHistory[symbol][len(ca.priceHistory[symbol])-30:]
		}
	}
	ca.mu.Unlock()

	// Step 3: If enough data points (>= 5), calculate pair-wise correlations.
	ca.mu.RLock()
	hasEnoughData := true
	for _, symbol := range ca.cfg.Watchlist {
		if len(ca.priceHistory[symbol]) < 5 {
			hasEnoughData = false
			break
		}
	}
	ca.mu.RUnlock()

	if !hasEnoughData {
		return
	}

	correlations := make(map[string]float64)
	ca.mu.RLock()
	for i := 0; i < len(ca.cfg.Watchlist); i++ {
		for j := i + 1; j < len(ca.cfg.Watchlist); j++ {
			symA := ca.cfg.Watchlist[i]
			symB := ca.cfg.Watchlist[j]
			histA := ca.priceHistory[symA]
			histB := ca.priceHistory[symB]

			// Use the minimum length of both histories.
			minLen := len(histA)
			if len(histB) < minLen {
				minLen = len(histB)
			}

			corr := calcCorrelation(histA[len(histA)-minLen:], histB[len(histB)-minLen:])
			key := fmt.Sprintf("%s vs %s", symA, symB)
			correlations[key] = corr
		}
	}
	ca.mu.RUnlock()

	// Step 4: Format correlation data.
	formatted := ca.formatCorrelationData(correlations, prices)

	// Step 5: If LLM configured, call LLM for regime analysis.
	var llmAnalysis string
	if ca.cfg.LLM != nil {
		systemPrompt := fmt.Sprintf(`You are a Cross-Market Correlation Analyst.

%s

## Your Analysis
1. Which correlations have broken or strengthened recently?
2. What does correlation breakdown mean for trading?
3. Capital flow direction: where is money going?
4. Regime assessment: trending_up / trending_down / ranging / volatile / transitioning
5. Any anomalies?

## Output as JSON
{"regime":"...", "correlation_breaks":[{"pair":"...","change":"...","significance":"..."}], "capital_flow":"...", "anomalies":["..."], "implications":["..."]}`, formatted)

		resp, err := ca.cfg.LLM.Call(ctx, systemPrompt, formatted)
		if err != nil {
			ca.mu.Lock()
			ca.errorCount++
			ca.lastError = fmt.Sprintf("llm call: %v", err)
			ca.mu.Unlock()
		} else {
			llmAnalysis = resp
		}
	}

	// Step 6: Publish Event{Type: "correlation"} with data.
	if ca.cfg.Bus != nil {
		data := map[string]any{
			"correlations": correlations,
			"prices":       prices,
			"formatted":    formatted,
		}
		if llmAnalysis != "" {
			data["llm_analysis"] = llmAnalysis
		}
		ca.cfg.Bus.Publish(Event{
			Type:   "correlation",
			Source: "correlation",
			Data:   data,
			Time:   time.Now(),
		})
	}
}

// findConnectedAdapter returns the first connected adapter from the config.
func (ca *CorrelationAgent) findConnectedAdapter() adapter.TradingAdapter {
	for _, adp := range ca.cfg.Adapters {
		if adp.IsConnected() {
			return adp
		}
	}
	return nil
}

// calcCorrelation computes the Pearson correlation coefficient between two
// float64 slices. Returns 0 if the slices have different lengths, fewer than
// 2 elements, or if either series has zero variance.
func calcCorrelation(a, b []float64) float64 {
	if len(a) != len(b) || len(a) < 2 {
		return 0
	}

	n := float64(len(a))

	// Calculate means.
	var sumA, sumB float64
	for i := range a {
		sumA += a[i]
		sumB += b[i]
	}
	meanA := sumA / n
	meanB := sumB / n

	// Calculate Pearson r.
	var num, denA, denB float64
	for i := range a {
		dA := a[i] - meanA
		dB := b[i] - meanB
		num += dA * dB
		denA += dA * dA
		denB += dB * dB
	}

	if denA == 0 || denB == 0 {
		return 0
	}

	return num / math.Sqrt(denA*denB)
}

// formatCorrelationData formats correlations and prices into a human-readable
// markdown string.
func (ca *CorrelationAgent) formatCorrelationData(correlations map[string]float64, prices map[string]float64) string {
	var b strings.Builder

	b.WriteString("## Cross-Asset Correlations\n")

	// Sort keys for deterministic output.
	corrKeys := make([]string, 0, len(correlations))
	for k := range correlations {
		corrKeys = append(corrKeys, k)
	}
	sort.Strings(corrKeys)

	for _, k := range corrKeys {
		fmt.Fprintf(&b, "- %s: %.2f\n", k, correlations[k])
	}

	b.WriteString("\n## Current Prices\n")

	priceKeys := make([]string, 0, len(prices))
	for k := range prices {
		priceKeys = append(priceKeys, k)
	}
	sort.Strings(priceKeys)

	for _, k := range priceKeys {
		fmt.Fprintf(&b, "- %s: $%s\n", k, formatPrice(prices[k]))
	}

	return b.String()
}

