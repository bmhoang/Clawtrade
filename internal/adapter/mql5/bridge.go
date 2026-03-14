// internal/adapter/mql5/bridge.go
package mql5

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/clawtrade/clawtrade/internal/adapter"
)

// ErrNotConnected is returned when MT5 bridge process is not running.
var ErrNotConnected = fmt.Errorf("metatrader not connected")

// Bridge manages a Python subprocess that communicates with MT5 via its Python API.
// Commands are sent as JSON via stdin; responses come back as JSON via stdout.
type Bridge struct {
	mu        sync.Mutex
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Scanner
	connected bool
	terminal  string // path to MT5 terminal (optional)
	login     string
	password  string
	server    string
	pending   map[string]chan json.RawMessage
	nextID    atomic.Int64
	done      chan struct{}
}

// Response from the Python bridge.
type Response struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// Command sent to the Python bridge.
type Command struct {
	ID     string      `json:"id"`
	Cmd    string      `json:"cmd"`
	Params interface{} `json:"params"`
}

// NewBridge creates a new MT5 bridge.
func NewBridge(terminal string) *Bridge {
	return &Bridge{
		terminal: terminal,
		pending:  make(map[string]chan json.RawMessage),
		done:     make(chan struct{}),
	}
}

// SetCredentials sets login credentials for MT5.
func (b *Bridge) SetCredentials(login, password, server string) {
	b.login = login
	b.password = password
	b.server = server
}

// findPython returns the python executable name.
func findPython() string {
	if runtime.GOOS == "windows" {
		// Try python first (Windows), then python3
		if _, err := exec.LookPath("python"); err == nil {
			return "python"
		}
	}
	if _, err := exec.LookPath("python3"); err == nil {
		return "python3"
	}
	return "python"
}

// findBridgeScript locates the mt5_bridge.py script.
func findBridgeScript() string {
	// Try relative to executable first, then common locations
	candidates := []string{
		"scripts/mt5_bridge.py",
		filepath.Join("..", "scripts", "mt5_bridge.py"),
		filepath.Join(".", "scripts", "mt5_bridge.py"),
	}
	for _, c := range candidates {
		if abs, err := filepath.Abs(c); err == nil {
			return abs
		}
	}
	return "scripts/mt5_bridge.py"
}

// Start launches the Python MT5 bridge subprocess and initializes MT5.
func (b *Bridge) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.connected {
		return nil
	}

	python := findPython()
	script := findBridgeScript()

	args := []string{script}
	if b.terminal != "" {
		args = append(args, "--terminal", b.terminal)
	}

	b.cmd = exec.Command(python, args...)

	var err error
	b.stdin, err = b.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("mt5 bridge stdin: %w", err)
	}

	stdoutPipe, err := b.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("mt5 bridge stdout: %w", err)
	}
	b.stdout = bufio.NewScanner(stdoutPipe)

	if err := b.cmd.Start(); err != nil {
		return fmt.Errorf("mt5 bridge start: %w", err)
	}

	// Start reading responses
	go b.readLoop()

	// If terminal was provided, auto-init response should come back
	if b.terminal != "" {
		// Wait briefly for auto-init
		time.Sleep(500 * time.Millisecond)
	}

	// Send init command with credentials
	params := map[string]string{}
	if b.terminal != "" {
		params["terminal"] = b.terminal
	}
	if b.login != "" {
		params["login"] = b.login
	}
	if b.password != "" {
		params["password"] = b.password
	}
	if b.server != "" {
		params["server"] = b.server
	}

	result, err := b.call("init", params)
	if err != nil {
		b.stop()
		return fmt.Errorf("mt5 init: %w", err)
	}

	var initResult struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(result, &initResult); err != nil {
		b.stop()
		return fmt.Errorf("mt5 init parse: %w", err)
	}
	if !initResult.OK {
		b.stop()
		return fmt.Errorf("mt5 init: %s", initResult.Error)
	}

	b.connected = true
	return nil
}

