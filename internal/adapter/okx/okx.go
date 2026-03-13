// internal/adapter/okx/okx.go
package okx

import (
	"context"
	"fmt"

	"github.com/clawtrade/clawtrade/internal/adapter"
)

var ErrNotImplemented = fmt.Errorf("not implemented")

// Adapter implements the TradingAdapter interface for OKX.
type Adapter struct {
	apiKey    string
	apiSecret string
	connected bool
}

// New creates a new OKX adapter.
func New(apiKey, apiSecret string) *Adapter {
	return &Adapter{
		apiKey:    apiKey,
		apiSecret: apiSecret,
	}
}

func (a *Adapter) Name() string {
	return "okx"
}

func (a *Adapter) Capabilities() adapter.AdapterCaps {
	return adapter.AdapterCaps{
		Name:      "okx",
		WebSocket: true,
		Margin:    true,
		Futures:   true,
		OrderTypes: []adapter.OrderType{
			adapter.OrderTypeMarket,
			adapter.OrderTypeLimit,
			adapter.OrderTypeStop,
		},
	}
}

func (a *Adapter) GetPrice(ctx context.Context, symbol string) (*adapter.Price, error) {
	return nil, ErrNotImplemented
}

func (a *Adapter) GetCandles(ctx context.Context, symbol, timeframe string, limit int) ([]adapter.Candle, error) {
	return nil, ErrNotImplemented
}

func (a *Adapter) GetOrderBook(ctx context.Context, symbol string, depth int) (*adapter.OrderBook, error) {
	return nil, ErrNotImplemented
}

func (a *Adapter) PlaceOrder(ctx context.Context, order adapter.Order) (*adapter.Order, error) {
	return nil, ErrNotImplemented
}

func (a *Adapter) CancelOrder(ctx context.Context, orderID string) error {
	return ErrNotImplemented
}

func (a *Adapter) GetOpenOrders(ctx context.Context) ([]adapter.Order, error) {
	return nil, ErrNotImplemented
}

func (a *Adapter) GetBalances(ctx context.Context) ([]adapter.Balance, error) {
	return nil, ErrNotImplemented
}

func (a *Adapter) GetPositions(ctx context.Context) ([]adapter.Position, error) {
	return nil, ErrNotImplemented
}

func (a *Adapter) Connect(ctx context.Context) error {
	a.connected = true
	return nil
}

func (a *Adapter) Disconnect() error {
	a.connected = false
	return nil
}

func (a *Adapter) IsConnected() bool {
	return a.connected
}
