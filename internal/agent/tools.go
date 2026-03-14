// internal/agent/tools.go
package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/clawtrade/clawtrade/internal/adapter"
	"github.com/clawtrade/clawtrade/internal/backtest"
	"github.com/clawtrade/clawtrade/internal/engine"
	"github.com/clawtrade/clawtrade/internal/risk"
)

// ToolDef describes a tool available to the AI agent.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// ToolCall represents a tool invocation from the LLM.
type ToolCall struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

// ToolResult holds the result of executing a tool.
type ToolResult struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	IsError bool   `json:"is_error,omitempty"`
}

// MCPBridge allows the tool registry to call external MCP tools.
type MCPBridge interface {
	GetAllTools() []struct {
		Name        string
		Description string
		InputSchema map[string]any
	}
	CallTool(name string, args map[string]any) (string, error)
}

// AlertService is the interface for managing alerts from agent tools.
type AlertService interface {
	CreateAlert(alertType, symbol, condition string, threshold float64, expression, message string, oneShot bool) (int64, error)
	DeleteAlert(id int64) error
	ListAlerts() ([]map[string]any, error)
}

// ToolRegistry manages available tools and their execution.
type ToolRegistry struct {
	adapters   map[string]adapter.TradingAdapter
	riskEngine *risk.Engine
	mcpBridge  MCPBridge
	bus        *engine.EventBus
	db         *sql.DB
	alertSvc   AlertService
}

// NewToolRegistry creates a registry with access to exchange adapters and risk engine.
func NewToolRegistry(adapters map[string]adapter.TradingAdapter, riskEngine *risk.Engine, bus *engine.EventBus, db *sql.DB) *ToolRegistry {
	return &ToolRegistry{
		adapters:   adapters,
		riskEngine: riskEngine,
		bus:        bus,
		db:         db,
	}
}

// SetMCPBridge sets the MCP client manager for external tool access.
func (tr *ToolRegistry) SetMCPBridge(bridge MCPBridge) {
	tr.mcpBridge = bridge
}

// SetAlertService sets the alert service for alert management tools.
func (tr *ToolRegistry) SetAlertService(svc AlertService) {
	tr.alertSvc = svc
}

