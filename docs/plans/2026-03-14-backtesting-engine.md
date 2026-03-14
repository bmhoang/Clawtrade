# Backtesting Engine Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a backtesting engine that replays historical candles through strategies, simulates portfolio state, and outputs performance metrics — accessible from CLI, AI agent tool, API endpoint, and WebSocket streaming.

**Architecture:** Monolithic `internal/backtest/` package with 4 files: data loader (SQLite cache + exchange API pagination), engine (time-loop orchestrator), strategy executor (code/config/AI modes), and portfolio simulator. Integrates with existing adapters, indicators, strategy arena, event bus, and API server.

**Tech Stack:** Go, SQLite (via existing `database.Open`), existing `adapter.TradingAdapter.GetCandles()`, `analysis` indicators, `engine.EventBus`, `chi` router.

---

### Task 1: Portfolio Simulator

The portfolio simulator tracks cash, positions, and trades during a backtest. It applies fees and slippage on each trade execution, checks stop-loss/take-profit on each candle tick, and records equity curve points.

**Files:**
- Create: `internal/backtest/portfolio.go`
- Create: `internal/backtest/portfolio_test.go`

**Context:**
- Reuse `adapter.Side` constants (`SideBuy`, `SideSell`) from `internal/adapter/types.go`
- This is a self-contained component with no external dependencies beyond the adapter types
- Must support: open long/short, close position, apply fees/slippage, tick (check SL/TP), equity snapshot

**Step 1: Write the test file**

```go
// internal/backtest/portfolio_test.go
package backtest

import (
	"testing"
	"time"
)

func TestPortfolio_OpenAndClose(t *testing.T) {
	p := NewPortfolio(10000, 0.001, 0.001, 0.0005) // 10k capital, 0.1% maker/taker, 0.05% slippage

	// Open a long position
	err := p.OpenPosition("BTC/USDT", SideLong, 0.1, 50000, time.Now())
	if err != nil {
		t.Fatalf("open position: %v", err)
	}

	if len(p.Positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(p.Positions))
	}

	// Cash should decrease by cost + fees + slippage
	// cost = 0.1 * 50000 = 5000
	// slippage = 5000 * 0.0005 = 2.5
	// fee = (5000 + 2.5) * 0.001 = 5.0025
	// total deducted = 5000 + 2.5 + 5.0025 = 5007.5025
	expectedCash := 10000 - 5007.5025
	if abs(p.Cash-expectedCash) > 0.01 {
		t.Fatalf("expected cash ~%.2f, got %.2f", expectedCash, p.Cash)
	}

	// Close position at higher price
	pnl := p.ClosePosition(0, 55000, time.Now())

	// Gross = 0.1 * 55000 = 5500
	// slippage = 5500 * 0.0005 = 2.75
	// fee = (5500 - 2.75) * 0.001 = 5.49725
	// net received = 5500 - 2.75 - 5.49725 = 5491.75275
	// PnL = net received - cost paid (5000 + 2.5 + 5.0025) = 5491.75275 - 5007.5025 = 484.25025
	if pnl <= 0 {
		t.Fatalf("expected positive pnl, got %.4f", pnl)
	}
	if len(p.Positions) != 0 {
		t.Fatalf("expected 0 positions after close, got %d", len(p.Positions))
	}
	if len(p.Trades) != 1 {
		t.Fatalf("expected 1 trade record, got %d", len(p.Trades))
	}
}

func TestPortfolio_StopLoss(t *testing.T) {
	p := NewPortfolio(10000, 0.001, 0.001, 0)

	err := p.OpenPosition("BTC/USDT", SideLong, 0.1, 50000, time.Now())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	p.Positions[0].StopLoss = 48000

	// Tick with price below stop loss
	closed := p.Tick(47500, time.Now())
	if len(closed) != 1 {
		t.Fatalf("expected 1 closed position from SL, got %d", len(closed))
	}
	if closed[0].PnL >= 0 {
		t.Fatalf("expected negative pnl from SL hit, got %.4f", closed[0].PnL)
	}
}

func TestPortfolio_TakeProfit(t *testing.T) {
	p := NewPortfolio(10000, 0.001, 0.001, 0)

	err := p.OpenPosition("BTC/USDT", SideLong, 0.1, 50000, time.Now())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	p.Positions[0].TakeProfit = 55000

	closed := p.Tick(56000, time.Now())
	if len(closed) != 1 {
		t.Fatalf("expected 1 closed position from TP, got %d", len(closed))
	}
	if closed[0].PnL <= 0 {
		t.Fatalf("expected positive pnl from TP hit, got %.4f", closed[0].PnL)
	}
}

func TestPortfolio_Equity(t *testing.T) {
	p := NewPortfolio(10000, 0, 0, 0) // no fees for simplicity

	if p.Equity(50000) != 10000 {
		t.Fatalf("initial equity should be 10000")
	}

	p.OpenPosition("BTC/USDT", SideLong, 0.1, 50000, time.Now())
	// Cash = 5000, position value at 55000 = 0.1 * 55000 = 5500
	// Equity = 5000 + 5500 = 10500
	eq := p.Equity(55000)
	if abs(eq-10500) > 0.01 {
		t.Fatalf("expected equity 10500, got %.2f", eq)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
```

**Step 2: Run test to verify it fails**

Run: `cd /d/Clawtrade && go test ./internal/backtest/ -v -run TestPortfolio`
Expected: FAIL — package does not exist yet

**Step 3: Write portfolio.go implementation**

```go
// internal/backtest/portfolio.go
package backtest

import (
	"fmt"
	"time"
)

const (
	SideLong  = "long"
	SideShort = "short"
)

// SimPosition represents an open position in the backtest.
type SimPosition struct {
	Symbol     string    `json:"symbol"`
	Side       string    `json:"side"` // "long" or "short"
	Size       float64   `json:"size"`
	EntryPrice float64   `json:"entry_price"`
	EntryCost  float64   `json:"entry_cost"` // total cost including fees/slippage
	StopLoss   float64   `json:"stop_loss,omitempty"`
	TakeProfit float64   `json:"take_profit,omitempty"`
	OpenedAt   time.Time `json:"opened_at"`
}

// TradeRecord represents a completed trade.
type TradeRecord struct {
	Symbol     string    `json:"symbol"`
	Side       string    `json:"side"`
	Size       float64   `json:"size"`
	EntryPrice float64   `json:"entry_price"`
	ExitPrice  float64   `json:"exit_price"`
	PnL        float64   `json:"pnl"`
	OpenedAt   time.Time `json:"opened_at"`
	ClosedAt   time.Time `json:"closed_at"`
}

// Portfolio simulates an account during backtesting.
type Portfolio struct {
	InitialCash float64        `json:"initial_cash"`
	Cash        float64        `json:"cash"`
	MakerFee    float64        `json:"maker_fee"`  // e.g. 0.001 = 0.1%
	TakerFee    float64        `json:"taker_fee"`
	Slippage    float64        `json:"slippage"`   // e.g. 0.0005 = 0.05%
	Positions   []SimPosition  `json:"positions"`
	Trades      []TradeRecord  `json:"trades"`
}

// NewPortfolio creates a new portfolio simulator.
func NewPortfolio(capital, makerFee, takerFee, slippage float64) *Portfolio {
	return &Portfolio{
		InitialCash: capital,
		Cash:        capital,
		MakerFee:    makerFee,
		TakerFee:    takerFee,
		Slippage:    slippage,
	}
}

// OpenPosition opens a new position. Deducts cost + slippage + fee from cash.
func (p *Portfolio) OpenPosition(symbol, side string, size, price float64, at time.Time) error {
	notional := size * price
	slip := notional * p.Slippage
	fee := (notional + slip) * p.TakerFee
	totalCost := notional + slip + fee

	if totalCost > p.Cash {
		return fmt.Errorf("insufficient cash: need %.2f, have %.2f", totalCost, p.Cash)
	}

	p.Cash -= totalCost
	p.Positions = append(p.Positions, SimPosition{
		Symbol:     symbol,
		Side:       side,
		Size:       size,
		EntryPrice: price,
		EntryCost:  totalCost,
		OpenedAt:   at,
	})
	return nil
}

// ClosePosition closes position at index, returns PnL.
func (p *Portfolio) ClosePosition(idx int, exitPrice float64, at time.Time) float64 {
	pos := p.Positions[idx]
	notional := pos.Size * exitPrice
	slip := notional * p.Slippage
	fee := (notional - slip) * p.TakerFee
	netReceived := notional - slip - fee

	var pnl float64
	if pos.Side == SideLong {
		pnl = netReceived - pos.EntryCost
	} else {
		// Short: profit when price goes down
		pnl = pos.EntryCost - netReceived
	}

	p.Cash += netReceived
	if pos.Side == SideShort {
		// For shorts, we need to adjust: cash += entry cost + pnl
		p.Cash = p.Cash - netReceived + pos.EntryCost + pnl
	}

	p.Trades = append(p.Trades, TradeRecord{
		Symbol:     pos.Symbol,
		Side:       pos.Side,
		Size:       pos.Size,
		EntryPrice: pos.EntryPrice,
		ExitPrice:  exitPrice,
		PnL:        pnl,
		OpenedAt:   pos.OpenedAt,
		ClosedAt:   at,
	})

	// Remove position
	p.Positions = append(p.Positions[:idx], p.Positions[idx+1:]...)
	return pnl
}

// Tick checks all open positions for stop-loss/take-profit hits. Returns closed trades.
func (p *Portfolio) Tick(currentPrice float64, at time.Time) []TradeRecord {
	var closed []TradeRecord
	for i := len(p.Positions) - 1; i >= 0; i-- {
		pos := p.Positions[i]
		hit := false

		if pos.Side == SideLong {
			if pos.StopLoss > 0 && currentPrice <= pos.StopLoss {
				hit = true
			}
			if pos.TakeProfit > 0 && currentPrice >= pos.TakeProfit {
				hit = true
			}
		} else {
			if pos.StopLoss > 0 && currentPrice >= pos.StopLoss {
				hit = true
			}
			if pos.TakeProfit > 0 && currentPrice <= pos.TakeProfit {
				hit = true
			}
		}

		if hit {
			pnl := p.ClosePosition(i, currentPrice, at)
			closed = append(closed, p.Trades[len(p.Trades)-1])
			_ = pnl
		}
	}
	return closed
}

// Equity returns total portfolio value at current price (cash + positions marked to market).
func (p *Portfolio) Equity(currentPrice float64) float64 {
	eq := p.Cash
	for _, pos := range p.Positions {
		eq += pos.Size * currentPrice
	}
	return eq
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /d/Clawtrade && go test ./internal/backtest/ -v -run TestPortfolio`
Expected: PASS (all 4 tests)

