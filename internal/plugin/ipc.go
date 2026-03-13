package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// JSON-RPC 2.0 types
type RPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type RPCResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      *int      `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type MethodHandler func(params json.RawMessage) (any, error)

// Bridge manages JSON-RPC communication with a subprocess
type Bridge struct {
	mu        sync.RWMutex
	methods   map[string]MethodHandler
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Scanner
	pending   map[int]chan *RPCResponse
	nextID    int
	pendingMu sync.Mutex
}

func NewBridge() *Bridge {
	return &Bridge{
		methods: make(map[string]MethodHandler),
		pending: make(map[int]chan *RPCResponse),
	}
}

// RegisterMethod registers a handler for a JSON-RPC method
func (b *Bridge) RegisterMethod(name string, handler MethodHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.methods[name] = handler
}

// GetMethod returns a registered method handler
func (b *Bridge) GetMethod(name string) (MethodHandler, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	h, ok := b.methods[name]
	return h, ok
}

// HandleRequest processes an incoming JSON-RPC request and returns a response
func (b *Bridge) HandleRequest(data []byte) (*RPCResponse, error) {
	var req RPCRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return &RPCResponse{
			JSONRPC: "2.0",
			Error:   &RPCError{Code: -32700, Message: "Parse error"},
		}, nil
	}

	if req.JSONRPC != "2.0" {
		return &RPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32600, Message: "Invalid Request"},
		}, nil
	}

	handler, ok := b.GetMethod(req.Method)
	if !ok {
		return &RPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32601, Message: fmt.Sprintf("Method not found: %s", req.Method)},
		}, nil
	}

	result, err := handler(req.Params)
	if err != nil {
		return &RPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32000, Message: err.Error()},
		}, nil
	}

	return &RPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}, nil
}

// SpawnProcess starts a subprocess and connects stdin/stdout for JSON-RPC
func (b *Bridge) SpawnProcess(ctx context.Context, command string, args ...string) error {
	b.cmd = exec.CommandContext(ctx, command, args...)

	var err error
	b.stdin, err = b.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdoutPipe, err := b.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	b.stdout = bufio.NewScanner(stdoutPipe)

	if err := b.cmd.Start(); err != nil {
		return fmt.Errorf("start process: %w", err)
	}

	// Read responses from subprocess in background
	go b.readLoop()

	return nil
}

func (b *Bridge) readLoop() {
	for b.stdout.Scan() {
		line := b.stdout.Bytes()

		// Try as response first (has result or error)
		var resp RPCResponse
		if err := json.Unmarshal(line, &resp); err == nil && resp.ID != nil {
			b.pendingMu.Lock()
			if ch, ok := b.pending[*resp.ID]; ok {
				ch <- &resp
				delete(b.pending, *resp.ID)
			}
			b.pendingMu.Unlock()
			continue
		}

		// Try as incoming request
		response, err := b.HandleRequest(line)
		if err == nil && response != nil {
			data, _ := json.Marshal(response)
			b.stdin.Write(append(data, '\n'))
		}
	}
}

// Call sends a JSON-RPC request to the subprocess and waits for a response
func (b *Bridge) Call(ctx context.Context, method string, params any) (*RPCResponse, error) {
	b.pendingMu.Lock()
	id := b.nextID
	b.nextID++
	ch := make(chan *RPCResponse, 1)
	b.pending[id] = ch
	b.pendingMu.Unlock()

	var rawParams json.RawMessage
	if params != nil {
		var err error
		rawParams, err = json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("marshal params: %w", err)
		}
	}

	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  method,
		Params:  rawParams,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	if _, err := b.stdin.Write(append(data, '\n')); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	select {
	case resp := <-ch:
		return resp, nil
	case <-ctx.Done():
		b.pendingMu.Lock()
		delete(b.pending, id)
		b.pendingMu.Unlock()
		return nil, ctx.Err()
	}
}

// Wait waits for the subprocess to exit
func (b *Bridge) Wait() error {
	if b.cmd == nil {
		return nil
	}
	return b.cmd.Wait()
}

// Close closes the bridge
func (b *Bridge) Close() error {
	if b.stdin != nil {
		b.stdin.Close()
	}
	return nil
}