// Definitions returns all tool definitions for the LLM.
func (tr *ToolRegistry) Definitions() []ToolDef {
	defs := []ToolDef{
		{
			Name:        "get_price",
			Description: "Get the current real-time price, bid, ask, and 24h volume for a trading symbol from a connected exchange.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"symbol":   map[string]any{"type": "string", "description": "Trading pair, e.g. BTC/USDT, ETH/USDT"},
					"exchange": map[string]any{"type": "string", "description": "Exchange name (default: binance)", "default": "binance"},
				},
				"required": []string{"symbol"},
			},
		},
		{
			Name:        "get_candles",
			Description: "Get historical candlestick (OHLCV) data for technical analysis. Returns open, high, low, close, volume for each candle.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"symbol":    map[string]any{"type": "string", "description": "Trading pair, e.g. BTC/USDT"},
					"timeframe": map[string]any{"type": "string", "description": "Candle timeframe: 1m, 5m, 15m, 1h, 4h, 1d, 1w", "default": "1h"},
					"limit":     map[string]any{"type": "integer", "description": "Number of candles (default: 50, max: 200)", "default": 50},
					"exchange":  map[string]any{"type": "string", "description": "Exchange name (default: binance)", "default": "binance"},
				},
				"required": []string{"symbol"},
			},
		},
		{
			Name:        "analyze_market",
			Description: "Run technical analysis on a symbol. Returns RSI, SMA(20), SMA(50), EMA(12), EMA(26), MACD, Bollinger Bands, and a trend summary. Use this to make data-driven trading decisions.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"symbol":    map[string]any{"type": "string", "description": "Trading pair, e.g. BTC/USDT"},
					"timeframe": map[string]any{"type": "string", "description": "Candle timeframe for analysis", "default": "1h"},
					"exchange":  map[string]any{"type": "string", "description": "Exchange name", "default": "binance"},
				},
				"required": []string{"symbol"},
			},
		},
		{
			Name:        "get_balances",
			Description: "Get the user's account balances showing free, locked, and total amount for each asset on a connected exchange.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"exchange": map[string]any{"type": "string", "description": "Exchange name (default: binance)", "default": "binance"},
				},
			},
		},
		{
			Name:        "get_positions",
			Description: "Get all open positions showing symbol, side, size, entry price, current price, and unrealized P&L.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"exchange": map[string]any{"type": "string", "description": "Exchange name (default: binance)", "default": "binance"},
				},
			},
		},
		{
			Name:        "risk_check",
			Description: "Check if a proposed trade passes risk management rules. Returns whether the trade is allowed and any risk violations. ALWAYS call this before placing an order.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"symbol":    map[string]any{"type": "string", "description": "Trading pair"},
					"side":      map[string]any{"type": "string", "enum": []string{"buy", "sell"}, "description": "Trade direction"},
					"size":      map[string]any{"type": "number", "description": "Order quantity"},
					"price":     map[string]any{"type": "number", "description": "Entry price"},
					"stop_loss": map[string]any{"type": "number", "description": "Stop loss price (optional, for risk calculation)"},
					"exchange":  map[string]any{"type": "string", "default": "binance"},
				},
				"required": []string{"symbol", "side", "size", "price"},
			},
		},
		{
			Name:        "calculate_position_size",
			Description: "Calculate the optimal position size based on risk management rules, given entry price and stop loss.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"entry_price": map[string]any{"type": "number", "description": "Planned entry price"},
					"stop_loss":   map[string]any{"type": "number", "description": "Planned stop loss price"},
					"exchange":    map[string]any{"type": "string", "default": "binance"},
				},
				"required": []string{"entry_price", "stop_loss"},
			},
		},
		{
			Name:        "place_order",
			Description: "Place a real trade order on the exchange. IMPORTANT: Always run risk_check first. Supports market, limit, and stop orders.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"symbol":   map[string]any{"type": "string", "description": "Trading pair, e.g. BTC/USDT"},
					"side":     map[string]any{"type": "string", "enum": []string{"BUY", "SELL"}, "description": "Order side"},
					"type":     map[string]any{"type": "string", "enum": []string{"MARKET", "LIMIT", "STOP"}, "description": "Order type"},
					"size":     map[string]any{"type": "number", "description": "Order quantity"},
					"price":    map[string]any{"type": "number", "description": "Price for LIMIT/STOP orders"},
					"exchange": map[string]any{"type": "string", "default": "binance"},
				},
				"required": []string{"symbol", "side", "type", "size"},
			},
		},
		{
			Name:        "cancel_order",
			Description: "Cancel an open order by its ID.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"order_id": map[string]any{"type": "string", "description": "Order ID to cancel (format: SYMBOL:orderId)"},
					"exchange": map[string]any{"type": "string", "default": "binance"},
				},
				"required": []string{"order_id"},
			},
		},
		{
			Name:        "get_open_orders",
			Description: "Get all open/pending orders on the exchange.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"exchange": map[string]any{"type": "string", "default": "binance"},
				},
			},
		},
		{
			Name:        "backtest",
			Description: "Run a backtest on historical data to evaluate a trading strategy. Returns performance metrics including P&L, win rate, Sharpe ratio, and max drawdown.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"symbol":    map[string]any{"type": "string", "description": "Trading pair, e.g. BTC/USDT"},
					"timeframe": map[string]any{"type": "string", "description": "Candle timeframe: 1h, 4h, 1d", "default": "1d"},
					"days":      map[string]any{"type": "integer", "description": "Number of days to backtest (default: 90)", "default": 90},
					"strategy":  map[string]any{"type": "string", "description": "Strategy: momentum, meanrevert, macd_cross", "default": "momentum"},
					"capital":   map[string]any{"type": "number", "description": "Initial capital USD (default: 10000)", "default": 10000},
					"exchange":  map[string]any{"type": "string", "description": "Exchange for data", "default": "binance"},
				},
				"required": []string{"symbol"},
			},
		},
		{
			Name:        "create_alert",
			Description: "Create a new alert. Supports price alerts (above/below threshold), PnL alerts, and custom expression rules (e.g. 'rsi < 30 AND close > sma_50').",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"alert_type": map[string]any{"type": "string", "enum": []string{"price", "pnl", "risk", "trade", "system", "custom"}, "description": "Type of alert"},
					"symbol":     map[string]any{"type": "string", "description": "Trading pair (e.g. BTC/USDT). Required for price/custom alerts."},
					"condition":  map[string]any{"type": "string", "enum": []string{"above", "below", "expression"}, "description": "Alert condition", "default": "above"},
					"threshold":  map[string]any{"type": "number", "description": "Price or PnL threshold value"},
					"expression": map[string]any{"type": "string", "description": "Custom rule expression (e.g. 'rsi < 30 AND close > sma_50'). Only for custom type."},
					"message":    map[string]any{"type": "string", "description": "Custom alert message"},
					"one_shot":   map[string]any{"type": "boolean", "description": "Auto-disable after first trigger (default: false)", "default": false},
				},
				"required": []string{"alert_type"},
			},
		},
		{
			Name:        "list_alerts",
			Description: "List all active alerts with their status, conditions, and last trigger time.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "delete_alert",
			Description: "Delete an alert by its ID.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "integer", "description": "Alert ID to delete"},
				},
				"required": []string{"id"},
			},
		},
	}

	// Append MCP tools from external servers
	if tr.mcpBridge != nil {
		for _, t := range tr.mcpBridge.GetAllTools() {
			defs = append(defs, ToolDef{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: t.InputSchema,
			})
		}
	}

	return defs
}

