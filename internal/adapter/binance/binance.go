// internal/adapter/binance/binance.go
package binance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/clawtrade/clawtrade/internal/adapter"
	"github.com/gorilla/websocket"
)

const (
	restBaseURL = "https://api.binance.com"
)

// PriceCallback is called when a real-time price update arrives via WebSocket.
type PriceCallback func(price adapter.Price)

// Adapter implements the TradingAdapter interface for Binance.
type Adapter struct {
	apiKey    string
	apiSecret string
	testnet   bool

	mu        sync.RWMutex
	connected bool
	wsConn    *websocket.Conn
	wsCancel  context.CancelFunc
	prices    map[string]adapter.Price

	onPrice PriceCallback
}

// New creates a new Binance adapter.
func New(apiKey, apiSecret string) *Adapter {
	return &Adapter{
		apiKey:    apiKey,
		apiSecret: apiSecret,
		prices:    make(map[string]adapter.Price),
	}
}

// SetTestnet enables the testnet endpoints.
func (a *Adapter) SetTestnet(enabled bool) {
	a.testnet = enabled
}

// OnPrice registers a callback for real-time price updates.
func (a *Adapter) OnPrice(cb PriceCallback) {
	a.onPrice = cb
}

func (a *Adapter) baseURL() string {
	if a.testnet {
		return "https://testnet.binance.vision"
	}
	return restBaseURL
}

func (a *Adapter) Name() string {
	return "binance"
}