// readLoop reads JSON responses from the Python process stdout.
func (b *Bridge) readLoop() {
	for b.stdout.Scan() {
		line := b.stdout.Bytes()
		if len(line) == 0 {
			continue
		}

		var resp Response
		if err := json.Unmarshal(line, &resp); err != nil {
			continue
		}

		b.mu.Lock()
		ch, ok := b.pending[resp.ID]
		if ok {
			delete(b.pending, resp.ID)
		}
		b.mu.Unlock()

		if ok && ch != nil {
			if resp.Error != "" {
				// Encode error as JSON so caller can detect it
				errJSON, _ := json.Marshal(map[string]string{"error": resp.Error})
				ch <- json.RawMessage(errJSON)
			} else {
				ch <- resp.Result
			}
		}
	}

	// Process ended
	b.mu.Lock()
	b.connected = false
	// Cancel all pending requests
	for id, ch := range b.pending {
		errJSON, _ := json.Marshal(map[string]string{"error": "bridge process exited"})
		ch <- json.RawMessage(errJSON)
		delete(b.pending, id)
	}
	b.mu.Unlock()
}

// call sends a command and waits for its response.
func (b *Bridge) call(cmd string, params interface{}) (json.RawMessage, error) {
	id := fmt.Sprintf("%d", b.nextID.Add(1))
	ch := make(chan json.RawMessage, 1)

	b.mu.Lock()
	b.pending[id] = ch
	b.mu.Unlock()

	command := Command{ID: id, Cmd: cmd, Params: params}
	data, err := json.Marshal(command)
	if err != nil {
		return nil, fmt.Errorf("mt5 marshal: %w", err)
	}
	data = append(data, '\n')

	if _, err := b.stdin.Write(data); err != nil {
		b.mu.Lock()
		delete(b.pending, id)
		b.mu.Unlock()
		return nil, fmt.Errorf("mt5 write: %w", err)
	}

	// Wait with timeout
	select {
	case result := <-ch:
		// Check if result is an error
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(result, &errResp) == nil && errResp.Error != "" {
			return nil, fmt.Errorf("%s", errResp.Error)
		}
		return result, nil
	case <-time.After(30 * time.Second):
		b.mu.Lock()
		delete(b.pending, id)
		b.mu.Unlock()
		return nil, fmt.Errorf("mt5 timeout: %s", cmd)
	}
}

// stop kills the Python process.
func (b *Bridge) stop() {
	if b.stdin != nil {
		b.stdin.Close()
	}
	if b.cmd != nil && b.cmd.Process != nil {
		b.cmd.Process.Kill()
		b.cmd.Wait()
	}
	b.connected = false
}

// Stop shuts down the bridge.
func (b *Bridge) Stop() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.connected {
		return nil
	}

	// Try graceful shutdown
	b.call("shutdown", map[string]string{})
	time.Sleep(200 * time.Millisecond)

	b.stop()
	return nil
}

// IsConnected returns whether the bridge is running and MT5 is connected.
func (b *Bridge) IsConnected() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.connected
}

// Call exposes the raw call method for custom commands.
func (b *Bridge) Call(cmd string, params interface{}) (json.RawMessage, error) {
	b.mu.Lock()
	connected := b.connected
	b.mu.Unlock()

	if !connected {
		return nil, ErrNotConnected
	}
	return b.call(cmd, params)
}

// ─── MQL5Adapter implements adapter.TradingAdapter ───

// MQL5Adapter wraps Bridge to implement the TradingAdapter interface.
type MQL5Adapter struct {
	bridge *Bridge
}

// Compile-time check.
var _ adapter.TradingAdapter = (*MQL5Adapter)(nil)

// NewMQL5Adapter creates a new adapter backed by the Python MT5 bridge.
func NewMQL5Adapter(terminal string) *MQL5Adapter {
	return &MQL5Adapter{bridge: NewBridge(terminal)}
}

func (a *MQL5Adapter) Name() string { return "mt5" }

