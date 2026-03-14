package api

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// handleGetPrice returns the current price for a symbol from the configured exchange.
func (s *Server) handleGetPrice(w http.ResponseWriter, r *http.Request) {
	symbol := r.URL.Query().Get("symbol")
	exchange := r.URL.Query().Get("exchange")
	if symbol == "" {
		http.Error(w, `{"error":"symbol parameter required"}`, http.StatusBadRequest)
		return
	}
	if exchange == "" {
		exchange = "binance"
	}

	adp, ok := s.adapters[exchange]
	if !ok {
		http.Error(w, `{"error":"exchange not configured: `+exchange+`"}`, http.StatusNotFound)
		return
	}

	price, err := adp.GetPrice(r.Context(), symbol)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(price)
}

// handleGetCandles returns candlestick data for a symbol.
func (s *Server) handleGetCandles(w http.ResponseWriter, r *http.Request) {
	symbol := r.URL.Query().Get("symbol")
	exchange := r.URL.Query().Get("exchange")
	timeframe := r.URL.Query().Get("timeframe")
	limitStr := r.URL.Query().Get("limit")

	if symbol == "" {
		http.Error(w, `{"error":"symbol parameter required"}`, http.StatusBadRequest)
		return
	}
	if exchange == "" {
		exchange = "binance"
	}
	if timeframe == "" {
		timeframe = "1h"
	}
	limit := 100
	if limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil {
			limit = v
		}
	}

	adp, ok := s.adapters[exchange]
	if !ok {
		http.Error(w, `{"error":"exchange not configured: `+exchange+`"}`, http.StatusNotFound)
		return
	}

	candles, err := adp.GetCandles(r.Context(), symbol, timeframe, limit)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(candles)
}

// handleGetOrderBook returns the order book for a symbol.
func (s *Server) handleGetOrderBook(w http.ResponseWriter, r *http.Request) {
	symbol := r.URL.Query().Get("symbol")
	exchange := r.URL.Query().Get("exchange")
	depthStr := r.URL.Query().Get("depth")

	if symbol == "" {
		http.Error(w, `{"error":"symbol parameter required"}`, http.StatusBadRequest)
		return
	}
	if exchange == "" {
		exchange = "binance"
	}
	depth := 20
	if depthStr != "" {
		if v, err := strconv.Atoi(depthStr); err == nil {
			depth = v
		}
	}

	adp, ok := s.adapters[exchange]
	if !ok {
		http.Error(w, `{"error":"exchange not configured: `+exchange+`"}`, http.StatusNotFound)
		return
	}

	ob, err := adp.GetOrderBook(r.Context(), symbol, depth)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ob)
}

// handleGetBalances returns account balances from the exchange.
func (s *Server) handleGetBalances(w http.ResponseWriter, r *http.Request) {
	exchange := r.URL.Query().Get("exchange")
	if exchange == "" {
		exchange = "binance"
	}

	adp, ok := s.adapters[exchange]
	if !ok {
		http.Error(w, `{"error":"exchange not configured: `+exchange+`"}`, http.StatusNotFound)
		return
	}

	balances, err := adp.GetBalances(r.Context())
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(balances)
}

// handleGetPositions returns open positions from the exchange.
func (s *Server) handleGetPositions(w http.ResponseWriter, r *http.Request) {
	exchange := r.URL.Query().Get("exchange")
	if exchange == "" {
		exchange = "binance"
	}

	adp, ok := s.adapters[exchange]
	if !ok {
		http.Error(w, `{"error":"exchange not configured: `+exchange+`"}`, http.StatusNotFound)
		return
	}

	positions, err := adp.GetPositions(r.Context())
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(positions)
}

// handleGetPortfolio returns aggregated portfolio data across all connected exchanges.
func (s *Server) handleGetPortfolio(w http.ResponseWriter, r *http.Request) {
	var allBalances []any
	var allPositions []any
	var totalPnL float64
	exchanges := map[string]any{}

	for name, adp := range s.adapters {
		if !adp.IsConnected() {
			continue
		}
		balances, err := adp.GetBalances(r.Context())
		if err != nil {
			continue
		}
		positions, err := adp.GetPositions(r.Context())
		if err != nil {
			continue
		}

		var exchTotal float64
		for _, b := range balances {
			allBalances = append(allBalances, b)
			exchTotal += b.Total
		}
		for _, p := range positions {
			allPositions = append(allPositions, p)
			totalPnL += p.PnL
		}
		exchanges[name] = map[string]any{
			"total":     exchTotal,
			"balances":  balances,
			"positions": positions,
		}
	}

	if allBalances == nil {
		allBalances = []any{}
	}
	if allPositions == nil {
		allPositions = []any{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"balances":  allBalances,
		"positions": allPositions,
		"total_pnl": totalPnL,
		"exchanges": exchanges,
	})
}

// handleListExchanges returns the list of configured exchanges.
func (s *Server) handleListExchanges(w http.ResponseWriter, r *http.Request) {
	var exchanges []map[string]any
	for name, adp := range s.adapters {
		exchanges = append(exchanges, map[string]any{
			"name":      name,
			"connected": adp.IsConnected(),
			"caps":      adp.Capabilities(),
		})
	}
	if exchanges == nil {
		exchanges = []map[string]any{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(exchanges)
}