**Step 5: Commit**

```bash
git add internal/backtest/portfolio.go internal/backtest/portfolio_test.go
git commit -m "feat(backtest): add portfolio simulator with fees, slippage, SL/TP"
```

---

### Task 2: Data Loader with SQLite Cache

The data loader fetches historical candles from exchange APIs, caches them in SQLite, and auto-paginates for large date ranges. Cache-first: if candles exist in SQLite for the requested range, use them; otherwise fetch from exchange and cache.

**Files:**
- Create: `internal/backtest/data.go`
- Create: `internal/backtest/data_test.go`
- Modify: `internal/database/migrations.go` — add candle cache table

**Context:**
- `adapter.TradingAdapter.GetCandles(ctx, symbol, timeframe, limit)` returns `[]adapter.Candle` (max ~200 per call)
- `database.Open(path)` returns `*sql.DB` with SQLite, already uses `modernc.org/sqlite`
- Migrations are in `internal/database/migrations.go` — append new `CREATE TABLE IF NOT EXISTS` statement
- Timeframe strings: "1m", "5m", "15m", "1h", "4h", "1d", "1w"
- Need to convert timeframe string to duration for pagination logic

**Step 1: Add candle cache migration**

In `internal/database/migrations.go`, append this to the `migrations` slice (before the closing `}`):

```go
`CREATE TABLE IF NOT EXISTS candle_cache (
    symbol TEXT NOT NULL,
    timeframe TEXT NOT NULL,
    timestamp INTEGER NOT NULL,
    open REAL NOT NULL,
    high REAL NOT NULL,
    low REAL NOT NULL,
    close REAL NOT NULL,
    volume REAL NOT NULL,
    PRIMARY KEY (symbol, timeframe, timestamp)
)`,
```

**Step 2: Write data_test.go**

```go
// internal/backtest/data_test.go
package backtest

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/clawtrade/clawtrade/internal/adapter"
	_ "modernc.org/sqlite"
)

// mockCandleAdapter implements adapter.TradingAdapter for testing
type mockCandleAdapter struct {
	candles []adapter.Candle
}

func (m *mockCandleAdapter) Name() string                    { return "mock" }
func (m *mockCandleAdapter) Capabilities() adapter.AdapterCaps { return adapter.AdapterCaps{} }
func (m *mockCandleAdapter) Connect(ctx context.Context) error { return nil }
func (m *mockCandleAdapter) Disconnect() error                { return nil }
func (m *mockCandleAdapter) IsConnected() bool                { return true }
func (m *mockCandleAdapter) GetPrice(ctx context.Context, symbol string) (*adapter.Price, error) {
	return nil, nil
}
func (m *mockCandleAdapter) GetCandles(ctx context.Context, symbol, timeframe string, limit int) ([]adapter.Candle, error) {
	if limit > len(m.candles) {
		return m.candles, nil
	}
	return m.candles[len(m.candles)-limit:], nil
}
func (m *mockCandleAdapter) GetOrderBook(ctx context.Context, symbol string, depth int) (*adapter.OrderBook, error) {
	return nil, nil
}
func (m *mockCandleAdapter) PlaceOrder(ctx context.Context, order adapter.Order) (*adapter.Order, error) {
	return nil, nil
}
func (m *mockCandleAdapter) CancelOrder(ctx context.Context, orderID string) error { return nil }
func (m *mockCandleAdapter) GetOpenOrders(ctx context.Context) ([]adapter.Order, error) {
	return nil, nil
}
func (m *mockCandleAdapter) GetBalances(ctx context.Context) ([]adapter.Balance, error) {
	return nil, nil
}
func (m *mockCandleAdapter) GetPositions(ctx context.Context) ([]adapter.Position, error) {
	return nil, nil
}

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tmp := t.TempDir()
	db, err := sql.Open("sqlite", tmp+"/test.db")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS candle_cache (
		symbol TEXT NOT NULL,
		timeframe TEXT NOT NULL,
		timestamp INTEGER NOT NULL,
		open REAL NOT NULL,
		high REAL NOT NULL,
		low REAL NOT NULL,
		close REAL NOT NULL,
		volume REAL NOT NULL,
		PRIMARY KEY (symbol, timeframe, timestamp)
	)`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestDataLoader_FetchAndCache(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now().Truncate(time.Hour)
	candles := make([]adapter.Candle, 10)
	for i := range candles {
		candles[i] = adapter.Candle{
			Open: 100, High: 110, Low: 90, Close: 105, Volume: 1000,
			Timestamp: now.Add(time.Duration(i) * time.Hour),
		}
	}

	mock := &mockCandleAdapter{candles: candles}
	loader := NewDataLoader(db, mock)

	// First call: fetches from adapter, caches in DB
	result, err := loader.LoadCandles(context.Background(), "BTC/USDT", "1h", now, now.Add(9*time.Hour))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(result) != 10 {
		t.Fatalf("expected 10 candles, got %d", len(result))
	}

	// Verify cached in DB
	var count int
	db.QueryRow("SELECT COUNT(*) FROM candle_cache WHERE symbol='BTC/USDT'").Scan(&count)
	if count != 10 {
		t.Fatalf("expected 10 cached rows, got %d", count)
	}

	// Second call with nil adapter should use cache
	loader2 := NewDataLoader(db, nil)
	result2, err := loader2.LoadCandles(context.Background(), "BTC/USDT", "1h", now, now.Add(9*time.Hour))
	if err != nil {
		t.Fatalf("load from cache: %v", err)
	}
	if len(result2) != 10 {
		t.Fatalf("expected 10 from cache, got %d", len(result2))
	}
}

func TestTimeframeToDuration(t *testing.T) {
	tests := []struct {
		tf   string
		want time.Duration
	}{
		{"1m", time.Minute},
		{"5m", 5 * time.Minute},
		{"15m", 15 * time.Minute},
		{"1h", time.Hour},
		{"4h", 4 * time.Hour},
		{"1d", 24 * time.Hour},
		{"1w", 7 * 24 * time.Hour},
	}
	for _, tt := range tests {
		got := timeframeToDuration(tt.tf)
		if got != tt.want {
			t.Errorf("timeframeToDuration(%q) = %v, want %v", tt.tf, got, tt.want)
		}
	}
}
```

**Step 3: Write data.go implementation**

```go
// internal/backtest/data.go
package backtest

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/clawtrade/clawtrade/internal/adapter"
)