// builtinTools is the set of tool names handled internally.
var builtinTools = map[string]bool{
	"get_price": true, "get_candles": true, "analyze_market": true,
	"get_balances": true, "get_positions": true, "risk_check": true,
	"calculate_position_size": true, "place_order": true,
	"cancel_order": true, "get_open_orders": true, "backtest": true,
	"create_alert": true, "list_alerts": true, "delete_alert": true,
}

// Execute runs a tool and returns the result.
func (tr *ToolRegistry) Execute(ctx context.Context, call ToolCall) ToolResult {
	switch call.Name {
	case "get_price":
		return tr.execGetPrice(ctx, call)
	case "get_candles":
		return tr.execGetCandles(ctx, call)
	case "analyze_market":
		return tr.execAnalyzeMarket(ctx, call)
	case "get_balances":
		return tr.execGetBalances(ctx, call)
	case "get_positions":
		return tr.execGetPositions(ctx, call)
	case "risk_check":
		return tr.execRiskCheck(ctx, call)
	case "calculate_position_size":
		return tr.execCalcPositionSize(ctx, call)
	case "place_order":
		return tr.execPlaceOrder(ctx, call)
	case "cancel_order":
		return tr.execCancelOrder(ctx, call)
	case "get_open_orders":
		return tr.execGetOpenOrders(ctx, call)
	case "backtest":
		return tr.execBacktest(ctx, call)
	case "create_alert":
		return tr.execCreateAlert(ctx, call)
	case "list_alerts":
		return tr.execListAlerts(ctx, call)
	case "delete_alert":
		return tr.execDeleteAlert(ctx, call)
	default:
		// Try MCP bridge for external tools
		if tr.mcpBridge != nil {
			result, err := tr.mcpBridge.CallTool(call.Name, call.Input)
			if err != nil {
				return ToolResult{ID: call.ID, Content: fmt.Sprintf("MCP tool error: %v", err), IsError: true}
			}
			return ToolResult{ID: call.ID, Content: result}
		}
		return ToolResult{ID: call.ID, Content: fmt.Sprintf("unknown tool: %s", call.Name), IsError: true}
	}
}

// ─── Tool Implementations ────────────────────────────────────────────

func (tr *ToolRegistry) getAdapter(input map[string]any) (adapter.TradingAdapter, error) {
	exchange := getString(input, "exchange", "binance")
	adp, ok := tr.adapters[exchange]
	if !ok {
		return nil, fmt.Errorf("exchange '%s' not configured", exchange)
	}
	return adp, nil
}

func (tr *ToolRegistry) execGetPrice(ctx context.Context, call ToolCall) ToolResult {
	adp, err := tr.getAdapter(call.Input)
	if err != nil {
		return ToolResult{ID: call.ID, Content: err.Error(), IsError: true}
	}
	symbol := getString(call.Input, "symbol", "")
	if symbol == "" {
		return ToolResult{ID: call.ID, Content: "symbol is required", IsError: true}
	}
	price, err := adp.GetPrice(ctx, symbol)
	if err != nil {
		return ToolResult{ID: call.ID, Content: fmt.Sprintf("failed to get price: %v", err), IsError: true}
	}
	data, _ := json.Marshal(price)
	return ToolResult{ID: call.ID, Content: string(data)}
}

func (tr *ToolRegistry) execGetCandles(ctx context.Context, call ToolCall) ToolResult {
	adp, err := tr.getAdapter(call.Input)
	if err != nil {
		return ToolResult{ID: call.ID, Content: err.Error(), IsError: true}
	}
	symbol := getString(call.Input, "symbol", "")
	timeframe := getString(call.Input, "timeframe", "1h")
	limit := getInt(call.Input, "limit", 50)
	if limit > 200 {
		limit = 200
	}
	candles, err := adp.GetCandles(ctx, symbol, timeframe, limit)
	if err != nil {
		return ToolResult{ID: call.ID, Content: fmt.Sprintf("failed to get candles: %v", err), IsError: true}
	}
	// Return compact summary to save tokens
	type compactCandle struct {
		T string  `json:"t"`
		O float64 `json:"o"`
		H float64 `json:"h"`
		L float64 `json:"l"`
		C float64 `json:"c"`
		V float64 `json:"v"`
	}
	compact := make([]compactCandle, len(candles))
	for i, c := range candles {
		compact[i] = compactCandle{
			T: c.Timestamp.Format("2006-01-02T15:04"),
			O: round(c.Open, 2),
			H: round(c.High, 2),
			L: round(c.Low, 2),
			C: round(c.Close, 2),
			V: round(c.Volume, 2),
		}
	}
	data, _ := json.Marshal(compact)
	return ToolResult{ID: call.ID, Content: string(data)}
}

