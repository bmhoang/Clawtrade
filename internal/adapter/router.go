// internal/adapter/router.go
package adapter

import (
	"fmt"
	"sort"
	"sync"
)

// RouteBookEntry represents a single price level from a specific exchange.
type RouteBookEntry struct {
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"`
	Exchange string  `json:"exchange"`
}

// AggregatedOrderBook combines orderbooks from multiple exchanges.
type AggregatedOrderBook struct {
	Symbol string           `json:"symbol"`
	Bids   []RouteBookEntry `json:"bids"` // sorted price desc
	Asks   []RouteBookEntry `json:"asks"` // sorted price asc
}

// RoutingResult shows how an order would be split across exchanges.
type RoutingResult struct {
	Symbol    string      `json:"symbol"`
	Side      string      `json:"side"` // "buy" or "sell"
	TotalQty  float64     `json:"total_qty"`
	Fills     []RouteFill `json:"fills"`
	AvgPrice  float64     `json:"avg_price"`
	TotalCost float64     `json:"total_cost"`
	Exchanges []string    `json:"exchanges"` // unique exchanges used
	Savings   float64     `json:"savings"`   // vs worst single-exchange price
}

// RouteFill is a single fill on one exchange.
type RouteFill struct {
	Exchange string  `json:"exchange"`
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"`
	Cost     float64 `json:"cost"`
}

// SmartRouter routes orders across multiple exchanges for best execution.
type SmartRouter struct {
	mu    sync.RWMutex
	books map[string]*AggregatedOrderBook // by symbol
	fees  map[string]float64              // exchange -> fee rate
}

// NewSmartRouter creates a new router.
func NewSmartRouter() *SmartRouter {
	return &SmartRouter{
		books: make(map[string]*AggregatedOrderBook),
		fees:  make(map[string]float64),
	}
}

// SetFee sets the fee rate for an exchange (e.g., 0.001 for 0.1%).
func (r *SmartRouter) SetFee(exchange string, rate float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fees[exchange] = rate
}

// UpdateOrderBook updates the aggregated book for a symbol from one exchange.
// It replaces all entries from the given exchange and re-sorts.
func (r *SmartRouter) UpdateOrderBook(symbol, exchange string, bids, asks []RouteBookEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()

	book, ok := r.books[symbol]
	if !ok {
		book = &AggregatedOrderBook{Symbol: symbol}
		r.books[symbol] = book
	}

	// Remove existing entries from this exchange.
	book.Bids = filterExchange(book.Bids, exchange)
	book.Asks = filterExchange(book.Asks, exchange)

	// Tag entries with exchange and append.
	for _, b := range bids {
		b.Exchange = exchange
		book.Bids = append(book.Bids, b)
	}
	for _, a := range asks {
		a.Exchange = exchange
		book.Asks = append(book.Asks, a)
	}

	// Sort bids descending by price.
	sort.Slice(book.Bids, func(i, j int) bool {
		return book.Bids[i].Price > book.Bids[j].Price
	})
	// Sort asks ascending by price.
	sort.Slice(book.Asks, func(i, j int) bool {
		return book.Asks[i].Price < book.Asks[j].Price
	})
}

// filterExchange removes all entries belonging to a given exchange.
func filterExchange(entries []RouteBookEntry, exchange string) []RouteBookEntry {
	n := 0
	for _, e := range entries {
		if e.Exchange != exchange {
			entries[n] = e
			n++
		}
	}
	return entries[:n]
}

// GetAggregatedBook returns the combined orderbook for a symbol.
func (r *SmartRouter) GetAggregatedBook(symbol string) *AggregatedOrderBook {
	r.mu.RLock()
	defer r.mu.RUnlock()

	book, ok := r.books[symbol]
	if !ok {
		return nil
	}

	// Return a copy so callers can't mutate internal state.
	cp := &AggregatedOrderBook{
		Symbol: book.Symbol,
		Bids:   make([]RouteBookEntry, len(book.Bids)),
		Asks:   make([]RouteBookEntry, len(book.Asks)),
	}
	copy(cp.Bids, book.Bids)
	copy(cp.Asks, book.Asks)
	return cp
}