func (a *MQL5Adapter) Capabilities() adapter.AdapterCaps {
	return adapter.AdapterCaps{
		Name:      "mt5",
		WebSocket: false,
		Margin:    true,
		Futures:   false,
		OrderTypes: []adapter.OrderType{
			adapter.OrderTypeMarket,
			adapter.OrderTypeLimit,
			adapter.OrderTypeStop,
		},
	}
}

func (a *MQL5Adapter) Connect(ctx context.Context) error {
	return a.bridge.Start()
}

func (a *MQL5Adapter) Disconnect() error {
	return a.bridge.Stop()
}

func (a *MQL5Adapter) IsConnected() bool {
	return a.bridge.IsConnected()
}

func (a *MQL5Adapter) GetPrice(ctx context.Context, symbol string) (*adapter.Price, error) {
	result, err := a.bridge.Call("get_price", map[string]string{"symbol": symbol})
	if err != nil {
		return nil, err
	}

	var pd struct {
		Symbol string  `json:"symbol"`
		Bid    float64 `json:"bid"`
		Ask    float64 `json:"ask"`
		Last   float64 `json:"last"`
		Volume float64 `json:"volume"`
		Time   int64   `json:"time"`
	}
	if err := json.Unmarshal(result, &pd); err != nil {
		return nil, fmt.Errorf("mt5 price parse: %w", err)
	}

	return &adapter.Price{
		Symbol:    pd.Symbol,
		Bid:       pd.Bid,
		Ask:       pd.Ask,
		Last:      pd.Last,
		Volume24h: pd.Volume,
		Timestamp: time.Unix(pd.Time, 0),
	}, nil
}

func (a *MQL5Adapter) GetCandles(ctx context.Context, symbol, timeframe string, limit int) ([]adapter.Candle, error) {
	result, err := a.bridge.Call("get_candles", map[string]interface{}{
		"symbol":    symbol,
		"timeframe": timeframe,
		"limit":     limit,
	})
	if err != nil {
		return nil, err
	}

	var resp struct {
		Candles []struct {
			Time   int64   `json:"time"`
			Open   float64 `json:"open"`
			High   float64 `json:"high"`
			Low    float64 `json:"low"`
			Close  float64 `json:"close"`
			Volume float64 `json:"volume"`
		} `json:"candles"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("mt5 candles parse: %w", err)
	}

	candles := make([]adapter.Candle, len(resp.Candles))
	for i, c := range resp.Candles {
		candles[i] = adapter.Candle{
			Timestamp: time.Unix(c.Time, 0),
			Open:      c.Open,
			High:      c.High,
			Low:       c.Low,
			Close:     c.Close,
			Volume:    c.Volume,
		}
	}
	return candles, nil
}

func (a *MQL5Adapter) GetOrderBook(ctx context.Context, symbol string, depth int) (*adapter.OrderBook, error) {
	return nil, fmt.Errorf("mt5: order book not supported")
}

func (a *MQL5Adapter) PlaceOrder(ctx context.Context, order adapter.Order) (*adapter.Order, error) {
	side := "buy"
	if order.Side == adapter.SideSell {
		side = "sell"
	}
	orderType := "market"
	if order.Type == adapter.OrderTypeLimit {
		orderType = "limit"
	} else if order.Type == adapter.OrderTypeStop {
		orderType = "stop"
	}

	result, err := a.bridge.Call("place_order", map[string]interface{}{
		"symbol":  order.Symbol,
		"side":    side,
		"size":    order.Size,
		"type":    orderType,
		"price":   order.Price,
		"comment": "clawtrade",
	})
	if err != nil {
		return nil, err
	}

	var resp struct {
		OK      bool    `json:"ok"`
		OrderID string  `json:"order_id"`
		Price   float64 `json:"price"`
		Volume  float64 `json:"volume"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("mt5 order parse: %w", err)
	}

	order.ID = resp.OrderID
	order.Status = adapter.OrderStatusFilled
	order.FilledAt = resp.Price
	order.Exchange = "mt5"
	return &order, nil
}