func (a *Adapter) Capabilities() adapter.AdapterCaps {
	return adapter.AdapterCaps{
		Name:      "binance",
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

// ─── REST API: Market Data ──────────────────────────────────────────

func (a *Adapter) GetPrice(ctx context.Context, symbol string) (*adapter.Price, error) {
	sym := toBinanceSymbol(symbol)

	resp, err := a.publicGet(ctx, "/api/v3/ticker/24hr", url.Values{"symbol": {sym}})
	if err != nil {
		return nil, fmt.Errorf("get price: %w", err)
	}

	var ticker struct {
		Symbol    string `json:"symbol"`
		BidPrice  string `json:"bidPrice"`
		AskPrice  string `json:"askPrice"`
		LastPrice string `json:"lastPrice"`
		Volume    string `json:"volume"`
	}
	if err := json.Unmarshal(resp, &ticker); err != nil {
		return nil, fmt.Errorf("parse ticker: %w", err)
	}

	bid, _ := strconv.ParseFloat(ticker.BidPrice, 64)
	ask, _ := strconv.ParseFloat(ticker.AskPrice, 64)
	last, _ := strconv.ParseFloat(ticker.LastPrice, 64)
	vol, _ := strconv.ParseFloat(ticker.Volume, 64)

	return &adapter.Price{
		Symbol:    symbol,
		Bid:       bid,
		Ask:       ask,
		Last:      last,
		Volume24h: vol,
		Timestamp: time.Now(),
	}, nil
}

func (a *Adapter) GetCandles(ctx context.Context, symbol, timeframe string, limit int) ([]adapter.Candle, error) {
	sym := toBinanceSymbol(symbol)

	params := url.Values{
		"symbol":   {sym},
		"interval": {timeframe},
		"limit":    {strconv.Itoa(limit)},
	}

	resp, err := a.publicGet(ctx, "/api/v3/klines", params)
	if err != nil {
		return nil, fmt.Errorf("get candles: %w", err)
	}

	var raw [][]json.RawMessage
	if err := json.Unmarshal(resp, &raw); err != nil {
		return nil, fmt.Errorf("parse klines: %w", err)
	}

	candles := make([]adapter.Candle, 0, len(raw))
	for _, k := range raw {
		if len(k) < 6 {
			continue
		}
		var tsMs float64
		var openS, highS, lowS, closeS, volS string
		json.Unmarshal(k[0], &tsMs)
		json.Unmarshal(k[1], &openS)
		json.Unmarshal(k[2], &highS)
		json.Unmarshal(k[3], &lowS)
		json.Unmarshal(k[4], &closeS)
		json.Unmarshal(k[5], &volS)

		o, _ := strconv.ParseFloat(openS, 64)
		h, _ := strconv.ParseFloat(highS, 64)
		l, _ := strconv.ParseFloat(lowS, 64)
		c, _ := strconv.ParseFloat(closeS, 64)
		v, _ := strconv.ParseFloat(volS, 64)

		candles = append(candles, adapter.Candle{
			Open:      o,
			High:      h,
			Low:       l,
			Close:     c,
			Volume:    v,
			Timestamp: time.UnixMilli(int64(tsMs)),
		})
	}

	return candles, nil
}

func (a *Adapter) GetOrderBook(ctx context.Context, symbol string, depth int) (*adapter.OrderBook, error) {
	sym := toBinanceSymbol(symbol)
	if depth <= 0 {
		depth = 20
	}

	params := url.Values{
		"symbol": {sym},
		"limit":  {strconv.Itoa(depth)},
	}

	resp, err := a.publicGet(ctx, "/api/v3/depth", params)
	if err != nil {
		return nil, fmt.Errorf("get order book: %w", err)
	}

	var raw struct {
		Bids [][]string `json:"bids"`
		Asks [][]string `json:"asks"`
	}
	if err := json.Unmarshal(resp, &raw); err != nil {
		return nil, fmt.Errorf("parse depth: %w", err)
	}

	ob := &adapter.OrderBook{Symbol: symbol}
	for _, b := range raw.Bids {
		if len(b) >= 2 {
			p, _ := strconv.ParseFloat(b[0], 64)
			amt, _ := strconv.ParseFloat(b[1], 64)
			ob.Bids = append(ob.Bids, adapter.OrderBookEntry{Price: p, Amount: amt})
		}
	}
	for _, ask := range raw.Asks {
		if len(ask) >= 2 {
			p, _ := strconv.ParseFloat(ask[0], 64)
			amt, _ := strconv.ParseFloat(ask[1], 64)
			ob.Asks = append(ob.Asks, adapter.OrderBookEntry{Price: p, Amount: amt})
		}
	}

	return ob, nil
}

// ─── REST API: Trading ──────────────────────────────────────────────

func (a *Adapter) PlaceOrder(ctx context.Context, order adapter.Order) (*adapter.Order, error) {
	params := url.Values{
		"symbol": {toBinanceSymbol(order.Symbol)},
		"side":   {string(order.Side)},
		"type":   {string(order.Type)},
	}

	switch order.Type {
	case adapter.OrderTypeMarket:
		params.Set("quantity", strconv.FormatFloat(order.Size, 'f', -1, 64))
	case adapter.OrderTypeLimit:
		params.Set("quantity", strconv.FormatFloat(order.Size, 'f', -1, 64))
		params.Set("price", strconv.FormatFloat(order.Price, 'f', -1, 64))
		params.Set("timeInForce", "GTC")
	case adapter.OrderTypeStop:
		params.Set("quantity", strconv.FormatFloat(order.Size, 'f', -1, 64))
		params.Set("stopPrice", strconv.FormatFloat(order.Price, 'f', -1, 64))
		params.Set("type", "STOP_LOSS")
	}

	resp, err := a.signedPost(ctx, "/api/v3/order", params)
	if err != nil {
		return nil, fmt.Errorf("place order: %w", err)
	}

	var result struct {
		OrderID      int64  `json:"orderId"`
		Symbol       string `json:"symbol"`
		Side         string `json:"side"`
		Type         string `json:"type"`
		Price        string `json:"price"`
		OrigQty      string `json:"origQty"`
		Status       string `json:"status"`
		TransactTime int64  `json:"transactTime"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse order response: %w", err)
	}

	price, _ := strconv.ParseFloat(result.Price, 64)
	size, _ := strconv.ParseFloat(result.OrigQty, 64)

	return &adapter.Order{
		ID:        strconv.FormatInt(result.OrderID, 10),
		Symbol:    order.Symbol,
		Side:      adapter.Side(result.Side),
		Type:      adapter.OrderType(result.Type),
		Price:     price,
		Size:      size,
		Status:    mapOrderStatus(result.Status),
		Exchange:  "binance",
		CreatedAt: time.UnixMilli(result.TransactTime),
	}, nil
}

func (a *Adapter) CancelOrder(ctx context.Context, orderID string) error {
	parts := strings.SplitN(orderID, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("orderID must be in symbol:orderId format")
	}

	params := url.Values{
		"symbol":  {parts[0]},
		"orderId": {parts[1]},
	}

	_, err := a.signedRequest(ctx, "DELETE", "/api/v3/order", params)
	return err
}

func (a *Adapter) GetOpenOrders(ctx context.Context) ([]adapter.Order, error) {
	resp, err := a.signedGet(ctx, "/api/v3/openOrders", url.Values{})
	if err != nil {
		return nil, fmt.Errorf("get open orders: %w", err)
	}

	var raw []struct {
		OrderID int64  `json:"orderId"`
		Symbol  string `json:"symbol"`
		Side    string `json:"side"`
		Type    string `json:"type"`
		Price   string `json:"price"`
		OrigQty string `json:"origQty"`
		Status  string `json:"status"`
		Time    int64  `json:"time"`
	}
	if err := json.Unmarshal(resp, &raw); err != nil {
		return nil, fmt.Errorf("parse orders: %w", err)
	}

	orders := make([]adapter.Order, 0, len(raw))
	for _, r := range raw {
		price, _ := strconv.ParseFloat(r.Price, 64)
		size, _ := strconv.ParseFloat(r.OrigQty, 64)
		orders = append(orders, adapter.Order{
			ID:        strconv.FormatInt(r.OrderID, 10),
			Symbol:    fromBinanceSymbol(r.Symbol),
			Side:      adapter.Side(r.Side),
			Type:      adapter.OrderType(r.Type),
			Price:     price,
			Size:      size,
			Status:    mapOrderStatus(r.Status),
			Exchange:  "binance",
			CreatedAt: time.UnixMilli(r.Time),
		})
	}

	return orders, nil
}

func (a *Adapter) GetBalances(ctx context.Context) ([]adapter.Balance, error) {
	resp, err := a.signedGet(ctx, "/api/v3/account", url.Values{})
	if err != nil {
		return nil, fmt.Errorf("get balances: %w", err)
	}

	var account struct {
		Balances []struct {
			Asset  string `json:"asset"`
			Free   string `json:"free"`
			Locked string `json:"locked"`
		} `json:"balances"`
	}
	if err := json.Unmarshal(resp, &account); err != nil {
		return nil, fmt.Errorf("parse account: %w", err)
	}

	var balances []adapter.Balance
	for _, b := range account.Balances {
		free, _ := strconv.ParseFloat(b.Free, 64)
		locked, _ := strconv.ParseFloat(b.Locked, 64)
		total := free + locked
		if total > 0 {
			balances = append(balances, adapter.Balance{
				Asset:  b.Asset,
				Free:   free,
				Locked: locked,
				Total:  total,
			})
		}
	}

	return balances, nil
}

func (a *Adapter) GetPositions(ctx context.Context) ([]adapter.Position, error) {
	balances, err := a.GetBalances(ctx)
	if err != nil {
		return nil, err
	}

	var positions []adapter.Position
	for _, b := range balances {
		if b.Asset == "USDT" || b.Asset == "BUSD" || b.Asset == "USDC" {
			continue
		}
		if b.Total <= 0 {
			continue
		}

		symbol := b.Asset + "/USDT"
		price, err := a.GetPrice(ctx, symbol)
		if err != nil {
			continue
		}

		positions = append(positions, adapter.Position{
			Symbol:       symbol,
			Side:         adapter.SideBuy,
			Size:         b.Total,
			EntryPrice:   0,
			CurrentPrice: price.Last,
			PnL:          0,
			Exchange:     "binance",
			OpenedAt:     time.Now(),
		})
	}

	return positions, nil
}

// ─── WebSocket: Real-time Price Stream ──────────────────────────────

func (a *Adapter) Connect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.connected {
		return nil
	}

	a.connected = true
	return nil
}

// SubscribePrices connects to Binance WebSocket and streams mini-ticker for given symbols.
func (a *Adapter) SubscribePrices(ctx context.Context, symbols []string) error {
	if len(symbols) == 0 {
		return nil
	}

	var streams []string
	for _, s := range symbols {
		sym := strings.ToLower(strings.ReplaceAll(s, "/", ""))
		streams = append(streams, sym+"@miniTicker")
	}

	wsURL := "wss://stream.binance.com:9443/stream?streams=" + strings.Join(streams, "/")

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("websocket connect: %w", err)
	}

	a.mu.Lock()
	a.wsConn = conn
	a.connected = true
	wsCtx, cancel := context.WithCancel(ctx)
	a.wsCancel = cancel
	a.mu.Unlock()

	go a.readWsLoop(wsCtx)

	log.Printf("binance: WebSocket connected, streaming %d symbols", len(symbols))
	return nil
}

func (a *Adapter) readWsLoop(ctx context.Context) {
	defer func() {
		a.mu.Lock()
		a.connected = false
		if a.wsConn != nil {
			a.wsConn.Close()
		}
		a.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_, message, err := a.wsConn.ReadMessage()
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("binance: websocket read error: %v", err)
			}
			return
		}

		var envelope struct {
			Stream string          `json:"stream"`
			Data   json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(message, &envelope); err != nil {
			continue
		}

		var ticker struct {
			Symbol string `json:"s"`
			Close  string `json:"c"`
			Open   string `json:"o"`
			High   string `json:"h"`
			Low    string `json:"l"`
			Volume string `json:"v"`
		}
		if err := json.Unmarshal(envelope.Data, &ticker); err != nil {
			continue
		}

		last, _ := strconv.ParseFloat(ticker.Close, 64)
		vol, _ := strconv.ParseFloat(ticker.Volume, 64)

		price := adapter.Price{
			Symbol:    fromBinanceSymbol(ticker.Symbol),
			Last:      last,
			Bid:       last,
			Ask:       last,
			Volume24h: vol,
			Timestamp: time.Now(),
		}

		a.mu.Lock()
		a.prices[price.Symbol] = price
		a.mu.Unlock()

		if a.onPrice != nil {
			a.onPrice(price)
		}
	}
}

// GetCachedPrice returns the last known price from WebSocket stream.
func (a *Adapter) GetCachedPrice(symbol string) (adapter.Price, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	p, ok := a.prices[symbol]
	return p, ok
}

func (a *Adapter) Disconnect() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.wsCancel != nil {
		a.wsCancel()
	}
	if a.wsConn != nil {
		a.wsConn.Close()
		a.wsConn = nil
	}
	a.connected = false
	return nil
}

func (a *Adapter) IsConnected() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.connected
}

// ─── HTTP helpers ───────────────────────────────────────────────────

func (a *Adapter) publicGet(ctx context.Context, path string, params url.Values) ([]byte, error) {
	u := a.baseURL() + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("binance API error (%d): %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (a *Adapter) signedGet(ctx context.Context, path string, params url.Values) ([]byte, error) {
	return a.signedRequest(ctx, "GET", path, params)
}

func (a *Adapter) signedPost(ctx context.Context, path string, params url.Values) ([]byte, error) {
	return a.signedRequest(ctx, "POST", path, params)
}

func (a *Adapter) signedRequest(ctx context.Context, method, path string, params url.Values) ([]byte, error) {
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	params.Set("recvWindow", "5000")

	queryString := params.Encode()
	signature := a.sign(queryString)
	params.Set("signature", signature)

	u := a.baseURL() + path + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-MBX-APIKEY", a.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("binance API error (%d): %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (a *Adapter) sign(payload string) string {
	mac := hmac.New(sha256.New, []byte(a.apiSecret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// ─── Symbol conversion ─────────────────────────────────────────────

func toBinanceSymbol(symbol string) string {
	return strings.ReplaceAll(strings.ToUpper(symbol), "/", "")
}

func fromBinanceSymbol(symbol string) string {
	stables := []string{"USDT", "BUSD", "USDC", "BTC", "ETH", "BNB"}
	upper := strings.ToUpper(symbol)
	for _, quote := range stables {
		if strings.HasSuffix(upper, quote) {
			base := upper[:len(upper)-len(quote)]
			if base != "" {
				return base + "/" + quote
			}
		}
	}
	return symbol
}

func mapOrderStatus(s string) adapter.OrderStatus {
	switch s {
	case "NEW", "PARTIALLY_FILLED":
		return adapter.OrderStatusPending
	case "FILLED":
		return adapter.OrderStatusFilled
	case "CANCELED", "EXPIRED", "REJECTED":
		return adapter.OrderStatusCanceled
	default:
		return adapter.OrderStatusPending
	}
}