// Route calculates optimal order routing for a given quantity.
func (r *SmartRouter) Route(symbol, side string, quantity float64) (*RoutingResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	book, ok := r.books[symbol]
	if !ok {
		return nil, fmt.Errorf("no orderbook for symbol %s", symbol)
	}

	var levels []RouteBookEntry
	switch side {
	case "buy":
		levels = book.Asks // walk asks lowest first
	case "sell":
		levels = book.Bids // walk bids highest first
	default:
		return nil, fmt.Errorf("invalid side %q: must be \"buy\" or \"sell\"", side)
	}

	if len(levels) == 0 {
		return nil, fmt.Errorf("no liquidity for %s %s", side, symbol)
	}

	result := &RoutingResult{
		Symbol: symbol,
		Side:   side,
	}

	remaining := quantity
	exchangeSet := make(map[string]bool)

	for _, lvl := range levels {
		if remaining <= 0 {
			break
		}
		fillQty := lvl.Quantity
		if fillQty > remaining {
			fillQty = remaining
		}

		fee := r.fees[lvl.Exchange]
		cost := fillQty * lvl.Price * (1 + fee)
		if side == "sell" {
			cost = fillQty * lvl.Price * (1 - fee)
		}

		result.Fills = append(result.Fills, RouteFill{
			Exchange: lvl.Exchange,
			Price:    lvl.Price,
			Quantity: fillQty,
			Cost:     cost,
		})

		result.TotalCost += cost
		result.TotalQty += fillQty
		exchangeSet[lvl.Exchange] = true
		remaining -= fillQty
	}

	if result.TotalQty > 0 {
		result.AvgPrice = result.TotalCost / result.TotalQty
	}

	for ex := range exchangeSet {
		result.Exchanges = append(result.Exchanges, ex)
	}
	sort.Strings(result.Exchanges)

	// Calculate savings vs worst single-exchange execution.
	result.Savings = r.calcSavings(result, levels, side)

	return result, nil
}

// calcSavings computes how much better the routed result is versus filling
// entirely on the single worst-priced exchange that was used.
func (r *SmartRouter) calcSavings(result *RoutingResult, levels []RouteBookEntry, side string) float64 {
	if len(result.Fills) <= 1 {
		return 0
	}

	// Find the worst price among the fills.
	worstPrice := result.Fills[0].Price
	worstExchange := result.Fills[0].Exchange
	for _, f := range result.Fills {
		if side == "buy" && f.Price > worstPrice {
			worstPrice = f.Price
			worstExchange = f.Exchange
		} else if side == "sell" && f.Price < worstPrice {
			worstPrice = f.Price
			worstExchange = f.Exchange
		}
	}

	fee := r.fees[worstExchange]
	var worstCost float64
	if side == "buy" {
		worstCost = result.TotalQty * worstPrice * (1 + fee)
	} else {
		worstCost = result.TotalQty * worstPrice * (1 - fee)
	}

	if side == "buy" {
		return worstCost - result.TotalCost
	}
	return result.TotalCost - worstCost
}

// BestPrice returns the best bid/ask across all exchanges for a symbol.
func (r *SmartRouter) BestPrice(symbol string) (bestBid, bestAsk float64, err error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	book, ok := r.books[symbol]
	if !ok {
		return 0, 0, fmt.Errorf("no orderbook for symbol %s", symbol)
	}

	if len(book.Bids) == 0 && len(book.Asks) == 0 {
		return 0, 0, fmt.Errorf("empty orderbook for symbol %s", symbol)
	}

	if len(book.Bids) > 0 {
		bestBid = book.Bids[0].Price // highest bid
	}
	if len(book.Asks) > 0 {
		bestAsk = book.Asks[0].Price // lowest ask
	}
	return bestBid, bestAsk, nil
}

// Spread returns the best bid-ask spread across exchanges for a symbol.
func (r *SmartRouter) Spread(symbol string) (float64, error) {
	bid, ask, err := r.BestPrice(symbol)
	if err != nil {
		return 0, err
	}
	if bid == 0 || ask == 0 {
		return 0, fmt.Errorf("incomplete orderbook for symbol %s", symbol)
	}
	return ask - bid, nil
}

// Clear removes all orderbook data.
func (r *SmartRouter) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.books = make(map[string]*AggregatedOrderBook)
}
