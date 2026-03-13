package adapter

import (
	"sync"
	"testing"
)

func TestUpdateOrderBookAddsEntries(t *testing.T) {
	r := NewSmartRouter()
	bids := []RouteBookEntry{
		{Price: 100, Quantity: 1},
		{Price: 99, Quantity: 2},
	}
	asks := []RouteBookEntry{
		{Price: 101, Quantity: 1},
		{Price: 102, Quantity: 2},
	}
	r.UpdateOrderBook("BTC/USD", "binance", bids, asks)

	book := r.GetAggregatedBook("BTC/USD")
	if book == nil {
		t.Fatal("expected non-nil book")
	}
	if len(book.Bids) != 2 {
		t.Fatalf("expected 2 bids, got %d", len(book.Bids))
	}
	if len(book.Asks) != 2 {
		t.Fatalf("expected 2 asks, got %d", len(book.Asks))
	}
}

func TestGetAggregatedBookSorted(t *testing.T) {
	r := NewSmartRouter()
	r.UpdateOrderBook("BTC/USD", "binance", []RouteBookEntry{
		{Price: 99, Quantity: 1},
		{Price: 101, Quantity: 1},
	}, []RouteBookEntry{
		{Price: 105, Quantity: 1},
		{Price: 103, Quantity: 1},
	})
	r.UpdateOrderBook("BTC/USD", "kraken", []RouteBookEntry{
		{Price: 100, Quantity: 1},
	}, []RouteBookEntry{
		{Price: 104, Quantity: 1},
	})

	book := r.GetAggregatedBook("BTC/USD")

	// Bids should be sorted descending.
	for i := 1; i < len(book.Bids); i++ {
		if book.Bids[i].Price > book.Bids[i-1].Price {
			t.Errorf("bids not sorted desc: %v > %v at index %d", book.Bids[i].Price, book.Bids[i-1].Price, i)
		}
	}

	// Asks should be sorted ascending.
	for i := 1; i < len(book.Asks); i++ {
		if book.Asks[i].Price < book.Asks[i-1].Price {
			t.Errorf("asks not sorted asc: %v < %v at index %d", book.Asks[i].Price, book.Asks[i-1].Price, i)
		}
	}
}

func TestRouteBuyFillsCheapestFirst(t *testing.T) {
	r := NewSmartRouter()
	r.UpdateOrderBook("ETH/USD", "binance", nil, []RouteBookEntry{
		{Price: 2000, Quantity: 5},
		{Price: 2010, Quantity: 5},
	})
	r.UpdateOrderBook("ETH/USD", "kraken", nil, []RouteBookEntry{
		{Price: 1995, Quantity: 3},
	})

	result, err := r.Route("ETH/USD", "buy", 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fills) < 1 {
		t.Fatal("expected at least one fill")
	}
	// First fill should be from kraken at 1995 (cheapest).
	if result.Fills[0].Exchange != "kraken" {
		t.Errorf("expected first fill from kraken, got %s", result.Fills[0].Exchange)
	}
	if result.Fills[0].Price != 1995 {
		t.Errorf("expected first fill price 1995, got %v", result.Fills[0].Price)
	}
}

func TestRouteSellFillsHighestFirst(t *testing.T) {
	r := NewSmartRouter()
	r.UpdateOrderBook("ETH/USD", "binance", []RouteBookEntry{
		{Price: 2000, Quantity: 5},
	}, nil)
	r.UpdateOrderBook("ETH/USD", "kraken", []RouteBookEntry{
		{Price: 2010, Quantity: 3},
	}, nil)

	result, err := r.Route("ETH/USD", "sell", 4)
	if err != nil {
		t.Fatal(err)
	}
	// First fill should be from kraken at 2010 (highest bid).
	if result.Fills[0].Exchange != "kraken" {
		t.Errorf("expected first fill from kraken, got %s", result.Fills[0].Exchange)
	}
	if result.Fills[0].Price != 2010 {
		t.Errorf("expected first fill price 2010, got %v", result.Fills[0].Price)
	}
}

