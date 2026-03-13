package mcp

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
)

func TestNewServer(t *testing.T) {
	s := NewServer("clawtrade", "1.0.0")
	if s.name != "clawtrade" {
		t.Errorf("expected name 'clawtrade', got '%s'", s.name)
	}
	if s.version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got '%s'", s.version)
	}
	if s.tools == nil || s.handlers == nil || s.resources == nil {
		t.Error("maps should be initialized")
	}
}

func TestRegisterTool(t *testing.T) {
	s := NewServer("test", "1.0")
	tool := Tool{
		Name:        "test_tool",
		Description: "A test tool",
		InputSchema: map[string]interface{}{"type": "object"},
	}
	handler := func(args map[string]interface{}) (interface{}, error) {
		return "ok", nil
	}
	s.RegisterTool(tool, handler)

	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.tools["test_tool"]; !ok {
		t.Error("tool should be registered")
	}
	if _, ok := s.handlers["test_tool"]; !ok {
		t.Error("handler should be registered")
	}
}

func TestUnregisterTool(t *testing.T) {
	s := NewServer("test", "1.0")
	tool := Tool{Name: "removeme", Description: "temp", InputSchema: map[string]interface{}{}}
	s.RegisterTool(tool, func(args map[string]interface{}) (interface{}, error) { return nil, nil })

	s.UnregisterTool("removeme")

	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.tools["removeme"]; ok {
		t.Error("tool should be unregistered")
	}
	if _, ok := s.handlers["removeme"]; ok {
		t.Error("handler should be unregistered")
	}
}

func TestRegisterResource(t *testing.T) {
	s := NewServer("test", "1.0")
	res := Resource{
		URI:         "test://resource",
		Name:        "Test Resource",
		Description: "A test resource",
		MimeType:    "application/json",
	}
	s.RegisterResource(res)

	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.resources["test://resource"]; !ok {
		t.Error("resource should be registered")
	}
}

func TestListTools(t *testing.T) {
	s := NewServer("test", "1.0")
	s.RegisterTool(Tool{Name: "a", Description: "tool a", InputSchema: map[string]interface{}{}},
		func(args map[string]interface{}) (interface{}, error) { return nil, nil })
	s.RegisterTool(Tool{Name: "b", Description: "tool b", InputSchema: map[string]interface{}{}},
		func(args map[string]interface{}) (interface{}, error) { return nil, nil })

	tools := s.ListTools()
	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}
}

func TestHandleMessageInitialize(t *testing.T) {
	s := NewServer("clawtrade", "1.0.0")
	req := `{"jsonrpc":"2.0","id":1,"method":"initialize"}`
	resp, err := s.HandleMessage([]byte(req))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var r JSONRPCResponse
	if err := json.Unmarshal(resp, &r); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if r.Error != nil {
		t.Fatalf("unexpected error in response: %v", r.Error)
	}

	result, ok := r.Result.(map[string]interface{})
	if !ok {
		t.Fatal("result should be a map")
	}
	serverInfo, ok := result["serverInfo"].(map[string]interface{})
	if !ok {
		t.Fatal("serverInfo should be a map")
	}
	if serverInfo["name"] != "clawtrade" {
		t.Errorf("expected server name 'clawtrade', got '%v'", serverInfo["name"])
	}
	if serverInfo["version"] != "1.0.0" {
		t.Errorf("expected version '1.0.0', got '%v'", serverInfo["version"])
	}
}

func TestHandleMessageToolsList(t *testing.T) {
	s := NewServer("test", "1.0")
	s.RegisterTool(Tool{Name: "my_tool", Description: "desc", InputSchema: map[string]interface{}{}},
		func(args map[string]interface{}) (interface{}, error) { return nil, nil })

	req := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
	resp, err := s.HandleMessage([]byte(req))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var r JSONRPCResponse
	json.Unmarshal(resp, &r)
	if r.Error != nil {
		t.Fatalf("unexpected error: %v", r.Error)
	}

	result := r.Result.(map[string]interface{})
	tools := result["tools"].([]interface{})
	if len(tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(tools))
	}
}

func TestHandleMessageToolsCall(t *testing.T) {
	s := NewServer("test", "1.0")
	s.RegisterTool(Tool{Name: "echo", Description: "echoes input", InputSchema: map[string]interface{}{}},
		func(args map[string]interface{}) (interface{}, error) {
			return map[string]interface{}{"echoed": args["msg"]}, nil
		})

	req := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"msg":"hello"}}}`
	resp, err := s.HandleMessage([]byte(req))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var r JSONRPCResponse
	json.Unmarshal(resp, &r)
	if r.Error != nil {
		t.Fatalf("unexpected error: %v", r.Error)
	}

	result := r.Result.(map[string]interface{})
	content := result["content"].([]interface{})
	if len(content) == 0 {
		t.Fatal("expected content in response")
	}
	item := content[0].(map[string]interface{})
	if item["type"] != "text" {
		t.Errorf("expected type 'text', got '%v'", item["type"])
	}
	text := item["text"].(string)
	if text == "" {
		t.Error("expected non-empty text")
	}
}

func TestHandleMessageToolsCallUnknown(t *testing.T) {
	s := NewServer("test", "1.0")
	req := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"nonexistent"}}`
	resp, err := s.HandleMessage([]byte(req))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var r JSONRPCResponse
	json.Unmarshal(resp, &r)
	if r.Error == nil {
		t.Fatal("expected error for unknown tool")
	}
	if r.Error.Code != -32602 {
		t.Errorf("expected code -32602, got %d", r.Error.Code)
	}
}