func (a *MQL5Adapter) CancelOrder(ctx context.Context, orderID string) error {
	_, err := a.bridge.Call("cancel_order", map[string]string{"order_id": orderID})
	return err
}

func (a *MQL5Adapter) GetOpenOrders(ctx context.Context) ([]adapter.Order, error) {
	result, err := a.bridge.Call("get_open_orders", map[string]string{})
	if err != nil {
		return nil, err
	}

	var resp struct {
		Orders []struct {
			ID        string  `json:"id"`
			Symbol    string  `json:"symbol"`
			Side      string  `json:"side"`
			Type      string  `json:"type"`
			Price     float64 `json:"price"`
			Size      float64 `json:"size"`
			CreatedAt int64   `json:"created_at"`
		} `json:"orders"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("mt5 orders parse: %w", err)
	}

	orders := make([]adapter.Order, len(resp.Orders))
	for i, o := range resp.Orders {
		side := adapter.SideBuy
		if o.Side == "sell" {
			side = adapter.SideSell
		}
		orderType := adapter.OrderTypeMarket
		if o.Type == "limit" {
			orderType = adapter.OrderTypeLimit
		} else if o.Type == "stop" {
			orderType = adapter.OrderTypeStop
		}
		orders[i] = adapter.Order{
			ID:        o.ID,
			Symbol:    o.Symbol,
			Side:      side,
			Type:      orderType,
			Price:     o.Price,
			Size:      o.Size,
			Status:    adapter.OrderStatusPending,
			Exchange:  "mt5",
			CreatedAt: time.Unix(o.CreatedAt, 0),
		}
	}
	return orders, nil
}

func (a *MQL5Adapter) GetBalances(ctx context.Context) ([]adapter.Balance, error) {
	result, err := a.bridge.Call("get_balances", map[string]string{})
	if err != nil {
		return nil, err
	}

	var resp struct {
		Balances []struct {
			Asset  string  `json:"asset"`
			Free   float64 `json:"free"`
			Locked float64 `json:"locked"`
			Total  float64 `json:"total"`
		} `json:"balances"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("mt5 balances parse: %w", err)
	}

	balances := make([]adapter.Balance, len(resp.Balances))
	for i, b := range resp.Balances {
		balances[i] = adapter.Balance{
			Asset:  b.Asset,
			Free:   b.Free,
			Locked: b.Locked,
			Total:  b.Total,
		}
	}
	return balances, nil
}

func (a *MQL5Adapter) GetPositions(ctx context.Context) ([]adapter.Position, error) {
	result, err := a.bridge.Call("get_positions", map[string]string{})
	if err != nil {
		return nil, err
	}

	var resp struct {
		Positions []struct {
			Symbol       string  `json:"symbol"`
			Side         string  `json:"side"`
			Size         float64 `json:"size"`
			EntryPrice   float64 `json:"entry_price"`
			CurrentPrice float64 `json:"current_price"`
			PnL          float64 `json:"pnl"`
			OpenedAt     int64   `json:"opened_at"`
		} `json:"positions"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("mt5 positions parse: %w", err)
	}

	positions := make([]adapter.Position, len(resp.Positions))
	for i, p := range resp.Positions {
		side := adapter.SideBuy
		if p.Side == "sell" {
			side = adapter.SideSell
		}
		positions[i] = adapter.Position{
			Symbol:       p.Symbol,
			Side:         side,
			Size:         p.Size,
			EntryPrice:   p.EntryPrice,
			CurrentPrice: p.CurrentPrice,
			PnL:          p.PnL,
			Exchange:     "mt5",
			OpenedAt:     time.Unix(p.OpenedAt, 0),
		}
	}
	return positions, nil
}

// Bridge returns the underlying bridge for direct access.
func (a *MQL5Adapter) Bridge() *Bridge {
	return a.bridge
}