func TestRouteSplitsAcrossExchanges(t *testing.T) {
	r := NewSmartRouter()
	r.UpdateOrderBook("BTC/USD", "binance", nil, []RouteBookEntry{
		{Price: 50000, Quantity: 2},
	})
	r.UpdateOrderBook("BTC/USD", "kraken", nil, []RouteBookEntry{
		{Price: 50001, Quantity: 2},
	})

	result, err := r.Route("BTC/USD", "buy", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fills) != 2 {
		t.Fatalf("expected 2 fills, got %d", len(result.Fills))
	}
	if len(result.Exchanges) != 2 {
		t.Errorf("expected 2 exchanges, got %d", len(result.Exchanges))
	}
	if result.TotalQty != 3 {
		t.Errorf("expected total qty 3, got %v", result.TotalQty)
	}
}

func TestRouteWithFeesReducesSavings(t *testing.T) {
	r := NewSmartRouter()
	r.SetFee("binance", 0.001)  // 0.1%
	r.SetFee("kraken", 0.002)   // 0.2%

	r.UpdateOrderBook("BTC/USD", "binance", nil, []RouteBookEntry{
		{Price: 50000, Quantity: 2},
	})
	r.UpdateOrderBook("BTC/USD", "kraken", nil, []RouteBookEntry{
		{Price: 50001, Quantity: 2},
	})

	result, err := r.Route("BTC/USD", "buy", 3)
	if err != nil {
		t.Fatal(err)
	}

	// Cost should include fees.
	fill0 := result.Fills[0]
	expectedCost := fill0.Quantity * fill0.Price * (1 + 0.001)
	if fill0.Exchange != "binance" {
		t.Fatalf("expected first fill from binance, got %s", fill0.Exchange)
	}
	if abs(fill0.Cost-expectedCost) > 0.01 {
		t.Errorf("expected fill cost ~%v, got %v", expectedCost, fill0.Cost)
	}
}

func TestRouteInsufficientLiquidityPartialFill(t *testing.T) {
	r := NewSmartRouter()
	r.UpdateOrderBook("BTC/USD", "binance", nil, []RouteBookEntry{
		{Price: 50000, Quantity: 1},
	})

	result, err := r.Route("BTC/USD", "buy", 5)
	if err != nil {
		t.Fatal(err)
	}
	// Should fill only 1 of the 5 requested.
	if result.TotalQty != 1 {
		t.Errorf("expected partial fill of 1, got %v", result.TotalQty)
	}
}

func TestBestPriceReturnsBestBidAndAsk(t *testing.T) {
	r := NewSmartRouter()
	r.UpdateOrderBook("BTC/USD", "binance", []RouteBookEntry{
		{Price: 49990, Quantity: 1},
	}, []RouteBookEntry{
		{Price: 50010, Quantity: 1},
	})
	r.UpdateOrderBook("BTC/USD", "kraken", []RouteBookEntry{
		{Price: 50000, Quantity: 1},
	}, []RouteBookEntry{
		{Price: 50005, Quantity: 1},
	})

	bid, ask, err := r.BestPrice("BTC/USD")
	if err != nil {
		t.Fatal(err)
	}
	if bid != 50000 {
		t.Errorf("expected best bid 50000, got %v", bid)
	}
	if ask != 50005 {
		t.Errorf("expected best ask 50005, got %v", ask)
	}
}

func TestSpreadCalculation(t *testing.T) {
	r := NewSmartRouter()
	r.UpdateOrderBook("BTC/USD", "binance", []RouteBookEntry{
		{Price: 49990, Quantity: 1},
	}, []RouteBookEntry{
		{Price: 50010, Quantity: 1},
	})
	r.UpdateOrderBook("BTC/USD", "kraken", []RouteBookEntry{
		{Price: 50000, Quantity: 1},
	}, []RouteBookEntry{
		{Price: 50005, Quantity: 1},
	})

	spread, err := r.Spread("BTC/USD")
	if err != nil {
		t.Fatal(err)
	}
	// Best bid 50000, best ask 50005 => spread 5.
	if spread != 5 {
		t.Errorf("expected spread 5, got %v", spread)
	}
}