const maxCandlesPerRequest = 200

// DataLoader fetches and caches historical candle data.
type DataLoader struct {
	db      *sql.DB
	adapter adapter.TradingAdapter
}

// NewDataLoader creates a data loader with SQLite cache and exchange adapter.
func NewDataLoader(db *sql.DB, adp adapter.TradingAdapter) *DataLoader {
	return &DataLoader{db: db, adapter: adp}
}

// LoadCandles returns candles for the given range, using cache when available.
func (d *DataLoader) LoadCandles(ctx context.Context, symbol, timeframe string, from, to time.Time) ([]adapter.Candle, error) {
	// Try cache first
	cached, err := d.loadFromCache(symbol, timeframe, from, to)
	if err == nil && len(cached) > 0 {
		return cached, nil
	}

	// Fetch from exchange
	if d.adapter == nil {
		return cached, nil
	}

	candles, err := d.fetchAll(ctx, symbol, timeframe, from, to)
	if err != nil {
		return nil, fmt.Errorf("fetch candles: %w", err)
	}

	// Cache fetched data
	if d.db != nil {
		d.saveToCache(symbol, timeframe, candles)
	}

	return candles, nil
}

// fetchAll paginates through the exchange API to get all candles in range.
func (d *DataLoader) fetchAll(ctx context.Context, symbol, timeframe string, from, to time.Time) ([]adapter.Candle, error) {
	dur := timeframeToDuration(timeframe)
	totalCandles := int(to.Sub(from)/dur) + 1

	// If fits in one request, just fetch
	if totalCandles <= maxCandlesPerRequest {
		return d.adapter.GetCandles(ctx, symbol, timeframe, totalCandles)
	}

	// Paginate: fetch in chunks of maxCandlesPerRequest
	var all []adapter.Candle
	remaining := totalCandles
	for remaining > 0 {
		limit := remaining
		if limit > maxCandlesPerRequest {
			limit = maxCandlesPerRequest
		}
		batch, err := d.adapter.GetCandles(ctx, symbol, timeframe, limit)
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		remaining -= len(batch)
		if len(batch) < limit {
			break // no more data
		}
	}
	return all, nil
}