func TestHandleMessageResourcesList(t *testing.T) {
	s := NewServer("test", "1.0")
	s.RegisterResource(Resource{URI: "test://r1", Name: "R1"})

	req := `{"jsonrpc":"2.0","id":5,"method":"resources/list"}`
	resp, err := s.HandleMessage([]byte(req))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var r JSONRPCResponse
	json.Unmarshal(resp, &r)
	if r.Error != nil {
		t.Fatalf("unexpected error: %v", r.Error)
	}

	result := r.Result.(map[string]interface{})
	resources := result["resources"].([]interface{})
	if len(resources) != 1 {
		t.Errorf("expected 1 resource, got %d", len(resources))
	}
}

func TestHandleMessagePing(t *testing.T) {
	s := NewServer("test", "1.0")
	req := `{"jsonrpc":"2.0","id":6,"method":"ping"}`
	resp, err := s.HandleMessage([]byte(req))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var r JSONRPCResponse
	json.Unmarshal(resp, &r)
	if r.Error != nil {
		t.Fatalf("unexpected error: %v", r.Error)
	}
	if r.Result == nil {
		t.Error("expected non-nil result for ping")
	}
}

func TestHandleMessageUnknownMethod(t *testing.T) {
	s := NewServer("test", "1.0")
	req := `{"jsonrpc":"2.0","id":7,"method":"unknown/method"}`
	resp, err := s.HandleMessage([]byte(req))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var r JSONRPCResponse
	json.Unmarshal(resp, &r)
	if r.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if r.Error.Code != -32601 {
		t.Errorf("expected code -32601, got %d", r.Error.Code)
	}
}

func TestHandleMessageInvalidJSON(t *testing.T) {
	s := NewServer("test", "1.0")
	resp, err := s.HandleMessage([]byte("not json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var r JSONRPCResponse
	json.Unmarshal(resp, &r)
	if r.Error == nil {
		t.Fatal("expected parse error")
	}
	if r.Error.Code != -32700 {
		t.Errorf("expected code -32700, got %d", r.Error.Code)
	}
}

func TestRegisterClawtradeTools(t *testing.T) {
	s := NewServer("clawtrade", "1.0.0")
	s.RegisterClawtradeTools()

	tools := s.ListTools()
	expectedTools := map[string]bool{
		"get_portfolio":  false,
		"get_positions":  false,
		"get_price":      false,
		"place_order":    false,
		"get_memory":     false,
		"analyze_symbol": false,
	}

	for _, tool := range tools {
		if _, ok := expectedTools[tool.Name]; ok {
			expectedTools[tool.Name] = true
		}
	}

	for name, found := range expectedTools {
		if !found {
			t.Errorf("expected tool '%s' to be registered", name)
		}
	}

	resources := s.ListResources()
	if len(resources) < 3 {
		t.Errorf("expected at least 3 resources, got %d", len(resources))
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := NewServer("test", "1.0")
	var wg sync.WaitGroup
	const n = 100

	// Concurrent tool registration
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("tool_%d", i)
			s.RegisterTool(Tool{Name: name, Description: "concurrent", InputSchema: map[string]interface{}{}},
				func(args map[string]interface{}) (interface{}, error) { return "ok", nil })
		}(i)
	}
	wg.Wait()

	tools := s.ListTools()
	if len(tools) != n {
		t.Errorf("expected %d tools, got %d", n, len(tools))
	}

	// Concurrent reads and writes
	for i := 0; i < n; i++ {
		wg.Add(3)
		go func(i int) {
			defer wg.Done()
			s.ListTools()
		}(i)
		go func(i int) {
			defer wg.Done()
			s.ListResources()
		}(i)
		go func(i int) {
			defer wg.Done()
			req := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
			s.HandleMessage([]byte(req))
		}(i)
	}
	wg.Wait()

	// Concurrent unregister
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s.UnregisterTool(fmt.Sprintf("tool_%d", i))
		}(i)
	}
	wg.Wait()

	tools = s.ListTools()
	if len(tools) != 0 {
		t.Errorf("expected 0 tools after unregister, got %d", len(tools))
	}
}

func TestHandleMessageResourcesRead(t *testing.T) {
	s := NewServer("test", "1.0")
	s.RegisterResource(Resource{
		URI:      "test://data",
		Name:     "Test Data",
		MimeType: "application/json",
	})

	req := `{"jsonrpc":"2.0","id":8,"method":"resources/read","params":{"uri":"test://data"}}`
	resp, err := s.HandleMessage([]byte(req))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var r JSONRPCResponse
	json.Unmarshal(resp, &r)
	if r.Error != nil {
		t.Fatalf("unexpected error: %v", r.Error)
	}

	result := r.Result.(map[string]interface{})
	contents := result["contents"].([]interface{})
	if len(contents) != 1 {
		t.Errorf("expected 1 content entry, got %d", len(contents))
	}
}
