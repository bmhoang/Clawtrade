// internal/mcp/client.go
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"sync/atomic"
)

// Client connects to an external MCP server via stdio (JSON-RPC over stdin/stdout).
type Client struct {
	name    string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	mu      sync.Mutex
	nextID  atomic.Int64
	tools   []Tool
	running bool
}

// NewClient creates a new MCP client that spawns the given command.
func NewClient(name, command string, args []string, env []string) *Client {
	return &Client{
		name: name,
		cmd:  newCmd(command, args, env),
	}
}

func newCmd(command string, args []string, env []string) *exec.Cmd {
	cmd := exec.Command(command, args...)
	if len(env) > 0 {
		cmd.Env = append(cmd.Environ(), env...)
	}
	return cmd
}

// Start spawns the MCP server process and performs initialization.
func (c *Client) Start() error {
	var err error
	c.stdin, err = c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("mcp client %s: stdin pipe: %w", c.name, err)
	}

	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("mcp client %s: stdout pipe: %w", c.name, err)
	}
	c.stdout = bufio.NewReader(stdout)

	// Capture stderr for debugging
	stderr, _ := c.cmd.StderrPipe()
	go func() {
		if stderr == nil {
			return
		}
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			log.Printf("mcp[%s] stderr: %s", c.name, scanner.Text())
		}
	}()

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("mcp client %s: start: %w", c.name, err)
	}
	c.running = true

	// Initialize
	_, err = c.call("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "clawtrade",
			"version": "0.4.0",
		},
	})
	if err != nil {
		c.Stop()
		return fmt.Errorf("mcp client %s: initialize: %w", c.name, err)
	}

	// Discover tools
	result, err := c.call("tools/list", nil)
	if err != nil {
		c.Stop()
		return fmt.Errorf("mcp client %s: tools/list: %w", c.name, err)
	}

	// Parse tools from result
	var toolsResult struct {
		Tools []Tool `json:"tools"`
	}
	raw, _ := json.Marshal(result)
	if err := json.Unmarshal(raw, &toolsResult); err == nil {
		c.tools = toolsResult.Tools
	}

	log.Printf("mcp[%s]: connected, %d tools available", c.name, len(c.tools))
	return nil
}

// Stop kills the MCP server process.
func (c *Client) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stdin != nil {
		c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
	}
	c.running = false
}

// Name returns the server name.
func (c *Client) Name() string {
	return c.name
}

// Tools returns the discovered tools from this MCP server.
func (c *Client) Tools() []Tool {
	return c.tools
}

// IsRunning returns whether the MCP server process is alive.
func (c *Client) IsRunning() bool {
	return c.running
}

// CallTool invokes a tool on the MCP server and returns the text result.
func (c *Client) CallTool(name string, args map[string]any) (string, error) {
	result, err := c.call("tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return "", err
	}

	// Extract text content from MCP response
	raw, _ := json.Marshal(result)
	var callResult struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &callResult); err == nil && len(callResult.Content) > 0 {
		return callResult.Content[0].Text, nil
	}

	// Fallback: return raw JSON
	return string(raw), nil
}

// call sends a JSON-RPC request and reads the response.
func (c *Client) call(method string, params any) (any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil, fmt.Errorf("mcp server %s is not running", c.name)
	}

	id := c.nextID.Add(1)

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
	}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("marshal params: %w", err)
		}
		req.Params = raw
	}

	// Write request as a single line (JSON-RPC over stdio uses newline-delimited JSON)
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	data = append(data, '\n')

	if _, err := c.stdin.Write(data); err != nil {
		c.running = false
		return nil, fmt.Errorf("write to mcp server: %w", err)
	}

	// Read response line
	line, err := c.stdout.ReadBytes('\n')
	if err != nil {
		c.running = false
		return nil, fmt.Errorf("read from mcp server: %w", err)
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("mcp error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Result, nil
}

// ClientManager manages multiple MCP client connections.
type ClientManager struct {
	mu      sync.RWMutex
	clients map[string]*Client
}

// NewClientManager creates a new manager.
func NewClientManager() *ClientManager {
	return &ClientManager{
		clients: make(map[string]*Client),
	}
}

// Add registers and starts an MCP client.
func (cm *ClientManager) Add(client *Client) error {
	if err := client.Start(); err != nil {
		return err
	}
	cm.mu.Lock()
	cm.clients[client.Name()] = client
	cm.mu.Unlock()
	return nil
}

// GetAllTools returns all tools from all connected MCP servers, prefixed with server name.
func (cm *ClientManager) GetAllTools() []Tool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	var all []Tool
	for _, c := range cm.clients {
		for _, t := range c.Tools() {
			// Prefix tool name with server name to avoid collisions
			prefixed := Tool{
				Name:        c.Name() + ":" + t.Name,
				Description: fmt.Sprintf("[%s] %s", c.Name(), t.Description),
				InputSchema: t.InputSchema,
			}
			all = append(all, prefixed)
		}
	}
	return all
}

// CallTool routes a prefixed tool call to the correct MCP server.
func (cm *ClientManager) CallTool(prefixedName string, args map[string]any) (string, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Parse "serverName:toolName"
	for serverName, client := range cm.clients {
		prefix := serverName + ":"
		if len(prefixedName) > len(prefix) && prefixedName[:len(prefix)] == prefix {
			toolName := prefixedName[len(prefix):]
			return client.CallTool(toolName, args)
		}
	}
	return "", fmt.Errorf("no MCP server found for tool: %s", prefixedName)
}

// StopAll stops all MCP server connections.
func (cm *ClientManager) StopAll() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	for _, c := range cm.clients {
		c.Stop()
	}
	cm.clients = make(map[string]*Client)
}

// ServerCount returns the number of connected MCP servers.
func (cm *ClientManager) ServerCount() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return len(cm.clients)
}
