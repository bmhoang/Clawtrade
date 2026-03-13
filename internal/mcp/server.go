package mcp

import (
	"encoding/json"
	"fmt"
	"sync"
)

// MCP JSON-RPC types following the Model Context Protocol spec

// JSONRPCRequest represents an incoming JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents an outgoing JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// RPCError represents a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Tool represents an MCP tool that can be invoked by clients.
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// Resource represents an MCP resource that can be read by clients.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ToolHandler handles tool execution and returns a result or error.
type ToolHandler func(args map[string]interface{}) (interface{}, error)

// Server implements the MCP server protocol, managing tools and resources.
type Server struct {
	mu        sync.RWMutex
	name      string
	version   string
	tools     map[string]Tool
	handlers  map[string]ToolHandler
	resources map[string]Resource
}

// NewServer creates a new MCP server with the given name and version.
func NewServer(name, version string) *Server {
	return &Server{
		name:      name,
		version:   version,
		tools:     make(map[string]Tool),
		handlers:  make(map[string]ToolHandler),
		resources: make(map[string]Resource),
	}
}

// RegisterTool adds a tool and its handler to the server.
func (s *Server) RegisterTool(tool Tool, handler ToolHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[tool.Name] = tool
	s.handlers[tool.Name] = handler
}

// UnregisterTool removes a tool from the server.
func (s *Server) UnregisterTool(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tools, name)
	delete(s.handlers, name)
}

// RegisterResource adds a resource to the server.
func (s *Server) RegisterResource(resource Resource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resources[resource.URI] = resource
}

// ListTools returns all registered tools.
func (s *Server) ListTools() []Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tools := make([]Tool, 0, len(s.tools))
	for _, t := range s.tools {
		tools = append(tools, t)
	}
	return tools
}

// ListResources returns all registered resources.
func (s *Server) ListResources() []Resource {
	s.mu.RLock()
	defer s.mu.RUnlock()
	resources := make([]Resource, 0, len(s.resources))
	for _, r := range s.resources {
		resources = append(resources, r)
	}
	return resources
}

// HandleMessage processes an incoming JSON-RPC message and returns a response.
func (s *Server) HandleMessage(data []byte) ([]byte, error) {
	var req JSONRPCRequest
	if err := json.Unmarshal(data, &req); err != nil {
		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      nil,
			Error: &RPCError{
				Code:    -32700,
				Message: "Parse error",
				Data:    err.Error(),
			},
		}
		return json.Marshal(resp)
	}

	var resp JSONRPCResponse
	resp.JSONRPC = "2.0"
	resp.ID = req.ID

	switch req.Method {
	case "initialize":
		resp.Result = s.handleInitialize()
	case "tools/list":
		resp.Result = s.handleToolsList()
	case "tools/call":
		result, rpcErr := s.handleToolsCall(req.Params)
		if rpcErr != nil {
			resp.Error = rpcErr
		} else {
			resp.Result = result
		}
	case "resources/list":
		resp.Result = s.handleResourcesList()
	case "resources/read":
		result, rpcErr := s.handleResourcesRead(req.Params)
		if rpcErr != nil {
			resp.Error = rpcErr
		} else {
			resp.Result = result
		}
	case "ping":
		resp.Result = map[string]interface{}{}
	default:
		resp.Error = &RPCError{
			Code:    -32601,
			Message: fmt.Sprintf("Method not found: %s", req.Method),
		}
	}

	return json.Marshal(resp)
}

func (s *Server) handleInitialize() interface{} {
	return map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools":     map[string]interface{}{},
			"resources": map[string]interface{}{},
		},
		"serverInfo": map[string]interface{}{
			"name":    s.name,
			"version": s.version,
		},
	}
}

func (s *Server) handleToolsList() interface{} {
	return map[string]interface{}{
		"tools": s.ListTools(),
	}
}

func (s *Server) handleToolsCall(params json.RawMessage) (interface{}, *RPCError) {
	var p struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if params != nil {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &RPCError{
				Code:    -32602,
				Message: "Invalid params",
				Data:    err.Error(),
			}
		}
	}

	s.mu.RLock()
	handler, ok := s.handlers[p.Name]
	s.mu.RUnlock()

	if !ok {
		return nil, &RPCError{
			Code:    -32602,
			Message: fmt.Sprintf("Unknown tool: %s", p.Name),
		}
	}

	args := p.Arguments
	if args == nil {
		args = make(map[string]interface{})
	}

	result, err := handler(args)
	if err != nil {
		return nil, &RPCError{
			Code:    -32000,
			Message: err.Error(),
		}
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": mustMarshal(result),
			},
		},
	}, nil
}