func (tr *ToolRegistry) execAnalyzeMarket(ctx context.Context, call ToolCall) ToolResult {
	adp, err := tr.getAdapter(call.Input)
	if err != nil {
		return ToolResult{ID: call.ID, Content: err.Error(), IsError: true}
	}
	symbol := getString(call.Input, "symbol", "")
	timeframe := getString(call.Input, "timeframe", "1h")

	candles, err := adp.GetCandles(ctx, symbol, timeframe, 100)
	if err != nil {
		return ToolResult{ID: call.ID, Content: fmt.Sprintf("failed to get candles: %v", err), IsError: true}
	}
	if len(candles) < 26 {
		return ToolResult{ID: call.ID, Content: "insufficient data for analysis (need at least 26 candles)", IsError: true}
	}

	closes := make([]float64, len(candles))
	for i, c := range candles {
		closes[i] = c.Close
	}

	last := closes[len(closes)-1]
	sma20 := sma(closes, 20)
	sma50 := sma(closes, 50)
	ema12 := ema(closes, 12)
	ema26 := ema(closes, 26)
	rsiVal := rsi(closes, 14)
	macdLine := ema12 - ema26
	bbUpper, bbMiddle, bbLower := bollingerBands(closes, 20)

	// Determine trend
	var trend string
	switch {
	case last > sma20 && sma20 > sma50 && rsiVal > 50:
		trend = "BULLISH"
	case last < sma20 && sma20 < sma50 && rsiVal < 50:
		trend = "BEARISH"
	default:
		trend = "NEUTRAL"
	}

	// Signals
	var signals []string
	if rsiVal > 70 {
		signals = append(signals, "RSI overbought (>70) — potential reversal down")
	} else if rsiVal < 30 {
		signals = append(signals, "RSI oversold (<30) — potential reversal up")
	}
	if last > bbUpper {
		signals = append(signals, "Price above upper Bollinger Band — overbought")
	} else if last < bbLower {
		signals = append(signals, "Price below lower Bollinger Band — oversold")
	}
	if macdLine > 0 && ema12 > ema26 {
		signals = append(signals, "MACD bullish (above signal line)")
	} else if macdLine < 0 {
		signals = append(signals, "MACD bearish (below signal line)")
	}

	result := map[string]any{
		"symbol":    symbol,
		"timeframe": timeframe,
		"price":     round(last, 2),
		"trend":     trend,
		"indicators": map[string]any{
			"rsi":        round(rsiVal, 2),
			"sma_20":     round(sma20, 2),
			"sma_50":     round(sma50, 2),
			"ema_12":     round(ema12, 2),
			"ema_26":     round(ema26, 2),
			"macd":       round(macdLine, 2),
			"bb_upper":   round(bbUpper, 2),
			"bb_middle":  round(bbMiddle, 2),
			"bb_lower":   round(bbLower, 2),
		},
		"signals": signals,
	}
	data, _ := json.Marshal(result)
	return ToolResult{ID: call.ID, Content: string(data)}
}

func (tr *ToolRegistry) execGetBalances(ctx context.Context, call ToolCall) ToolResult {
	exchange := getString(call.Input, "exchange", "")
	if exchange != "" && exchange != "all" {
		// Single exchange mode
		adp, err := tr.getAdapter(call.Input)
		if err != nil {
			return ToolResult{ID: call.ID, Content: err.Error(), IsError: true}
		}
		balances, err := adp.GetBalances(ctx)
		if err != nil {
			return ToolResult{ID: call.ID, Content: fmt.Sprintf("failed to get balances: %v", err), IsError: true}
		}
		data, _ := json.Marshal(balances)
		return ToolResult{ID: call.ID, Content: string(data)}
	}

	// Multi-exchange aggregation
	var allBalances []map[string]any
	var grandTotal float64
	for name, adp := range tr.adapters {
		if !adp.IsConnected() {
			continue
		}
		balances, err := adp.GetBalances(ctx)
		if err != nil {
			continue
		}
		var exchTotal float64
		for _, b := range balances {
			exchTotal += b.Total
			allBalances = append(allBalances, map[string]any{
				"exchange": name,
				"asset":    b.Asset,
				"free":     b.Free,
				"locked":   b.Locked,
				"total":    b.Total,
			})
		}
		grandTotal += exchTotal
	}
	result := map[string]any{
		"balances":    allBalances,
		"grand_total": grandTotal,
	}
	data, _ := json.Marshal(result)
	return ToolResult{ID: call.ID, Content: string(data)}
}

