// internal/api/server.go
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/clawtrade/clawtrade/internal/adapter"
	"github.com/clawtrade/clawtrade/internal/agent"
	"github.com/clawtrade/clawtrade/internal/backtest"
	"github.com/clawtrade/clawtrade/internal/config"
	"github.com/clawtrade/clawtrade/internal/engine"
	"github.com/clawtrade/clawtrade/internal/mcp"
	"github.com/clawtrade/clawtrade/internal/memory"
	"github.com/clawtrade/clawtrade/internal/risk"
	"github.com/clawtrade/clawtrade/internal/security"
	"github.com/clawtrade/clawtrade/internal/subagent"
)

type Server struct {
	router   chi.Router
	cfg      *config.Config
	bus      *engine.EventBus
	hub      *Hub
	memory   *memory.Store
	audit    *security.AuditLog
	adapters map[string]adapter.TradingAdapter
	agentMgr *subagent.AgentManager
	ag       *agent.Agent
}

// SetAgentManager sets the sub-agent manager for agent status endpoints.
func (s *Server) SetAgentManager(mgr *subagent.AgentManager) {
	s.agentMgr = mgr
}

// SetAlertService connects the alert service to the agent's tools.
func (s *Server) SetAlertService(svc interface{}) {
	if as, ok := svc.(agent.AlertService); ok {
		s.ag.SetAlertService(as)
	}
}

func NewServer(cfg *config.Config, bus *engine.EventBus, mem *memory.Store, audit *security.AuditLog, adapters map[string]adapter.TradingAdapter, riskEngine *risk.Engine, db *sql.DB) *Server {
	s := &Server{
		cfg:      cfg,
		bus:      bus,
		memory:   mem,
		audit:    audit,
		adapters: adapters,
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(120 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
	}))

	ag := agent.New(cfg, adapters, riskEngine, mem, bus, db)

	// Connect external MCP servers to agent
	if len(cfg.MCP.Servers) > 0 {
		mcpManager := mcp.NewClientManager()
		for _, srv := range cfg.MCP.Servers {
			if !srv.Enabled {
				continue
			}
			client := mcp.NewClient(srv.Name, srv.Command, srv.Args, srv.Env)
			if err := mcpManager.Add(client); err != nil {
				fmt.Printf("MCP server %s: failed to connect: %v\n", srv.Name, err)
				continue
			}
			fmt.Printf("MCP server %s: connected\n", srv.Name)
		}
		if mcpManager.ServerCount() > 0 {
			ag.SetMCPBridge(agent.NewMCPBridge(mcpManager))
		}
	}

	s.ag = ag

	llm := NewLLMHandler(ag)

	// WebSocket hub for real-time data
	hub := NewHub()
	go hub.Run()
	hub.SubscribeToEvents(bus, []string{
		"market.*",
		"trade.*",
		"risk.*",
		"system.*",
		"price.*",     // live price updates
		"agent.*",     // sub-agent insights
		"portfolio.*", // portfolio changes
		"backtest.*",  // backtest progress & results
		"alert.*",     // alert triggers
	})
	s.hub = hub

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/system/health", s.handleHealth)
		r.Get("/system/version", s.handleVersion)
		r.Post("/chat", llm.HandleChat)

		// Portfolio (aggregated across all exchanges)
		r.Get("/portfolio", s.handleGetPortfolio)

		// Exchange data
		r.Get("/price", s.handleGetPrice)
		r.Get("/candles", s.handleGetCandles)
		r.Get("/orderbook", s.handleGetOrderBook)
		r.Get("/balances", s.handleGetBalances)
		r.Get("/positions", s.handleGetPositions)
		r.Get("/exchanges", s.handleListExchanges)

		// Backtesting
		r.Post("/backtest", s.handleBacktest)

		// Sub-agent management
		r.Get("/agents", s.handleListAgents)
		r.Get("/agents/events", s.handleAgentEvents)
	})

	// WebSocket endpoint
	r.Get("/ws", hub.HandleWebSocket)

	s.router = r
	return s
}

func (s *Server) Router() chi.Router {
	return s.router
}

func (s *Server) Start(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:    addr,
		Handler: s.router,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	fmt.Printf("API server listening on %s\n", addr)
	return srv.ListenAndServe()
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	if s.agentMgr == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.agentMgr.Statuses())
}

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
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}

	// Defaults
	if req.Symbol == "" {
		req.Symbol = "BTC/USDT"
	}
	if req.Timeframe == "" {
		req.Timeframe = "1d"
	}
	if req.Days <= 0 {
		req.Days = 90
	}
	if req.Strategy == "" {
		req.Strategy = "momentum"
	}
	if req.Capital <= 0 {
		req.Capital = 10000
	}
	if req.Exchange == "" {
		req.Exchange = "binance"
	}

	adp, ok := s.adapters[req.Exchange]
	if !ok {
		http.Error(w, fmt.Sprintf(`{"error":"unknown exchange %q"}`, req.Exchange), http.StatusBadRequest)
		return
	}

	loader := backtest.NewDataLoader(nil, adp)
	now := time.Now().UTC()
	from := now.AddDate(0, 0, -req.Days)
	candles, err := loader.LoadCandles(r.Context(), req.Symbol, req.Timeframe, from, now)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"load candles: %s"}`, err), http.StatusInternalServerError)
		return
	}

	strat := backtest.GetBuiltinStrategy(req.Strategy)
	if strat == nil {
		http.Error(w, fmt.Sprintf(`{"error":"unknown strategy %q"}`, req.Strategy), http.StatusBadRequest)
		return
	}

	eng := &backtest.Engine{Bus: s.bus}
	result, err := eng.Run(r.Context(), backtest.BacktestConfig{
		Symbol:    req.Symbol,
		Timeframe: req.Timeframe,
		From:      from,
		To:        now,
		Capital:   req.Capital,
	}, candles, strat)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"backtest run: %s"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleAgentEvents(w http.ResponseWriter, r *http.Request) {
	if s.agentMgr == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{})
		return
	}
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.agentMgr.Bus().RecentEvents(limit))
}
