// internal/api/server.go
package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/clawtrade/clawtrade/internal/adapter"
	"github.com/clawtrade/clawtrade/internal/config"
	"github.com/clawtrade/clawtrade/internal/engine"
	"github.com/clawtrade/clawtrade/internal/memory"
	"github.com/clawtrade/clawtrade/internal/security"
)

type Server struct {
	router   chi.Router
	cfg      *config.Config
	bus      *engine.EventBus
	hub      *Hub
	memory   *memory.Store
	audit    *security.AuditLog
	adapters map[string]adapter.TradingAdapter
}

func NewServer(cfg *config.Config, bus *engine.EventBus, mem *memory.Store, audit *security.AuditLog, adapters map[string]adapter.TradingAdapter) *Server {
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

	llm := NewLLMHandler(cfg)

	// WebSocket hub for real-time data
	hub := NewHub()
	go hub.Run()
	hub.SubscribeToEvents(bus, []string{
		"market.*",
		"trade.*",
		"risk.*",
		"system.*",
	})
	s.hub = hub

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/system/health", s.handleHealth)
		r.Get("/system/version", s.handleVersion)
		r.Post("/chat", llm.HandleChat)

		// Exchange data
		r.Get("/price", s.handleGetPrice)
		r.Get("/candles", s.handleGetCandles)
		r.Get("/orderbook", s.handleGetOrderBook)
		r.Get("/balances", s.handleGetBalances)
		r.Get("/positions", s.handleGetPositions)
		r.Get("/exchanges", s.handleListExchanges)
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