func (s *Server) handleResourcesList() interface{} {
	return map[string]interface{}{
		"resources": s.ListResources(),
	}
}

func (s *Server) handleResourcesRead(params json.RawMessage) (interface{}, *RPCError) {
	var p struct {
		URI string `json:"uri"`
	}
	if params != nil {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &RPCError{
				Code:    -32602,
				Message: "Invalid params",
				Data:    err.Error(),
			}
		}
	}

	s.mu.RLock()
	resource, ok := s.resources[p.URI]
	s.mu.RUnlock()

	if !ok {
		return nil, &RPCError{
			Code:    -32602,
			Message: fmt.Sprintf("Unknown resource: %s", p.URI),
		}
	}

	return map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"uri":      resource.URI,
				"mimeType": resource.MimeType,
				"text":     fmt.Sprintf("Resource: %s", resource.Name),
			},
		},
	}, nil
}

func mustMarshal(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// RegisterClawtradeTools registers built-in Clawtrade tools.
func (s *Server) RegisterClawtradeTools() {
	s.RegisterTool(Tool{
		Name:        "get_portfolio",
		Description: "Get current portfolio summary including balance and PnL",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, func(args map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{
			"balance":        10000,
			"unrealized_pnl": 0,
			"status":         "paper_trading",
		}, nil
	})

	s.RegisterTool(Tool{
		Name:        "get_positions",
		Description: "Get current open positions",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, func(args map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{
			"positions": []interface{}{},
		}, nil
	})

	s.RegisterTool(Tool{
		Name:        "get_price",
		Description: "Get current price for a symbol",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"symbol": map[string]interface{}{
					"type":        "string",
					"description": "Trading symbol (e.g. BTC-USD)",
				},
			},
			"required": []string{"symbol"},
		},
	}, func(args map[string]interface{}) (interface{}, error) {
		symbol, _ := args["symbol"].(string)
		if symbol == "" {
			return nil, fmt.Errorf("symbol is required")
		}
		return map[string]interface{}{
			"symbol": symbol,
			"price":  0,
			"source": "paper",
		}, nil
	})

	s.RegisterTool(Tool{
		Name:        "place_order",
		Description: "Place a trading order",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"symbol": map[string]interface{}{
					"type":        "string",
					"description": "Trading symbol",
				},
				"side": map[string]interface{}{
					"type":        "string",
					"description": "Order side: buy or sell",
					"enum":        []string{"buy", "sell"},
				},
				"quantity": map[string]interface{}{
					"type":        "number",
					"description": "Order quantity",
				},
			},
			"required": []string{"symbol", "side", "quantity"},
		},
	}, func(args map[string]interface{}) (interface{}, error) {
		symbol, _ := args["symbol"].(string)
		side, _ := args["side"].(string)
		quantity, _ := args["quantity"].(float64)
		if symbol == "" || side == "" || quantity == 0 {
			return nil, fmt.Errorf("symbol, side, and quantity are required")
		}
		return map[string]interface{}{
			"order_id": "paper-001",
			"symbol":   symbol,
			"side":     side,
			"quantity": quantity,
			"status":   "filled",
			"mode":     "paper",
		}, nil
	})

	s.RegisterTool(Tool{
		Name:        "get_memory",
		Description: "Query agent memory and past learnings",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Search query for memory",
				},
			},
		},
	}, func(args map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{
			"memories": []interface{}{},
			"count":    0,
		}, nil
	})

	s.RegisterTool(Tool{
		Name:        "analyze_symbol",
		Description: "Run technical analysis on a trading symbol",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"symbol": map[string]interface{}{
					"type":        "string",
					"description": "Trading symbol to analyze",
				},
			},
			"required": []string{"symbol"},
		},
	}, func(args map[string]interface{}) (interface{}, error) {
		symbol, _ := args["symbol"].(string)
		if symbol == "" {
			return nil, fmt.Errorf("symbol is required")
		}
		return map[string]interface{}{
			"symbol":     symbol,
			"trend":      "neutral",
			"confidence": 0.5,
			"signals":    []interface{}{},
		}, nil
	})

	// Register resources
	s.RegisterResource(Resource{
		URI:         "clawtrade://portfolio",
		Name:        "Portfolio",
		Description: "Current portfolio state",
		MimeType:    "application/json",
	})

	s.RegisterResource(Resource{
		URI:         "clawtrade://positions",
		Name:        "Positions",
		Description: "Current open positions",
		MimeType:    "application/json",
	})

	s.RegisterResource(Resource{
		URI:         "clawtrade://memory",
		Name:        "Memory",
		Description: "Agent memory and learnings",
		MimeType:    "application/json",
	})
}