func (tr *ToolRegistry) execGetPositions(ctx context.Context, call ToolCall) ToolResult {
	exchange := getString(call.Input, "exchange", "")
	if exchange != "" && exchange != "all" {
		// Single exchange mode
		adp, err := tr.getAdapter(call.Input)
		if err != nil {
			return ToolResult{ID: call.ID, Content: err.Error(), IsError: true}
		}
		positions, err := adp.GetPositions(ctx)
		if err != nil {
			return ToolResult{ID: call.ID, Content: fmt.Sprintf("failed to get positions: %v", err), IsError: true}
		}
		data, _ := json.Marshal(positions)
		return ToolResult{ID: call.ID, Content: string(data)}
	}

	// Multi-exchange aggregation
	var allPositions []map[string]any
	var totalPnL float64
	for name, adp := range tr.adapters {
		if !adp.IsConnected() {
			continue
		}
		positions, err := adp.GetPositions(ctx)
		if err != nil {
			continue
		}
		for _, p := range positions {
			totalPnL += p.PnL
			allPositions = append(allPositions, map[string]any{
				"exchange":      name,
				"symbol":        p.Symbol,
				"side":          p.Side,
				"size":          p.Size,
				"entry_price":   p.EntryPrice,
				"current_price": p.CurrentPrice,
				"pnl":           p.PnL,
			})
		}
	}
	result := map[string]any{
		"positions": allPositions,
		"total_pnl": totalPnL,
	}
	data, _ := json.Marshal(result)
	return ToolResult{ID: call.ID, Content: string(data)}
}

func (tr *ToolRegistry) execRiskCheck(ctx context.Context, call ToolCall) ToolResult {
	if tr.riskEngine == nil {
		return ToolResult{ID: call.ID, Content: `{"allowed":true,"reasons":["risk engine not configured"]}`, IsError: false}
	}

	symbol := getString(call.Input, "symbol", "")
	side := getString(call.Input, "side", "buy")
	size := getFloat(call.Input, "size", 0)
	price := getFloat(call.Input, "price", 0)
	stopLoss := getFloat(call.Input, "stop_loss", 0)

	// Get portfolio state
	portfolio := risk.PortfolioState{Balance: 10000} // default
	adp, err := tr.getAdapter(call.Input)
	if err == nil {
		if balances, err := adp.GetBalances(ctx); err == nil {
			for _, b := range balances {
				if b.Asset == "USDT" || b.Asset == "BUSD" || b.Asset == "USDC" {
					portfolio.Balance += b.Total
				}
			}
			if portfolio.Balance > 10000 {
				portfolio.Balance -= 10000 // remove default
			}
		}
		if positions, err := adp.GetPositions(ctx); err == nil {
			portfolio.OpenPositions = len(positions)
			for _, p := range positions {
				portfolio.TotalExposure += p.Size * p.CurrentPrice
			}
		}
	}

	proposal := risk.TradeProposal{
		Symbol:   symbol,
		Side:     side,
		Size:     size,
		Price:    price,
		StopLoss: stopLoss,
	}

	result := tr.riskEngine.Check(proposal, portfolio)
	data, _ := json.Marshal(result)
	return ToolResult{ID: call.ID, Content: string(data)}
}

func (tr *ToolRegistry) execCalcPositionSize(ctx context.Context, call ToolCall) ToolResult {
	if tr.riskEngine == nil {
		return ToolResult{ID: call.ID, Content: "risk engine not configured", IsError: true}
	}

	entry := getFloat(call.Input, "entry_price", 0)
	stop := getFloat(call.Input, "stop_loss", 0)

	// Get balance
	balance := 10000.0
	adp, err := tr.getAdapter(call.Input)
	if err == nil {
		if balances, err := adp.GetBalances(ctx); err == nil {
			for _, b := range balances {
				if b.Asset == "USDT" || b.Asset == "BUSD" || b.Asset == "USDC" {
					balance += b.Total
				}
			}
			if balance > 10000 {
				balance -= 10000
			}
		}
	}

	size := tr.riskEngine.CalculatePositionSize(balance, entry, stop)
	result := map[string]any{
		"recommended_size": round(size, 6),
		"entry_price":      entry,
		"stop_loss":        stop,
		"balance":          round(balance, 2),
		"risk_per_trade":   fmt.Sprintf("%.1f%%", tr.riskEngine.GetLimits().MaxRiskPerTradePct*100),
	}
	data, _ := json.Marshal(result)
	return ToolResult{ID: call.ID, Content: string(data)}
}

