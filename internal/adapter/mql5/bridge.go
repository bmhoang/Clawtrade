// internal/adapter/mql5/bridge.go
package mql5

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/clawtrade/clawtrade/internal/adapter"
)

// ErrNotImplemented is returned for methods not supported by the MQL5 bridge.
var ErrNotImplemented = fmt.Errorf("not implemented")

// ErrNotConnected is returned when attempting to send without a connection.
var ErrNotConnected = fmt.Errorf("metatrader not connected")

// Message types for MQL5 communication.
const (
	MsgTypePrice     = "price"
	MsgTypeOrder     = "order"
	MsgTypePosition  = "position"
	MsgTypeAccount   = "account"
	MsgTypeHeartbeat = "heartbeat"
	MsgTypeCommand   = "command"
	MsgTypeResponse  = "response"
)

// Message is the wire format for TCP communication.
type Message struct {
	Type      string          `json:"type"`
	ID        string          `json:"id,omitempty"`
	Symbol    string          `json:"symbol,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	Timestamp int64           `json:"timestamp"`
}

// PriceData from MetaTrader.
type PriceData struct {
	Symbol string  `json:"symbol"`
	Bid    float64 `json:"bid"`
	Ask    float64 `json:"ask"`
	Last   float64 `json:"last"`
	Volume float64 `json:"volume"`
	Time   int64   `json:"time"`
}

// OrderCommand sent to MetaTrader.
type OrderCommand struct {
	Action     string  `json:"action"`
	Symbol     string  `json:"symbol"`
	Volume     float64 `json:"volume"`
	Price      float64 `json:"price,omitempty"`
	StopLoss   float64 `json:"stop_loss,omitempty"`
	TakeProfit float64 `json:"take_profit,omitempty"`
	Comment    string  `json:"comment,omitempty"`
}

// MessageHandler handles a received message.
type MessageHandler func(msg Message) error

// Bridge manages TCP connection to MetaTrader EA.
type Bridge struct {
	mu           sync.RWMutex
	address      string
	listener     net.Listener
	conn         net.Conn
	connected    bool
	prices       map[string]PriceData
	handlers     map[string]MessageHandler
	onConnect    func()
	onDisconnect func()
	done         chan struct{}
}

// NewBridge creates a new MQL5 bridge that listens on the given address.
func NewBridge(address string) *Bridge {
	return &Bridge{
		address:  address,
		prices:   make(map[string]PriceData),
		handlers: make(map[string]MessageHandler),
		done:     make(chan struct{}),
	}
}

// Start begins listening for MetaTrader connections.
func (b *Bridge) Start() error {
	ln, err := net.Listen("tcp", b.address)
	if err != nil {
		return fmt.Errorf("mql5 bridge listen: %w", err)
	}

	b.mu.Lock()
	b.listener = ln
	b.mu.Unlock()

	go b.acceptLoop(ln)

	return nil
}

// acceptLoop accepts incoming connections from MetaTrader.
func (b *Bridge) acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-b.done:
				return
			default:
				return
			}
		}
		b.handleConnection(conn)
	}
}

// Stop closes the bridge and any active connection.
func (b *Bridge) Stop() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	select {
	case <-b.done:
		// already closed
	default:
		close(b.done)
	}

	if b.conn != nil {
		b.conn.Close()
		b.conn = nil
	}
	b.connected = false

	if b.listener != nil {
		err := b.listener.Close()
		b.listener = nil
		return err
	}
	return nil
}

// IsConnected returns whether MetaTrader is connected.
func (b *Bridge) IsConnected() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.connected
}

// Address returns the listener's actual address (useful when port is 0).
func (b *Bridge) Address() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.listener != nil {
		return b.listener.Addr().String()
	}
	return b.address
}

// Send sends a message to MetaTrader.
func (b *Bridge) Send(msg Message) error {
	b.mu.RLock()
	conn := b.conn
	connected := b.connected
	b.mu.RUnlock()

	if !connected || conn == nil {
		return ErrNotConnected
	}

	msg.Timestamp = time.Now().UnixMilli()
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("mql5 marshal: %w", err)
	}

	data = append(data, '\n')
	_, err = conn.Write(data)
	return err
}

// SendOrder sends a trade order to MetaTrader.
func (b *Bridge) SendOrder(cmd OrderCommand) error {
	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("mql5 marshal order: %w", err)
	}

	msg := Message{
		Type:   MsgTypeCommand,
		Symbol: cmd.Symbol,
		Data:   json.RawMessage(data),
	}
	return b.Send(msg)
}

// GetLatestPrice returns the latest price for a symbol.
func (b *Bridge) GetLatestPrice(symbol string) (*PriceData, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	p, ok := b.prices[symbol]
	if !ok {
		return nil, false
	}
	return &p, true
}

// OnMessage registers a handler for a message type.
func (b *Bridge) OnMessage(msgType string, handler MessageHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[msgType] = handler
}

// OnConnect sets the callback for when MetaTrader connects.
func (b *Bridge) OnConnect(fn func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.onConnect = fn
}

// OnDisconnect sets the callback for when MetaTrader disconnects.
func (b *Bridge) OnDisconnect(fn func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.onDisconnect = fn
}

// handleConnection processes incoming messages from MetaTrader.
func (b *Bridge) handleConnection(conn net.Conn) {
	b.mu.Lock()
	// Close any existing connection.
	if b.conn != nil {
		b.conn.Close()
	}
	b.conn = conn
	b.connected = true
	onConnect := b.onConnect
	b.mu.Unlock()

	if onConnect != nil {
		onConnect()
	}

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var msg Message
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		// Update price cache for price messages.
		if msg.Type == MsgTypePrice && msg.Data != nil {
			var pd PriceData
			if err := json.Unmarshal(msg.Data, &pd); err == nil {
				b.mu.Lock()
				b.prices[pd.Symbol] = pd
				b.mu.Unlock()
			}
		}

		// Route to registered handler.
		b.mu.RLock()
		handler, ok := b.handlers[msg.Type]
		b.mu.RUnlock()

		if ok && handler != nil {
			_ = handler(msg)
		}
	}

	b.mu.Lock()
	b.connected = false
	b.conn = nil
	onDisconnect := b.onDisconnect
	b.mu.Unlock()

	if onDisconnect != nil {
		onDisconnect()
	}
}

// MQL5Adapter wraps Bridge to implement the TradingAdapter interface.
type MQL5Adapter struct {
	bridge *Bridge
}

// Compile-time check that MQL5Adapter implements TradingAdapter.
var _ adapter.TradingAdapter = (*MQL5Adapter)(nil)

// NewMQL5Adapter creates a new adapter wrapping the bridge.
func NewMQL5Adapter(address string) *MQL5Adapter {
	return &MQL5Adapter{
		bridge: NewBridge(address),
	}
}

func (a *MQL5Adapter) Name() string {
	return "mql5"
}

func (a *MQL5Adapter) Capabilities() adapter.AdapterCaps {
	return adapter.AdapterCaps{
		Name:      "mql5",
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
	pd, ok := a.bridge.GetLatestPrice(symbol)
	if !ok {
		return nil, fmt.Errorf("no price data for %s", symbol)
	}
	return &adapter.Price{
		Symbol:    pd.Symbol,
		Bid:       pd.Bid,
		Ask:       pd.Ask,
		Last:      pd.Last,
		Volume24h: pd.Volume,
		Timestamp: time.UnixMilli(pd.Time),
	}, nil
}

func (a *MQL5Adapter) GetCandles(ctx context.Context, symbol, timeframe string, limit int) ([]adapter.Candle, error) {
	return nil, ErrNotImplemented
}

func (a *MQL5Adapter) GetOrderBook(ctx context.Context, symbol string, depth int) (*adapter.OrderBook, error) {
	return nil, ErrNotImplemented
}

func (a *MQL5Adapter) PlaceOrder(ctx context.Context, order adapter.Order) (*adapter.Order, error) {
	action := "buy"
	if order.Side == adapter.SideSell {
		action = "sell"
	}
	cmd := OrderCommand{
		Action: action,
		Symbol: order.Symbol,
		Volume: order.Size,
		Price:  order.Price,
	}
	if err := a.bridge.SendOrder(cmd); err != nil {
		return nil, err
	}
	order.Status = adapter.OrderStatusPending
	return &order, nil
}

func (a *MQL5Adapter) CancelOrder(ctx context.Context, orderID string) error {
	return ErrNotImplemented
}

func (a *MQL5Adapter) GetOpenOrders(ctx context.Context) ([]adapter.Order, error) {
	return nil, ErrNotImplemented
}

func (a *MQL5Adapter) GetBalances(ctx context.Context) ([]adapter.Balance, error) {
	return nil, ErrNotImplemented
}

func (a *MQL5Adapter) GetPositions(ctx context.Context) ([]adapter.Position, error) {
	return nil, ErrNotImplemented
}

// Bridge returns the underlying bridge for direct access.
func (a *MQL5Adapter) Bridge() *Bridge {
	return a.bridge
}
