package clawtrade

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Config holds client configuration.
type Config struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
}

// Client is the Clawtrade API client.
type Client struct {
	config Config
	http   *http.Client
}

// NewClient creates a new Clawtrade client with the given configuration.
func NewClient(config Config) *Client {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	return &Client{
		config: config,
		http:   &http.Client{Timeout: config.Timeout},
	}
}

// HealthResponse represents the API health check response.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// PortfolioResponse represents the portfolio summary.
type PortfolioResponse struct {
	Balance       float64 `json:"balance"`
	UnrealizedPnL float64 `json:"unrealized_pnl"`
	TodayPnL      float64 `json:"today_pnl"`
}

// Position represents a single trading position.
type Position struct {
	Symbol  string  `json:"symbol"`
	Side    string  `json:"side"`
	Size    float64 `json:"size"`
	Entry   float64 `json:"entry"`
	Current float64 `json:"current"`
	PnL     float64 `json:"pnl"`
}

// ChatRequest represents a chat message to the AI.
type ChatRequest struct {
	Message string `json:"message"`
}

// ChatResponse represents the AI chat response.
type ChatResponse struct {
	Response string `json:"response"`
	Model    string `json:"model"`
}

// OrderRequest represents a new order to place.
type OrderRequest struct {
	Symbol   string  `json:"symbol"`
	Side     string  `json:"side"`
	Quantity float64 `json:"quantity"`
	Price    float64 `json:"price,omitempty"`
	Type     string  `json:"type"`
}

// OrderResponse represents the result of placing an order.
type OrderResponse struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
}

// PriceResponse represents a price lookup result.
type PriceResponse struct {
	Symbol string  `json:"symbol"`
	Price  float64 `json:"price"`
}

// CancelResponse represents the result of cancelling an order.
type CancelResponse struct {
	Success bool `json:"success"`
}

// Health checks the API health status.
func (c *Client) Health() (*HealthResponse, error) {
	var resp HealthResponse
	err := c.get("/api/health", &resp)
	return &resp, err
}

// GetPortfolio returns the current portfolio summary.
func (c *Client) GetPortfolio() (*PortfolioResponse, error) {
	var resp PortfolioResponse
	err := c.get("/api/portfolio", &resp)
	return &resp, err
}

// GetPositions returns all open positions.
func (c *Client) GetPositions() ([]Position, error) {
	var resp []Position
	err := c.get("/api/positions", &resp)
	return resp, err
}

// Chat sends a message to the AI trading assistant.
func (c *Client) Chat(message string) (*ChatResponse, error) {
	var resp ChatResponse
	err := c.post("/api/chat", ChatRequest{Message: message}, &resp)
	return &resp, err
}

// PlaceOrder places a new order.
func (c *Client) PlaceOrder(order OrderRequest) (*OrderResponse, error) {
	var resp OrderResponse
	err := c.post("/api/orders", order, &resp)
	return &resp, err
}

// CancelOrder cancels an existing order.
func (c *Client) CancelOrder(orderID string) (*CancelResponse, error) {
	var resp CancelResponse
	err := c.doDelete("/api/orders/"+orderID, &resp)
	return &resp, err
}

// GetPrice returns the current price for a symbol.
func (c *Client) GetPrice(symbol string) (*PriceResponse, error) {
	var resp PriceResponse
	err := c.get("/api/price/"+symbol, &resp)
	return &resp, err
}

func (c *Client) get(path string, result interface{}) error {
	req, err := http.NewRequest("GET", c.config.BaseURL+path, nil)
	if err != nil {
		return err
	}
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}
	return json.NewDecoder(resp.Body).Decode(result)
}

func (c *Client) post(path string, body, result interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", c.config.BaseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}
	return json.NewDecoder(resp.Body).Decode(result)
}

func (c *Client) doDelete(path string, result interface{}) error {
	req, err := http.NewRequest("DELETE", c.config.BaseURL+path, nil)
	if err != nil {
		return err
	}
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}
	return json.NewDecoder(resp.Body).Decode(result)
}