func (tr *ToolRegistry) execPlaceOrder(ctx context.Context, call ToolCall) ToolResult {
	adp, err := tr.getAdapter(call.Input)
	if err != nil {
		return ToolResult{ID: call.ID, Content: err.Error(), IsError: true}
	}

	order := adapter.Order{
		Symbol: getString(call.Input, "symbol", ""),
		Side:   adapter.Side(strings.ToUpper(getString(call.Input, "side", ""))),
		Type:   adapter.OrderType(strings.ToUpper(getString(call.Input, "type", "MARKET"))),
		Size:   getFloat(call.Input, "size", 0),
		Price:  getFloat(call.Input, "price", 0),
	}

	if order.Symbol == "" || order.Size <= 0 {
		return ToolResult{ID: call.ID, Content: "symbol and size are required", IsError: true}
	}

	result, err := adp.PlaceOrder(ctx, order)
	if err != nil {
		return ToolResult{ID: call.ID, Content: fmt.Sprintf("order failed: %v", err), IsError: true}
	}

	if tr.bus != nil {
		tr.bus.Publish(engine.Event{
			Type: "trade.executed",
			Data: map[string]any{
				"symbol":   result.Symbol,
				"side":     string(result.Side),
				"size":     result.Size,
				"price":    result.FilledAt,
				"type":     string(result.Type),
				"status":   string(result.Status),
				"order_id": result.ID,
				"exchange": result.Exchange,
			},
		})
	}

	data, _ := json.Marshal(result)
	return ToolResult{ID: call.ID, Content: string(data)}
}

func (tr *ToolRegistry) execCancelOrder(ctx context.Context, call ToolCall) ToolResult {
	adp, err := tr.getAdapter(call.Input)
	if err != nil {
		return ToolResult{ID: call.ID, Content: err.Error(), IsError: true}
	}
	orderID := getString(call.Input, "order_id", "")
	if orderID == "" {
		return ToolResult{ID: call.ID, Content: "order_id is required", IsError: true}
	}
	if err := adp.CancelOrder(ctx, orderID); err != nil {
		return ToolResult{ID: call.ID, Content: fmt.Sprintf("cancel failed: %v", err), IsError: true}
	}
	return ToolResult{ID: call.ID, Content: `{"status":"cancelled"}`}
}

func (tr *ToolRegistry) execGetOpenOrders(ctx context.Context, call ToolCall) ToolResult {
	adp, err := tr.getAdapter(call.Input)
	if err != nil {
		return ToolResult{ID: call.ID, Content: err.Error(), IsError: true}
	}
	orders, err := adp.GetOpenOrders(ctx)
	if err != nil {
		return ToolResult{ID: call.ID, Content: fmt.Sprintf("failed to get orders: %v", err), IsError: true}
	}
	data, _ := json.Marshal(orders)
	return ToolResult{ID: call.ID, Content: string(data)}
}

