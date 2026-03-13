package plugin

import (
	"encoding/json"
	"testing"
)

func TestBridge_RegisterAndHandleMethod(t *testing.T) {
	b := NewBridge()

	b.RegisterMethod("echo", func(params json.RawMessage) (any, error) {
		var msg string
		json.Unmarshal(params, &msg)
		return map[string]string{"echo": msg}, nil
	})

	id := 1
	req, _ := json.Marshal(RPCRequest{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "echo",
		Params:  json.RawMessage(`"hello"`),
	})

	resp, err := b.HandleRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("unexpected RPC error: %s", resp.Error.Message)
	}

	result, _ := json.Marshal(resp.Result)
	expected := `{"echo":"hello"}`
	if string(result) != expected {
		t.Errorf("expected %s, got %s", expected, string(result))
	}
}

func TestBridge_MethodNotFound(t *testing.T) {
	b := NewBridge()

	id := 1
	req, _ := json.Marshal(RPCRequest{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "nonexistent",
	})

	resp, err := b.HandleRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}

	if resp.Error.Code != -32601 {
		t.Errorf("expected code -32601, got %d", resp.Error.Code)
	}
}

func TestBridge_InvalidJSON(t *testing.T) {
	b := NewBridge()

	resp, err := b.HandleRequest([]byte(`{invalid json`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected parse error")
	}

	if resp.Error.Code != -32700 {
		t.Errorf("expected code -32700, got %d", resp.Error.Code)
	}
}