func TestSetFeeAndFeeImpactOnRouting(t *testing.T) {
	r := NewSmartRouter()
	r.UpdateOrderBook("BTC/USD", "binance", nil, []RouteBookEntry{
		{Price: 50000, Quantity: 10},
	})

	// Route without fee.
	noFee, err := r.Route("BTC/USD", "buy", 1)
	if err != nil {
		t.Fatal(err)
	}

	// Set a fee and route again.
	r.SetFee("binance", 0.01) // 1%
	withFee, err := r.Route("BTC/USD", "buy", 1)
	if err != nil {
		t.Fatal(err)
	}

	if withFee.TotalCost <= noFee.TotalCost {
		t.Errorf("expected cost with fee (%v) > cost without fee (%v)", withFee.TotalCost, noFee.TotalCost)
	}
	expected := 50000 * 1.01
	if abs(withFee.TotalCost-expected) > 0.01 {
		t.Errorf("expected cost %v, got %v", expected, withFee.TotalCost)
	}
}

func TestClearRemovesAllData(t *testing.T) {
	r := NewSmartRouter()
	r.UpdateOrderBook("BTC/USD", "binance", []RouteBookEntry{
		{Price: 100, Quantity: 1},
	}, []RouteBookEntry{
		{Price: 101, Quantity: 1},
	})

	r.Clear()

	book := r.GetAggregatedBook("BTC/USD")
	if book != nil {
		t.Error("expected nil book after clear")
	}
}

func TestConcurrentUpdateAndRoute(t *testing.T) {
	r := NewSmartRouter()
	r.UpdateOrderBook("BTC/USD", "binance", []RouteBookEntry{
		{Price: 50000, Quantity: 10},
	}, []RouteBookEntry{
		{Price: 50001, Quantity: 10},
	})

	var wg sync.WaitGroup
	const goroutines = 50

	// Half update, half route concurrently.
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		if i%2 == 0 {
			go func(i int) {
				defer wg.Done()
				r.UpdateOrderBook("BTC/USD", "kraken", []RouteBookEntry{
					{Price: 49999 + float64(i), Quantity: 1},
				}, []RouteBookEntry{
					{Price: 50002 + float64(i), Quantity: 1},
				})
			}(i)
		} else {
			go func() {
				defer wg.Done()
				_, _ = r.Route("BTC/USD", "buy", 1)
			}()
		}
	}
	wg.Wait()

	// Just verify it didn't panic or deadlock.
	book := r.GetAggregatedBook("BTC/USD")
	if book == nil {
		t.Fatal("expected non-nil book after concurrent ops")
	}
}

func TestRouteNoOrderbook(t *testing.T) {
	r := NewSmartRouter()
	_, err := r.Route("UNKNOWN", "buy", 1)
	if err == nil {
		t.Error("expected error for unknown symbol")
	}
}

func TestRouteInvalidSide(t *testing.T) {
	r := NewSmartRouter()
	r.UpdateOrderBook("BTC/USD", "binance", nil, []RouteBookEntry{
		{Price: 50000, Quantity: 1},
	})
	_, err := r.Route("BTC/USD", "invalid", 1)
	if err == nil {
		t.Error("expected error for invalid side")
	}
}

func TestUpdateOrderBookReplacesExchange(t *testing.T) {
	r := NewSmartRouter()
	r.UpdateOrderBook("BTC/USD", "binance", []RouteBookEntry{
		{Price: 100, Quantity: 5},
	}, nil)
	// Update replaces binance entries.
	r.UpdateOrderBook("BTC/USD", "binance", []RouteBookEntry{
		{Price: 200, Quantity: 10},
	}, nil)

	book := r.GetAggregatedBook("BTC/USD")
	if len(book.Bids) != 1 {
		t.Fatalf("expected 1 bid after replacement, got %d", len(book.Bids))
	}
	if book.Bids[0].Price != 200 || book.Bids[0].Quantity != 10 {
		t.Errorf("expected replaced bid {200, 10}, got {%v, %v}", book.Bids[0].Price, book.Bids[0].Quantity)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