func (tr *ToolRegistry) execBacktest(ctx context.Context, call ToolCall) ToolResult {
	symbol := getString(call.Input, "symbol", "")
	if symbol == "" {
		return ToolResult{ID: call.ID, Content: "symbol is required", IsError: true}
	}

	timeframe := getString(call.Input, "timeframe", "1d")
	days := getInt(call.Input, "days", 90)
	strategyName := getString(call.Input, "strategy", "momentum")
	capital := getFloat(call.Input, "capital", 10000)
	exchange := getString(call.Input, "exchange", "binance")

	if tr.db == nil {
		return ToolResult{ID: call.ID, Content: "database not available for backtest", IsError: true}
	}

	// Get adapter for data
	adp, _ := tr.adapters[exchange]
	// adp can be nil — DataLoader will work in cache-only mode

	// Set up date range
	to := time.Now().UTC().Truncate(24 * time.Hour)
	from := to.AddDate(0, 0, -days)

	// Create DataLoader and load candles
	loader := backtest.NewDataLoader(tr.db, adp)
	candles, err := loader.LoadCandles(ctx, symbol, timeframe, from, to)
	if err != nil {
		return ToolResult{ID: call.ID, Content: fmt.Sprintf("failed to load candles: %v", err), IsError: true}
	}
	if len(candles) == 0 {
		return ToolResult{ID: call.ID, Content: fmt.Sprintf("no candle data available for %s", symbol), IsError: true}
	}

	// Build strategy
	var strat backtest.StrategyRunner
	if strings.Contains(strategyName, "<") || strings.Contains(strategyName, ">") {
		// Treat as custom ConfigStrategy rule expression
		strat = &backtest.ConfigStrategy{BuyWhen: strategyName, SellWhen: ""}
	} else {
		strat = backtest.GetBuiltinStrategy(strategyName)
	}

	// Determine trade size (10% of capital)
	tradeSize := 0.0
	if candles[0].Close > 0 {
		tradeSize = (capital * 0.10) / candles[0].Close
	}

	// Run backtest
	eng := &backtest.Engine{Bus: tr.bus}
	btCfg := backtest.BacktestConfig{
		Symbol:    symbol,
		Timeframe: timeframe,
		From:      from,
		To:        to,
		Capital:   capital,
		MakerFee:  0.001,
		TakerFee:  0.001,
		Slippage:  0.0005,
		TradeSize: tradeSize,
	}

	result, err := eng.Run(ctx, btCfg, candles, strat)
	if err != nil {
		return ToolResult{ID: call.ID, Content: fmt.Sprintf("backtest failed: %v", err), IsError: true}
	}

	// Format readable text summary
	pnlSign := "+"
	if result.TotalPnL < 0 {
		pnlSign = ""
	}
	returnSign := "+"
	if result.TotalReturn < 0 {
		returnSign = ""
	}

	summary := fmt.Sprintf(`Backtest Results: %s (%s)
Strategy: %s | Capital: $%.2f

Final Equity: $%.2f | P&L: %s$%.2f (%s%.2f%%)
Trades: %d (W:%d L:%d) | Win Rate: %.1f%%
Avg Win: $%.2f | Avg Loss: $%.2f
Profit Factor: %.2f | Max Drawdown: %.2f%%
Sharpe Ratio: %.2f`,
		result.Symbol, result.Period,
		strategyName, result.Capital,
		result.FinalEquity, pnlSign, result.TotalPnL, returnSign, result.TotalReturn,
		result.TotalTrades, result.WinCount, result.LossCount, result.WinRate*100,
		result.AvgWin, result.AvgLoss,
		result.ProfitFactor, result.MaxDrawdown*100,
		result.SharpeRatio,
	)

	// Append recent trades
	if len(result.Trades) > 0 {
		summary += "\n\nRecent Trades:"
		start := 0
		if len(result.Trades) > 5 {
			start = len(result.Trades) - 5
		}
		for _, t := range result.Trades[start:] {
			tPnl := fmt.Sprintf("+$%.2f", t.PnL)
			if t.PnL < 0 {
				tPnl = fmt.Sprintf("-$%.2f", -t.PnL)
			}
			summary += fmt.Sprintf("\n  %s %s %.4f @ %.2f -> %.2f %s",
				t.ClosedAt.Format("01-02"), strings.ToUpper(t.Side), t.Size, t.EntryPrice, t.ExitPrice, tPnl)
		}
	}

	// Append JSON for structured consumption
	jsonResult := map[string]any{
		"symbol":        result.Symbol,
		"timeframe":     result.Timeframe,
		"period":        result.Period,
		"capital":       result.Capital,
		"final_equity":  round(result.FinalEquity, 2),
		"total_pnl":     round(result.TotalPnL, 2),
		"total_return":  round(result.TotalReturn, 2),
		"total_trades":  result.TotalTrades,
		"win_count":     result.WinCount,
		"loss_count":    result.LossCount,
		"win_rate":      round(result.WinRate*100, 1),
		"avg_win":       round(result.AvgWin, 2),
		"avg_loss":      round(result.AvgLoss, 2),
		"profit_factor": round(result.ProfitFactor, 2),
		"max_drawdown":  round(result.MaxDrawdown*100, 2),
		"sharpe_ratio":  round(result.SharpeRatio, 2),
	}
	jsonBytes, _ := json.Marshal(jsonResult)
	summary += "\n\n" + string(jsonBytes)

	return ToolResult{ID: call.ID, Content: summary}
}

// ─── Alert Tool Implementations ──────────────────────────────────────

func (tr *ToolRegistry) execCreateAlert(ctx context.Context, call ToolCall) ToolResult {
	if tr.alertSvc == nil {
		return ToolResult{ID: call.ID, Content: "alert service not configured", IsError: true}
	}

	alertType := getString(call.Input, "alert_type", "price")
	symbol := getString(call.Input, "symbol", "")
	condition := getString(call.Input, "condition", "above")
	threshold := getFloat(call.Input, "threshold", 0)
	expression := getString(call.Input, "expression", "")
	message := getString(call.Input, "message", "")
	oneShot := false
	if v, ok := call.Input["one_shot"].(bool); ok {
		oneShot = v
	}

	if alertType == "custom" {
		condition = "expression"
	}

	id, err := tr.alertSvc.CreateAlert(alertType, symbol, condition, threshold, expression, message, oneShot)
	if err != nil {
		return ToolResult{ID: call.ID, Content: fmt.Sprintf("failed to create alert: %v", err), IsError: true}
	}

	result := map[string]any{
		"id":      id,
		"type":    alertType,
		"symbol":  symbol,
		"status":  "created",
		"message": fmt.Sprintf("Alert #%d created successfully", id),
	}
	data, _ := json.Marshal(result)
	return ToolResult{ID: call.ID, Content: string(data)}
}

