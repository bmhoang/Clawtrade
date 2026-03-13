package clawtrade

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		json.NewEncoder(w).Encode(HealthResponse{Status: "ok", Version: "1.0.0"})
	}))
	defer srv.Close()

	client := NewClient(Config{BaseURL: srv.URL})
	resp, err := client.Health()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", resp.Status)
	}
	if resp.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", resp.Version)
	}
}

func TestGetPortfolio(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/portfolio" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(PortfolioResponse{
			Balance:       10000.50,
			UnrealizedPnL: 250.75,
			TodayPnL:      -50.25,
		})
	}))
	defer srv.Close()

	client := NewClient(Config{BaseURL: srv.URL})
	resp, err := client.GetPortfolio()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Balance != 10000.50 {
		t.Errorf("expected balance 10000.50, got %f", resp.Balance)
	}
	if resp.UnrealizedPnL != 250.75 {
		t.Errorf("expected unrealized PnL 250.75, got %f", resp.UnrealizedPnL)
	}
	if resp.TodayPnL != -50.25 {
		t.Errorf("expected today PnL -50.25, got %f", resp.TodayPnL)
	}
}

func TestGetPositions(t *testing.T) {
	positions := []Position{
		{Symbol: "BTC-USD", Side: "long", Size: 0.5, Entry: 40000, Current: 42000, PnL: 1000},
		{Symbol: "ETH-USD", Side: "short", Size: 10, Entry: 3000, Current: 2900, PnL: 1000},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/positions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(positions)
	}))
	defer srv.Close()

	client := NewClient(Config{BaseURL: srv.URL})
	resp, err := client.GetPositions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("expected 2 positions, got %d", len(resp))
	}
	if resp[0].Symbol != "BTC-USD" {
		t.Errorf("expected symbol 'BTC-USD', got %q", resp[0].Symbol)
	}
	if resp[1].Side != "short" {
		t.Errorf("expected side 'short', got %q", resp[1].Side)
	}
}

func TestAPIKeyHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key-123" {
			t.Errorf("expected 'Bearer test-key-123', got %q", auth)
		}
		json.NewEncoder(w).Encode(HealthResponse{Status: "ok", Version: "1.0.0"})
	}))
	defer srv.Close()

	client := NewClient(Config{BaseURL: srv.URL, APIKey: "test-key-123"})
	_, err := client.Health()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestErrorOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer srv.Close()

	client := NewClient(Config{BaseURL: srv.URL})
	_, err := client.Health()
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
	expected := "HTTP 500: internal server error"
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}
}