func (d *DataLoader) loadFromCache(symbol, timeframe string, from, to time.Time) ([]adapter.Candle, error) {
	if d.db == nil {
		return nil, fmt.Errorf("no database")
	}

	rows, err := d.db.Query(
		`SELECT timestamp, open, high, low, close, volume FROM candle_cache
		 WHERE symbol = ? AND timeframe = ? AND timestamp >= ? AND timestamp <= ?
		 ORDER BY timestamp ASC`,
		symbol, timeframe, from.Unix(), to.Unix(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candles []adapter.Candle
	for rows.Next() {
		var ts int64
		var c adapter.Candle
		if err := rows.Scan(&ts, &c.Open, &c.High, &c.Low, &c.Close, &c.Volume); err != nil {
			return nil, err
		}
		c.Timestamp = time.Unix(ts, 0)
		candles = append(candles, c)
	}
	return candles, rows.Err()
}

func (d *DataLoader) saveToCache(symbol, timeframe string, candles []adapter.Candle) {
	for _, c := range candles {
		d.db.Exec(
			`INSERT OR REPLACE INTO candle_cache (symbol, timeframe, timestamp, open, high, low, close, volume)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			symbol, timeframe, c.Timestamp.Unix(), c.Open, c.High, c.Low, c.Close, c.Volume,
		)
	}
}

// timeframeToDuration converts a timeframe string to a time.Duration.
func timeframeToDuration(tf string) time.Duration {
	tf = strings.ToLower(tf)
	if strings.HasSuffix(tf, "w") {
		n, _ := strconv.Atoi(strings.TrimSuffix(tf, "w"))
		if n == 0 {
			n = 1
		}
		return time.Duration(n) * 7 * 24 * time.Hour
	}
	if strings.HasSuffix(tf, "d") {
		n, _ := strconv.Atoi(strings.TrimSuffix(tf, "d"))
		if n == 0 {
			n = 1
		}
		return time.Duration(n) * 24 * time.Hour
	}
	if strings.HasSuffix(tf, "h") {
		n, _ := strconv.Atoi(strings.TrimSuffix(tf, "h"))
		if n == 0 {
			n = 1
		}
		return time.Duration(n) * time.Hour
	}
	if strings.HasSuffix(tf, "m") {
		n, _ := strconv.Atoi(strings.TrimSuffix(tf, "m"))
		if n == 0 {
			n = 1
		}
		return time.Duration(n) * time.Minute
	}
	return time.Hour // default
}
```

**Step 4: Run tests**

Run: `cd /d/Clawtrade && go test ./internal/backtest/ -v -run "TestDataLoader|TestTimeframe"`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/backtest/data.go internal/backtest/data_test.go internal/database/migrations.go
git commit -m "feat(backtest): add data loader with SQLite cache and pagination"
```

---

### Task 3: Strategy Executor (Code + Config modes)

The strategy executor provides a unified interface for running strategies against candle data. Three modes: CodeStrategy wraps existing `strategy.Arena`, ConfigStrategy evaluates simple rule expressions, AIStrategy deferred to Task 4.

**Files:**
- Create: `internal/backtest/strategy.go`
- Add tests to: `internal/backtest/portfolio_test.go` (rename to `backtest_test.go` or keep adding)

**Context:**
- `strategy.Arena.RunSignal(symbol, prices)` returns `map[string]*strategy.Signal` where Signal has Side ("buy"/"sell") and Strength (0-1)
- `analysis.SMA/EMA/RSI/MACD/BollingerBands` all take `[]analysis.Candle` and return slices
- `analysis.Candle` has same fields as `adapter.Candle` but with `Timestamp int64` instead of `time.Time`
- ConfigStrategy needs a simple expression parser: `"rsi < 30 AND close > sma_50"` → evaluate against pre-computed indicator values

**Step 1: Write strategy_test.go**

```go
// internal/backtest/strategy_test.go
package backtest

import (
	"testing"
	"time"

	"github.com/clawtrade/clawtrade/internal/adapter"
	"github.com/clawtrade/clawtrade/internal/strategy"
)

func TestCodeStrategy(t *testing.T) {
	arena := strategy.NewArena()
	// Register a simple momentum strategy
	arena.Register("test_momentum", "buy when last price > first", func(symbol string, prices []float64) *strategy.Signal {
		if len(prices) < 2 {
			return nil
		}
		if prices[len(prices)-1] > prices[0] {
			return &strategy.Signal{Symbol: symbol, Side: "buy", Strength: 0.8}
		}
		return &strategy.Signal{Symbol: symbol, Side: "sell", Strength: 0.6}
	})

	cs := &CodeStrategy{Arena: arena, StrategyName: "test_momentum"}

	// Rising prices → buy signal
	candles := makeCandles([]float64{100, 101, 102, 103, 104})
	sig := cs.Evaluate("BTC/USDT", candles)
	if sig == nil {
		t.Fatal("expected signal, got nil")
	}
	if sig.Action != ActionBuy {
		t.Fatalf("expected buy, got %s", sig.Action)
	}

	// Falling prices → sell signal
	candles2 := makeCandles([]float64{104, 103, 102, 101, 100})
	sig2 := cs.Evaluate("BTC/USDT", candles2)
	if sig2 == nil {
		t.Fatal("expected signal, got nil")
	}
	if sig2.Action != ActionSell {
		t.Fatalf("expected sell, got %s", sig2.Action)
	}
}

func TestConfigStrategy(t *testing.T) {
	cfg := &ConfigStrategy{
		BuyWhen:  "rsi < 35",
		SellWhen: "rsi > 65",
	}

	// Generate enough candles for RSI calculation (need > 14 candles)
	// Create a downtrend so RSI is low
	prices := make([]float64, 30)
	for i := range prices {
		prices[i] = 100 - float64(i)*0.5 // declining
	}
	candles := makeCandles(prices)

	sig := cfg.Evaluate("BTC/USDT", candles)
	// RSI should be low in a downtrend → buy signal
	if sig != nil && sig.Action != ActionBuy {
		t.Logf("signal action: %s", sig.Action)
	}
	// This is a smoke test — the key is that it doesn't panic
}

func makeCandles(closes []float64) []adapter.Candle {
	candles := make([]adapter.Candle, len(closes))
	base := time.Now().Add(-time.Duration(len(closes)) * time.Hour)
	for i, c := range closes {
		candles[i] = adapter.Candle{
			Open: c - 1, High: c + 2, Low: c - 2, Close: c, Volume: 1000,
			Timestamp: base.Add(time.Duration(i) * time.Hour),
		}
	}
	return candles
}
```

**Step 2: Write strategy.go**

```go
// internal/backtest/strategy.go
package backtest

import (
	"math"
	"strconv"
	"strings"

	"github.com/clawtrade/clawtrade/internal/adapter"
	"github.com/clawtrade/clawtrade/internal/analysis"
	"github.com/clawtrade/clawtrade/internal/strategy"
)

// Action represents a trading action.
type Action string

const (
	ActionBuy  Action = "buy"
	ActionSell Action = "sell"
	ActionHold Action = "hold"
)

// BacktestSignal is the output of a strategy evaluation.
type BacktestSignal struct {
	Action     Action  `json:"action"`
	Size       float64 `json:"size,omitempty"`       // 0 = use default sizing
	StopLoss   float64 `json:"stop_loss,omitempty"`
	TakeProfit float64 `json:"take_profit,omitempty"`
	Strength   float64 `json:"strength"`             // 0-1
	Reason     string  `json:"reason,omitempty"`
}

// StrategyRunner evaluates candles and returns a trading signal.
type StrategyRunner interface {
	Evaluate(symbol string, candles []adapter.Candle) *BacktestSignal
}

// ─── Code Strategy ──────────────────────────────────────────────────

// CodeStrategy wraps the existing strategy.Arena to run registered Go strategies.
type CodeStrategy struct {
	Arena        *strategy.Arena
	StrategyName string // which registered strategy to use
}

func (cs *CodeStrategy) Evaluate(symbol string, candles []adapter.Candle) *BacktestSignal {
	prices := make([]float64, len(candles))
	for i, c := range candles {
		prices[i] = c.Close
	}

	signals := cs.Arena.RunSignal(symbol, prices)
	sig, ok := signals[cs.StrategyName]
	if !ok {
		return &BacktestSignal{Action: ActionHold}
	}

	action := ActionHold
	switch strings.ToLower(sig.Side) {
	case "buy":
		action = ActionBuy
	case "sell":
		action = ActionSell
	}

	return &BacktestSignal{
		Action:   action,
		Strength: sig.Strength,
		Reason:   sig.Reason,
	}
}

// ─── Config Strategy ────────────────────────────────────────────────

// ConfigStrategy evaluates simple rule expressions against indicator values.
// Supported: rsi, sma_N, ema_N, close, open, high, low, volume
// Operators: <, >, <=, >=
// Connector: AND (only)
type ConfigStrategy struct {
	BuyWhen  string `json:"buy_when"`
	SellWhen string `json:"sell_when"`
}

func (cs *ConfigStrategy) Evaluate(symbol string, candles []adapter.Candle) *BacktestSignal {
	if len(candles) < 30 {
		return &BacktestSignal{Action: ActionHold}
	}

	indicators := computeIndicators(candles)

	if cs.BuyWhen != "" && evalExpr(cs.BuyWhen, indicators) {
		return &BacktestSignal{Action: ActionBuy, Strength: 0.7, Reason: cs.BuyWhen}
	}
	if cs.SellWhen != "" && evalExpr(cs.SellWhen, indicators) {
		return &BacktestSignal{Action: ActionSell, Strength: 0.7, Reason: cs.SellWhen}
	}
	return &BacktestSignal{Action: ActionHold}
}

// computeIndicators calculates common indicators and returns a value map.
func computeIndicators(candles []adapter.Candle) map[string]float64 {
	ac := toAnalysisCandles(candles)
	last := candles[len(candles)-1]
	vals := map[string]float64{
		"close":  last.Close,
		"open":   last.Open,
		"high":   last.High,
		"low":    last.Low,
		"volume": last.Volume,
	}

	// RSI
	rsi := analysis.RSI(ac, 14)
	if v := rsi[len(rsi)-1]; !math.IsNaN(v) {
		vals["rsi"] = v
	}

	// SMA periods
	for _, p := range []int{10, 20, 50, 100, 200} {
		sma := analysis.SMA(ac, p)
		if v := sma[len(sma)-1]; !math.IsNaN(v) {
			vals["sma_"+strconv.Itoa(p)] = v
		}
	}

	// EMA periods
	for _, p := range []int{9, 12, 21, 26, 50} {
		ema := analysis.EMA(ac, p)
		if v := ema[len(ema)-1]; !math.IsNaN(v) {
			vals["ema_"+strconv.Itoa(p)] = v
		}
	}

	// MACD
	macd := analysis.MACD(ac, 12, 26, 9)
	if v := macd.MACD[len(macd.MACD)-1]; !math.IsNaN(v) {
		vals["macd"] = v
	}
	if v := macd.Signal[len(macd.Signal)-1]; !math.IsNaN(v) {
		vals["macd_signal"] = v
	}
	if v := macd.Histogram[len(macd.Histogram)-1]; !math.IsNaN(v) {
		vals["macd_hist"] = v
	}

	// Bollinger Bands
	bb := analysis.BollingerBands(ac, 20, 2.0)
	if v := bb.Upper[len(bb.Upper)-1]; !math.IsNaN(v) {
		vals["bb_upper"] = v
	}
	if v := bb.Lower[len(bb.Lower)-1]; !math.IsNaN(v) {
		vals["bb_lower"] = v
	}

	return vals
}

func toAnalysisCandles(candles []adapter.Candle) []analysis.Candle {
	ac := make([]analysis.Candle, len(candles))
	for i, c := range candles {
		ac[i] = analysis.Candle{
			Open: c.Open, High: c.High, Low: c.Low, Close: c.Close,
			Volume: c.Volume, Timestamp: c.Timestamp.Unix(),
		}
	}
	return ac
}

// evalExpr evaluates simple expressions like "rsi < 30 AND close > sma_50"
func evalExpr(expr string, vals map[string]float64) bool {
	parts := strings.Split(strings.ToLower(expr), " and ")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if !evalCondition(part, vals) {
			return false
		}
	}
	return true
}

func evalCondition(cond string, vals map[string]float64) bool {
	operators := []string{"<=", ">=", "<", ">"}
	for _, op := range operators {
		if idx := strings.Index(cond, op); idx > 0 {
			left := strings.TrimSpace(cond[:idx])
			right := strings.TrimSpace(cond[idx+len(op):])

			leftVal := resolveValue(left, vals)
			rightVal := resolveValue(right, vals)

			if math.IsNaN(leftVal) || math.IsNaN(rightVal) {
				return false
			}

			switch op {
			case "<":
				return leftVal < rightVal
			case ">":
				return leftVal > rightVal
			case "<=":
				return leftVal <= rightVal
			case ">=":
				return leftVal >= rightVal
			}
		}
	}
	return false
}

func resolveValue(s string, vals map[string]float64) float64 {
	s = strings.TrimSpace(s)
	if v, ok := vals[s]; ok {
		return v
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return math.NaN()
}
```

**Step 3: Run tests**

Run: `cd /d/Clawtrade && go test ./internal/backtest/ -v -run "TestCodeStrategy|TestConfigStrategy"`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/backtest/strategy.go internal/backtest/strategy_test.go
git commit -m "feat(backtest): add strategy executor with code and config modes"
```

---

### Task 4: Backtest Engine Core

The engine orchestrates everything: loads data, runs the time loop, feeds candles to strategy, executes signals on portfolio, collects metrics, and emits events.

**Files:**
- Create: `internal/backtest/engine.go`
- Create: `internal/backtest/engine_test.go`

**Context:**
- Uses `DataLoader` from Task 2, `StrategyRunner` from Task 3, `Portfolio` from Task 1
- Emits events via `engine.EventBus` (optional, nil-safe): `backtest.progress` and `backtest.complete`
- `BacktestResult` contains metrics: TotalPnL, WinRate, MaxDrawdown, SharpeRatio, ProfitFactor, Trades, EquityCurve
- Metrics calculation reuses logic from `strategy.Arena.RecordResult` pattern (computeSharpe)

**Step 1: Write engine_test.go**

```go
// internal/backtest/engine_test.go
package backtest

import (
	"context"
	"testing"
	"time"

	"github.com/clawtrade/clawtrade/internal/adapter"
	enginepkg "github.com/clawtrade/clawtrade/internal/engine"
)

func TestEngine_RunBasicBacktest(t *testing.T) {
	// Create candles with a clear uptrend then downtrend
	now := time.Now().Truncate(time.Hour)
	candles := make([]adapter.Candle, 50)
	for i := 0; i < 50; i++ {
		var price float64
		if i < 30 {
			price = 100 + float64(i)*2 // uptrend: 100→158
		} else {
			price = 158 - float64(i-30)*3 // downtrend: 158→98
		}
		candles[i] = adapter.Candle{
			Open: price - 1, High: price + 2, Low: price - 2, Close: price,
			Volume: 1000, Timestamp: now.Add(time.Duration(i) * time.Hour),
		}
	}

	// Simple strategy: buy when price rises 3 candles in a row, sell when drops 3 in a row
	strat := &simpleTestStrategy{}

	bus := enginepkg.NewEventBus()
	var progressCount int
	bus.Subscribe("backtest.progress", func(e enginepkg.Event) {
		progressCount++
	})

	eng := &Engine{Bus: bus}
	cfg := BacktestConfig{
		Symbol:      "TEST/USDT",
		Timeframe:   "1h",
		From:        now,
		To:          now.Add(49 * time.Hour),
		Capital:     10000,
		MakerFee:    0.001,
		TakerFee:    0.001,
		Slippage:    0,
		TradeSize:   1.0, // fixed 1 unit per trade
	}

	result, err := eng.Run(context.Background(), cfg, candles, strat)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if result.TotalTrades == 0 {
		t.Fatal("expected some trades")
	}
	if len(result.EquityCurve) != 50 {
		t.Fatalf("expected 50 equity points, got %d", len(result.EquityCurve))
	}

	// Should have received progress events
	// (async, give a moment)
	time.Sleep(50 * time.Millisecond)
	if progressCount == 0 {
		t.Fatal("expected progress events")
	}

	t.Logf("Trades: %d, PnL: %.2f, WinRate: %.1f%%, MaxDD: %.2f, Sharpe: %.2f",
		result.TotalTrades, result.TotalPnL, result.WinRate*100, result.MaxDrawdown, result.SharpeRatio)
}

// simpleTestStrategy buys after 3 consecutive up-candles, sells after 3 down
type simpleTestStrategy struct{}

func (s *simpleTestStrategy) Evaluate(symbol string, candles []adapter.Candle) *BacktestSignal {
	n := len(candles)
	if n < 4 {
		return &BacktestSignal{Action: ActionHold}
	}
	ups, downs := 0, 0
	for i := n - 3; i < n; i++ {
		if candles[i].Close > candles[i-1].Close {
			ups++
		} else {
			downs++
		}
	}
	if ups == 3 {
		return &BacktestSignal{Action: ActionBuy, Strength: 0.8}
	}
	if downs == 3 {
		return &BacktestSignal{Action: ActionSell, Strength: 0.8}
	}
	return &BacktestSignal{Action: ActionHold}
}

func TestEngine_Metrics(t *testing.T) {
	// Test metric calculations
	returns := []float64{100, -50, 200, -30, 150, -80, 120}
	sharpe := computeSharpeRatio(returns)
	if sharpe == 0 {
		t.Fatal("expected non-zero sharpe")
	}

	maxDD := computeMaxDrawdown([]float64{10000, 10500, 10200, 9800, 10100, 9500, 10000})
	if maxDD <= 0 {
		t.Fatal("expected positive max drawdown")
	}
	t.Logf("Sharpe: %.4f, MaxDD: %.2f", sharpe, maxDD)
}
```

**Step 2: Write engine.go**

```go
// internal/backtest/engine.go
package backtest

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/clawtrade/clawtrade/internal/adapter"
	"github.com/clawtrade/clawtrade/internal/engine"
)

// BacktestConfig holds configuration for a backtest run.
type BacktestConfig struct {
	Symbol    string    `json:"symbol"`
	Timeframe string    `json:"timeframe"`
	From      time.Time `json:"from"`
	To        time.Time `json:"to"`
	Capital   float64   `json:"capital"`
	MakerFee  float64   `json:"maker_fee"`
	TakerFee  float64   `json:"taker_fee"`
	Slippage  float64   `json:"slippage"`
	TradeSize float64   `json:"trade_size"` // fixed size per trade (units)
}

// BacktestResult holds the output of a backtest run.
type BacktestResult struct {
	Symbol       string         `json:"symbol"`
	Timeframe    string         `json:"timeframe"`
	Period       string         `json:"period"`
	Capital      float64        `json:"capital"`
	FinalEquity  float64        `json:"final_equity"`
	TotalPnL     float64        `json:"total_pnl"`
	TotalReturn  float64        `json:"total_return"` // percentage
	TotalTrades  int            `json:"total_trades"`
	WinCount     int            `json:"win_count"`
	LossCount    int            `json:"loss_count"`
	WinRate      float64        `json:"win_rate"`
	AvgWin       float64        `json:"avg_win"`
	AvgLoss      float64        `json:"avg_loss"`
	ProfitFactor float64        `json:"profit_factor"`
	MaxDrawdown  float64        `json:"max_drawdown"`
	SharpeRatio  float64        `json:"sharpe_ratio"`
	Trades       []TradeRecord  `json:"trades"`
	EquityCurve  []EquityPoint  `json:"equity_curve"`
}

// EquityPoint is a snapshot of portfolio value at a point in time.
type EquityPoint struct {
	Time   time.Time `json:"time"`
	Equity float64   `json:"equity"`
	Price  float64   `json:"price"`
}

// Engine orchestrates backtesting.
type Engine struct {
	Bus *engine.EventBus // optional, for streaming progress
}

// Run executes a backtest with the given config, candles, and strategy.
func (e *Engine) Run(ctx context.Context, cfg BacktestConfig, candles []adapter.Candle, strat StrategyRunner) (*BacktestResult, error) {
	if len(candles) == 0 {
		return nil, fmt.Errorf("no candle data")
	}

	portfolio := NewPortfolio(cfg.Capital, cfg.MakerFee, cfg.TakerFee, cfg.Slippage)
	equityCurve := make([]EquityPoint, 0, len(candles))
	tradeSize := cfg.TradeSize
	if tradeSize == 0 {
		tradeSize = 1.0
	}

	// Time loop: iterate through each candle
	for i, candle := range candles {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		currentPrice := candle.Close

		// Check SL/TP on existing positions
		portfolio.Tick(currentPrice, candle.Timestamp)

		// Feed candle window to strategy (provide all candles up to current)
		window := candles[:i+1]
		signal := strat.Evaluate(cfg.Symbol, window)

		// Execute signal
		if signal != nil {
			switch signal.Action {
			case ActionBuy:
				if len(portfolio.Positions) == 0 { // no existing position
					size := tradeSize
					if signal.Size > 0 {
						size = signal.Size
					}
					err := portfolio.OpenPosition(cfg.Symbol, SideLong, size, currentPrice, candle.Timestamp)
					if err == nil && (signal.StopLoss > 0 || signal.TakeProfit > 0) {
						pos := &portfolio.Positions[len(portfolio.Positions)-1]
						pos.StopLoss = signal.StopLoss
						pos.TakeProfit = signal.TakeProfit
					}
				}
			case ActionSell:
				// Close any open long positions
				for j := len(portfolio.Positions) - 1; j >= 0; j-- {
					if portfolio.Positions[j].Side == SideLong {
						portfolio.ClosePosition(j, currentPrice, candle.Timestamp)
					}
				}
			}
		}

		// Record equity
		eq := portfolio.Equity(currentPrice)
		equityCurve = append(equityCurve, EquityPoint{
			Time:   candle.Timestamp,
			Equity: eq,
			Price:  currentPrice,
		})

		// Emit progress event (throttled: every 10 candles or last candle)
		if e.Bus != nil && (i%10 == 0 || i == len(candles)-1) {
			e.Bus.Publish(engine.Event{
				Type: "backtest.progress",
				Data: map[string]any{
					"index":    i,
					"total":    len(candles),
					"equity":   eq,
					"price":    currentPrice,
					"symbol":   cfg.Symbol,
					"positions": len(portfolio.Positions),
				},
			})
		}
	}

	// Close any remaining positions at last price
	lastPrice := candles[len(candles)-1].Close
	lastTime := candles[len(candles)-1].Timestamp
	for len(portfolio.Positions) > 0 {
		portfolio.ClosePosition(0, lastPrice, lastTime)
	}

	// Calculate metrics
	result := e.buildResult(cfg, portfolio, equityCurve)

	// Emit complete event
	if e.Bus != nil {
		e.Bus.Publish(engine.Event{
			Type: "backtest.complete",
			Data: map[string]any{
				"symbol":       result.Symbol,
				"total_pnl":    result.TotalPnL,
				"total_return":  result.TotalReturn,
				"total_trades": result.TotalTrades,
				"win_rate":     result.WinRate,
				"max_drawdown": result.MaxDrawdown,
				"sharpe_ratio": result.SharpeRatio,
			},
		})
	}

	return result, nil
}

func (e *Engine) buildResult(cfg BacktestConfig, portfolio *Portfolio, equityCurve []EquityPoint) *BacktestResult {
	trades := portfolio.Trades
	var winCount, lossCount int
	var grossProfit, grossLoss float64
	pnls := make([]float64, len(trades))

	for i, t := range trades {
		pnls[i] = t.PnL
		if t.PnL >= 0 {
			winCount++
			grossProfit += t.PnL
		} else {
			lossCount++
			grossLoss += math.Abs(t.PnL)
		}
	}

	totalTrades := len(trades)
	totalPnL := grossProfit - grossLoss
	winRate := 0.0
	if totalTrades > 0 {
		winRate = float64(winCount) / float64(totalTrades)
	}
	avgWin := 0.0
	if winCount > 0 {
		avgWin = grossProfit / float64(winCount)
	}
	avgLoss := 0.0
	if lossCount > 0 {
		avgLoss = grossLoss / float64(lossCount)
	}
	profitFactor := 0.0
	if grossLoss > 0 {
		profitFactor = grossProfit / grossLoss
	}

	eqValues := make([]float64, len(equityCurve))
	for i, ep := range equityCurve {
		eqValues[i] = ep.Equity
	}

	finalEquity := cfg.Capital
	if len(equityCurve) > 0 {
		finalEquity = equityCurve[len(equityCurve)-1].Equity
	}

	return &BacktestResult{
		Symbol:       cfg.Symbol,
		Timeframe:    cfg.Timeframe,
		Period:       fmt.Sprintf("%s to %s", cfg.From.Format("2006-01-02"), cfg.To.Format("2006-01-02")),
		Capital:      cfg.Capital,
		FinalEquity:  finalEquity,
		TotalPnL:     totalPnL,
		TotalReturn:  (finalEquity - cfg.Capital) / cfg.Capital * 100,
		TotalTrades:  totalTrades,
		WinCount:     winCount,
		LossCount:    lossCount,
		WinRate:      winRate,
		AvgWin:       avgWin,
		AvgLoss:      avgLoss,
		ProfitFactor: profitFactor,
		MaxDrawdown:  computeMaxDrawdown(eqValues),
		SharpeRatio:  computeSharpeRatio(pnls),
		Trades:       trades,
		EquityCurve:  equityCurve,
	}
}

func computeSharpeRatio(returns []float64) float64 {
	n := len(returns)
	if n < 2 {
		return 0
	}
	var sum float64
	for _, r := range returns {
		sum += r
	}
	mean := sum / float64(n)
	var variance float64
	for _, r := range returns {
		diff := r - mean
		variance += diff * diff
	}
	variance /= float64(n - 1)
	stddev := math.Sqrt(variance)
	if stddev == 0 {
		return 0
	}
	return mean / stddev
}

func computeMaxDrawdown(equity []float64) float64 {
	if len(equity) == 0 {
		return 0
	}
	peak := equity[0]
	maxDD := 0.0
	for _, eq := range equity {
		if eq > peak {
			peak = eq
		}
		dd := peak - eq
		if dd > maxDD {
			maxDD = dd
		}
	}
	return maxDD
}
```

**Step 3: Run tests**

Run: `cd /d/Clawtrade && go test ./internal/backtest/ -v -run "TestEngine"`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/backtest/engine.go internal/backtest/engine_test.go
git commit -m "feat(backtest): add engine core with time-loop, metrics, and event streaming"
```

---

### Task 5: CLI Command

Add `clawtrade backtest` CLI command that loads config, connects to exchange, runs backtest, and prints results as a formatted table.

**Files:**
- Modify: `cmd/clawtrade/main.go` — add `case "backtest"` and `handleBacktest()` function

**Context:**
- Follow existing CLI pattern: `case "backtest": err = handleBacktest(os.Args[2:])`
- Add to `printUsage()` under a new "Backtesting:" section
- Need to parse flags: `--symbol`, `--timeframe`, `--from`, `--to`, `--strategy`, `--capital`, `--config` (YAML rule file)
- Uses `database.Open()` for SQLite cache, adapter for candle data
- For config mode: read YAML file with `buy_when`/`sell_when` rules
- For code mode: need to register built-in strategies (create a few common ones)

**Step 1: Add backtest command to main.go switch**

In `cmd/clawtrade/main.go`, add this case after `case "status"`:

```go
case "backtest":
    err = handleBacktest(os.Args[2:])
```

Add to `printUsage()` before `Environment:`:

```go
Backtesting:
  backtest               Run a backtest (use --help for options)
```

Add the import for `"github.com/clawtrade/clawtrade/internal/backtest"` at the top.

**Step 2: Write handleBacktest function**

Append to `cmd/clawtrade/main.go`:

```go
// ─── backtest ─────────────────────────────────────────────────────────

func handleBacktest(args []string) error {
	symbol := "BTC/USDT"
	timeframe := "1d"
	fromStr := ""
	toStr := ""
	strategyName := ""
	configFile := ""
	capital := 10000.0

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--symbol", "-s":
			if i+1 < len(args) { i++; symbol = args[i] }
		case "--timeframe", "-t":
			if i+1 < len(args) { i++; timeframe = args[i] }
		case "--from":
			if i+1 < len(args) { i++; fromStr = args[i] }
		case "--to":
			if i+1 < len(args) { i++; toStr = args[i] }
		case "--strategy":
			if i+1 < len(args) { i++; strategyName = args[i] }
		case "--config":
			if i+1 < len(args) { i++; configFile = args[i] }
		case "--capital":
			if i+1 < len(args) {
				i++
				if v, err := strconv.ParseFloat(args[i], 64); err == nil { capital = v }
			}
		case "--help", "-h":
			fmt.Println(`Usage: clawtrade backtest [options]

Options:
  --symbol, -s      Trading pair (default: BTC/USDT)
  --timeframe, -t   Candle timeframe: 1m,5m,15m,1h,4h,1d,1w (default: 1d)
  --from            Start date YYYY-MM-DD (default: 90 days ago)
  --to              End date YYYY-MM-DD (default: today)
  --strategy        Strategy name: momentum, meanrevert, macd_cross
  --config          Path to YAML config with buy_when/sell_when rules
  --capital         Initial capital in USD (default: 10000)

Examples:
  clawtrade backtest --symbol BTC/USDT --timeframe 1d --strategy momentum
  clawtrade backtest --symbol ETH/USDT --timeframe 4h --config rules.yaml --capital 50000`)
			return nil
		}
	}

	// Parse dates
	now := time.Now()
	to := now
	from := now.AddDate(0, -3, 0) // default 90 days
	if fromStr != "" {
		if t, err := time.Parse("2006-01-02", fromStr); err == nil { from = t }
	}
	if toStr != "" {
		if t, err := time.Parse("2006-01-02", toStr); err == nil { to = t }
	}

	// Load config for exchange credentials
	cfg, err := config.Load(configPath)
	if err != nil {
		cfg, _ = config.Load("")
	}

	// Open DB for candle cache
	db, err := database.Open(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	// Find a connected adapter
	var adp adapter.TradingAdapter
	for _, ex := range cfg.Exchanges {
		if !ex.Enabled { continue }
		switch ex.Name {
		case "binance":
			ba := binance.New(ex.Fields["api_key"], ex.Fields["api_secret"])
			if err := ba.Connect(context.Background()); err == nil { adp = ba }
		case "bybit":
			by := bybit.New(ex.Fields["api_key"], ex.Fields["api_secret"])
			if err := by.Connect(context.Background()); err == nil { adp = by }
		}
		if adp != nil { break }
	}

	// Load candles
	loader := backtest.NewDataLoader(db, adp)
	candles, err := loader.LoadCandles(context.Background(), symbol, timeframe, from, to)
	if err != nil || len(candles) == 0 {
		return fmt.Errorf("no candle data available for %s (try connecting an exchange first)", symbol)
	}

	// Build strategy
	var strat backtest.StrategyRunner
	if configFile != "" {
		data, err := os.ReadFile(configFile)
		if err != nil {
			return fmt.Errorf("read config: %w", err)
		}
		// Simple parse: look for buy_when and sell_when lines
		buyWhen, sellWhen := "", ""
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "buy_when:") {
				buyWhen = strings.TrimSpace(strings.TrimPrefix(line, "buy_when:"))
				buyWhen = strings.Trim(buyWhen, "\"'")
			}
			if strings.HasPrefix(line, "sell_when:") {
				sellWhen = strings.TrimSpace(strings.TrimPrefix(line, "sell_when:"))
				sellWhen = strings.Trim(sellWhen, "\"'")
			}
		}
		strat = &backtest.ConfigStrategy{BuyWhen: buyWhen, SellWhen: sellWhen}
	} else {
		strat = backtest.GetBuiltinStrategy(strategyName)
	}

	// Run backtest
	bus := engine.NewEventBus()
	eng := &backtest.Engine{Bus: bus}
	btCfg := backtest.BacktestConfig{
		Symbol:    symbol,
		Timeframe: timeframe,
		From:      from,
		To:        to,
		Capital:   capital,
		MakerFee:  0.001,
		TakerFee:  0.001,
		Slippage:  0.0005,
		TradeSize: 1.0,
	}

	fmt.Printf("Running backtest: %s %s from %s to %s (%d candles)\n",
		symbol, timeframe, from.Format("2006-01-02"), to.Format("2006-01-02"), len(candles))

	result, err := eng.Run(context.Background(), btCfg, candles, strat)
	if err != nil {
		return fmt.Errorf("backtest: %w", err)
	}

	// Print results
	printBacktestResult(result)
	return nil
}

func printBacktestResult(r *backtest.BacktestResult) {
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════")
	fmt.Printf("  Backtest Results: %s (%s)\n", r.Symbol, r.Period)
	fmt.Println("═══════════════════════════════════════════════════")
	fmt.Printf("  Capital:        $%.2f\n", r.Capital)
	fmt.Printf("  Final Equity:   $%.2f\n", r.FinalEquity)

	pnlSign := "+"
	if r.TotalPnL < 0 { pnlSign = "" }
	fmt.Printf("  Total P&L:      %s$%.2f (%s%.2f%%)\n", pnlSign, r.TotalPnL, pnlSign, r.TotalReturn)
	fmt.Println("───────────────────────────────────────────────────")
	fmt.Printf("  Total Trades:   %d\n", r.TotalTrades)
	fmt.Printf("  Wins / Losses:  %d / %d\n", r.WinCount, r.LossCount)
	fmt.Printf("  Win Rate:       %.1f%%\n", r.WinRate*100)
	fmt.Printf("  Avg Win:        $%.2f\n", r.AvgWin)
	fmt.Printf("  Avg Loss:       $%.2f\n", r.AvgLoss)
	fmt.Printf("  Profit Factor:  %.2f\n", r.ProfitFactor)
	fmt.Println("───────────────────────────────────────────────────")
	fmt.Printf("  Max Drawdown:   $%.2f\n", r.MaxDrawdown)
	fmt.Printf("  Sharpe Ratio:   %.2f\n", r.SharpeRatio)
	fmt.Println("═══════════════════════════════════════════════════")

	if len(r.Trades) > 0 {
		fmt.Println()
		fmt.Println("  Recent Trades:")
		shown := r.Trades
		if len(shown) > 10 { shown = shown[len(shown)-10:] }
		for _, t := range shown {
			pSign := "+"
			if t.PnL < 0 { pSign = "" }
			fmt.Printf("    %s  %s  %.4f @ %.2f → %.2f  %s%.2f\n",
				t.ClosedAt.Format("01-02 15:04"), strings.ToUpper(t.Side),
				t.Size, t.EntryPrice, t.ExitPrice, pSign, t.PnL)
		}
	}
	fmt.Println()
}
```

**Step 3: Add built-in strategies to strategy.go**

Append to `internal/backtest/strategy.go`:

```go
// GetBuiltinStrategy returns a built-in strategy by name.
func GetBuiltinStrategy(name string) StrategyRunner {
	switch strings.ToLower(name) {
	case "momentum":
		return &ConfigStrategy{
			BuyWhen:  "rsi < 35 AND close > ema_21",
			SellWhen: "rsi > 70",
		}
	case "meanrevert", "mean_revert":
		return &ConfigStrategy{
			BuyWhen:  "close < bb_lower",
			SellWhen: "close > bb_upper",
		}
	case "macd_cross", "macd":
		return &ConfigStrategy{
			BuyWhen:  "macd > macd_signal AND macd_hist > 0",
			SellWhen: "macd < macd_signal AND macd_hist < 0",
		}
	default:
		// Default: simple RSI strategy
		return &ConfigStrategy{
			BuyWhen:  "rsi < 30",
			SellWhen: "rsi > 70",
		}
	}
}
```

**Step 4: Run full test suite**

Run: `cd /d/Clawtrade && go test ./internal/backtest/ -v`
Expected: PASS (all tests)

Run: `cd /d/Clawtrade && go build ./cmd/clawtrade/`
Expected: Build succeeds

**Step 5: Commit**

```bash
git add cmd/clawtrade/main.go internal/backtest/strategy.go
git commit -m "feat(backtest): add CLI command with built-in strategies"
```

---

### Task 6: Agent Tool Integration

Add a `backtest` tool to the AI agent's tool registry so the agent can run backtests programmatically.

**Files:**
- Modify: `internal/agent/tools.go` — add backtest tool definition and executor

**Context:**
- Follow existing tool pattern: add to `Definitions()` slice, add case to `Execute()` switch, add `execBacktest()` method
- Add `"backtest": true` to `builtinTools` map
- Tool needs access to `*sql.DB` for candle cache — add `db` field to `ToolRegistry`
- Update `NewToolRegistry` to accept `db *sql.DB` parameter
- Update call sites in `internal/api/server.go` and anywhere else that creates `ToolRegistry`

**Step 1: Add db field to ToolRegistry**

In `internal/agent/tools.go`, add `db *sql.DB` field:

```go
type ToolRegistry struct {
	adapters   map[string]adapter.TradingAdapter
	riskEngine *risk.Engine
	mcpBridge  MCPBridge
	bus        *engine.EventBus
	db         *sql.DB
}
```

Update `NewToolRegistry`:
```go
func NewToolRegistry(adapters map[string]adapter.TradingAdapter, riskEngine *risk.Engine, bus *engine.EventBus, db *sql.DB) *ToolRegistry {
	return &ToolRegistry{
		adapters:   adapters,
		riskEngine: riskEngine,
		bus:        bus,
		db:         db,
	}
}
```

**Step 2: Add backtest tool definition**

In the `Definitions()` method, add after the `get_open_orders` definition:

```go
{
    Name:        "backtest",
    Description: "Run a backtest on historical data to evaluate a trading strategy. Returns performance metrics including P&L, win rate, Sharpe ratio, and max drawdown. Use this to test strategy ideas before trading live.",
    InputSchema: map[string]any{
        "type": "object",
        "properties": map[string]any{
            "symbol":    map[string]any{"type": "string", "description": "Trading pair, e.g. BTC/USDT"},
            "timeframe": map[string]any{"type": "string", "description": "Candle timeframe: 1h, 4h, 1d", "default": "1d"},
            "days":      map[string]any{"type": "integer", "description": "Number of days to backtest (default: 90)", "default": 90},
            "strategy":  map[string]any{"type": "string", "description": "Strategy: momentum, meanrevert, macd_cross, or custom rule like 'rsi < 30'", "default": "momentum"},
            "capital":   map[string]any{"type": "number", "description": "Initial capital in USD (default: 10000)", "default": 10000},
            "exchange":  map[string]any{"type": "string", "description": "Exchange for data (default: binance)", "default": "binance"},
        },
        "required": []string{"symbol"},
    },
},
```

**Step 3: Add to builtinTools and Execute switch**

Add `"backtest": true` to `builtinTools` map.

Add case to `Execute()`:
```go
case "backtest":
    return tr.execBacktest(ctx, call)
```

**Step 4: Write execBacktest method**

```go
func (tr *ToolRegistry) execBacktest(ctx context.Context, call ToolCall) ToolResult {
	symbol := getString(call.Input, "symbol", "BTC/USDT")
	timeframe := getString(call.Input, "timeframe", "1d")
	days := getInt(call.Input, "days", 90)
	strategyStr := getString(call.Input, "strategy", "momentum")
	capital := getFloat(call.Input, "capital", 10000)
	exchange := getString(call.Input, "exchange", "binance")

	// Get adapter for data
	adp, _ := tr.adapters[exchange]

	// Load candles
	loader := backtest.NewDataLoader(tr.db, adp)
	from := time.Now().AddDate(0, 0, -days)
	to := time.Now()
	candles, err := loader.LoadCandles(ctx, symbol, timeframe, from, to)
	if err != nil || len(candles) == 0 {
		return ToolResult{ID: call.ID, Content: fmt.Sprintf("No candle data for %s. Ensure exchange is connected.", symbol), IsError: true}
	}

	// Build strategy
	var strat backtest.StrategyRunner
	if strings.Contains(strategyStr, "<") || strings.Contains(strategyStr, ">") {
		// Custom rule expression
		strat = &backtest.ConfigStrategy{BuyWhen: strategyStr, SellWhen: ""}
	} else {
		strat = backtest.GetBuiltinStrategy(strategyStr)
	}

	// Run
	eng := &backtest.Engine{Bus: tr.bus}
	cfg := backtest.BacktestConfig{
		Symbol: symbol, Timeframe: timeframe, From: from, To: to,
		Capital: capital, MakerFee: 0.001, TakerFee: 0.001, Slippage: 0.0005, TradeSize: 1.0,
	}
	result, err := eng.Run(ctx, cfg, candles, strat)
	if err != nil {
		return ToolResult{ID: call.ID, Content: fmt.Sprintf("Backtest error: %v", err), IsError: true}
	}

	// Format result as readable text
	out := fmt.Sprintf(`Backtest Results: %s (%s)
Period: %s | %d candles
Capital: $%.0f → $%.0f (%+.2f%%)
Trades: %d (Win: %d, Loss: %d) | Win Rate: %.1f%%
P&L: $%.2f | Profit Factor: %.2f
Max Drawdown: $%.2f | Sharpe: %.2f`,
		symbol, timeframe, result.Period, len(candles),
		result.Capital, result.FinalEquity, result.TotalReturn,
		result.TotalTrades, result.WinCount, result.LossCount, result.WinRate*100,
		result.TotalPnL, result.ProfitFactor,
		result.MaxDrawdown, result.SharpeRatio)

	data, _ := json.Marshal(result)
	return ToolResult{ID: call.ID, Content: out + "\n\n[Full JSON available in backtest.complete event]\n" + string(data)}
}
```

**Step 5: Update agent.New() and server.go to pass db**

In `internal/agent/agent.go`, update `New()` to accept `db *sql.DB` and pass to `NewToolRegistry`.

In `internal/api/server.go`, update the `agent.New()` call to pass `db`.

In `cmd/clawtrade/main.go` serve function, ensure `db` is passed through to the server/agent chain.

**Step 6: Run build**

Run: `cd /d/Clawtrade && go build ./cmd/clawtrade/`
Expected: Build succeeds

**Step 7: Commit**

```bash
git add internal/agent/tools.go internal/agent/agent.go internal/api/server.go cmd/clawtrade/main.go
git commit -m "feat(backtest): add backtest agent tool for AI-driven strategy testing"
```

---

### Task 7: API Endpoint + WebSocket Events

Add `POST /api/v1/backtest` endpoint and subscribe to `backtest.*` events on WebSocket hub.

**Files:**
- Modify: `internal/api/server.go` — add route and handler

**Context:**
- Follow existing endpoint pattern (e.g. `handleGetPrice`)
- Accept JSON body: `{symbol, timeframe, days, strategy, capital, exchange}`
- Run backtest synchronously (most complete in <5 seconds for <500 candles)
- Events `backtest.progress` and `backtest.complete` are already emitted by engine
- Just need to add `"backtest.*"` to `hub.SubscribeToEvents` patterns

**Step 1: Add backtest route**

In `internal/api/server.go`, inside `r.Route("/api/v1", ...)` add:

```go
r.Post("/backtest", s.handleBacktest)
```

**Step 2: Add backtest.* to WebSocket subscriptions**

Add `"backtest.*"` to the `hub.SubscribeToEvents` slice.

**Step 3: Write handler**

```go
func (s *Server) handleBacktest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Symbol    string  `json:"symbol"`
		Timeframe string  `json:"timeframe"`
		Days      int     `json:"days"`
		Strategy  string  `json:"strategy"`
		Capital   float64 `json:"capital"`
		Exchange  string  `json:"exchange"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Symbol == "" { req.Symbol = "BTC/USDT" }
	if req.Timeframe == "" { req.Timeframe = "1d" }
	if req.Days == 0 { req.Days = 90 }
	if req.Strategy == "" { req.Strategy = "momentum" }
	if req.Capital == 0 { req.Capital = 10000 }
	if req.Exchange == "" { req.Exchange = "binance" }

	adp := s.adapters[req.Exchange]
	loader := backtest.NewDataLoader(nil, adp) // no DB cache for API calls
	from := time.Now().AddDate(0, 0, -req.Days)
	to := time.Now()

	candles, err := loader.LoadCandles(r.Context(), req.Symbol, req.Timeframe, from, to)
	if err != nil || len(candles) == 0 {
		http.Error(w, "no candle data available", http.StatusServiceUnavailable)
		return
	}

	strat := backtest.GetBuiltinStrategy(req.Strategy)
	eng := &backtest.Engine{Bus: s.bus}
	cfg := backtest.BacktestConfig{
		Symbol: req.Symbol, Timeframe: req.Timeframe, From: from, To: to,
		Capital: req.Capital, MakerFee: 0.001, TakerFee: 0.001, Slippage: 0.0005, TradeSize: 1.0,
	}

	result, err := eng.Run(r.Context(), cfg, candles, strat)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
```

**Step 4: Add import**

Add `"github.com/clawtrade/clawtrade/internal/backtest"` to imports in server.go.

**Step 5: Build and verify**

Run: `cd /d/Clawtrade && go build ./cmd/clawtrade/`
Expected: Build succeeds

**Step 6: Commit**

```bash
git add internal/api/server.go
git commit -m "feat(backtest): add API endpoint and WebSocket event streaming"
```

---

### Task 8: Add ChatPanel tool label for backtest

Update the frontend `ChatPanel.tsx` to show a friendly label when the agent uses the backtest tool.

**Files:**
- Modify: `web/src/components/ChatPanel.tsx`

**Context:**
- `TOOL_LABELS` map at line 32 maps tool names to display labels
- Just add one entry: `backtest: 'Running backtest'`

**Step 1: Add label**

In `web/src/components/ChatPanel.tsx`, add to `TOOL_LABELS`:

```typescript
backtest: 'Running backtest',
```

**Step 2: Commit**

```bash
git add web/src/components/ChatPanel.tsx
git commit -m "feat(web): add backtest tool label to ChatPanel"
```

---

### Summary

| Task | Component | Files |
|------|-----------|-------|
| 1 | Portfolio Simulator | `internal/backtest/portfolio.go`, `portfolio_test.go` |
| 2 | Data Loader + SQLite Cache | `internal/backtest/data.go`, `data_test.go`, `database/migrations.go` |
| 3 | Strategy Executor | `internal/backtest/strategy.go`, `strategy_test.go` |
| 4 | Engine Core | `internal/backtest/engine.go`, `engine_test.go` |
| 5 | CLI Command | `cmd/clawtrade/main.go` |
| 6 | Agent Tool | `agent/tools.go`, `agent/agent.go`, `api/server.go` |
| 7 | API Endpoint + WebSocket | `api/server.go` |
| 8 | Frontend Tool Label | `web/src/components/ChatPanel.tsx` |

**Execution order:** Tasks 1-3 are independent foundations (can be parallelized). Task 4 depends on 1+3. Tasks 5-6 depend on 4. Task 7 depends on 4. Task 8 is independent.