func (tr *ToolRegistry) execListAlerts(ctx context.Context, call ToolCall) ToolResult {
	if tr.alertSvc == nil {
		return ToolResult{ID: call.ID, Content: "alert service not configured", IsError: true}
	}

	alerts, err := tr.alertSvc.ListAlerts()
	if err != nil {
		return ToolResult{ID: call.ID, Content: fmt.Sprintf("failed to list alerts: %v", err), IsError: true}
	}

	if len(alerts) == 0 {
		return ToolResult{ID: call.ID, Content: "No active alerts."}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Active Alerts (%d):\n", len(alerts)))
	for _, a := range alerts {
		sb.WriteString(fmt.Sprintf("  #%v [%v] %v %v %v",
			a["id"], a["type"], a["symbol"], a["condition"], a["threshold"]))
		if expr, ok := a["expression"].(string); ok && expr != "" {
			sb.WriteString(fmt.Sprintf(" expr=%s", expr))
		}
		if msg, ok := a["message"].(string); ok && msg != "" {
			sb.WriteString(fmt.Sprintf(" — %s", msg))
		}
		sb.WriteString("\n")
	}
	data, _ := json.Marshal(alerts)
	sb.WriteString("\n")
	sb.Write(data)

	return ToolResult{ID: call.ID, Content: sb.String()}
}

func (tr *ToolRegistry) execDeleteAlert(ctx context.Context, call ToolCall) ToolResult {
	if tr.alertSvc == nil {
		return ToolResult{ID: call.ID, Content: "alert service not configured", IsError: true}
	}

	id := int64(getInt(call.Input, "id", 0))
	if id == 0 {
		return ToolResult{ID: call.ID, Content: "alert ID is required", IsError: true}
	}

	if err := tr.alertSvc.DeleteAlert(id); err != nil {
		return ToolResult{ID: call.ID, Content: fmt.Sprintf("failed to delete alert: %v", err), IsError: true}
	}

	return ToolResult{ID: call.ID, Content: fmt.Sprintf(`{"status":"deleted","id":%d}`, id)}
}

// ─── Technical Analysis Helpers ──────────────────────────────────────

func sma(data []float64, period int) float64 {
	n := len(data)
	if n < period {
		return 0
	}
	sum := 0.0
	for i := n - period; i < n; i++ {
		sum += data[i]
	}
	return sum / float64(period)
}

func ema(data []float64, period int) float64 {
	if len(data) < period {
		return 0
	}
	k := 2.0 / float64(period+1)
	// Start with SMA
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += data[i]
	}
	val := sum / float64(period)
	for i := period; i < len(data); i++ {
		val = data[i]*k + val*(1-k)
	}
	return val
}

func rsi(data []float64, period int) float64 {
	if len(data) < period+1 {
		return 50
	}
	gains, losses := 0.0, 0.0
	for i := len(data) - period; i < len(data); i++ {
		diff := data[i] - data[i-1]
		if diff > 0 {
			gains += diff
		} else {
			losses -= diff
		}
	}
	if losses == 0 {
		return 100
	}
	rs := (gains / float64(period)) / (losses / float64(period))
	return 100 - 100/(1+rs)
}

func bollingerBands(data []float64, period int) (upper, middle, lower float64) {
	middle = sma(data, period)
	if len(data) < period {
		return middle, middle, middle
	}
	n := len(data)
	sum := 0.0
	for i := n - period; i < n; i++ {
		diff := data[i] - middle
		sum += diff * diff
	}
	stddev := math.Sqrt(sum / float64(period))
	return middle + 2*stddev, middle, middle - 2*stddev
}

// ─── Helpers ─────────────────────────────────────────────────────────

func getString(m map[string]any, key, def string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return def
}

func getFloat(m map[string]any, key string, def float64) float64 {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case int:
			return float64(n)
		case string:
			if f, err := strconv.ParseFloat(n, 64); err == nil {
				return f
			}
		}
	}
	return def
}

func getInt(m map[string]any, key string, def int) int {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case string:
			if i, err := strconv.Atoi(n); err == nil {
				return i
			}
		}
	}
	return def
}

func round(v float64, decimals int) float64 {
	pow := math.Pow(10, float64(decimals))
	return math.Round(v*pow) / pow
}
