// internal/agent/mcpbridge.go
package agent

import (
	"github.com/clawtrade/clawtrade/internal/mcp"
)

// mcpClientBridge adapts mcp.ClientManager to the MCPBridge interface.
type mcpClientBridge struct {
	manager *mcp.ClientManager
}

// NewMCPBridge wraps a ClientManager as an MCPBridge for the tool registry.
func NewMCPBridge(manager *mcp.ClientManager) MCPBridge {
	if manager == nil {
		return nil
	}
	return &mcpClientBridge{manager: manager}
}

func (b *mcpClientBridge) GetAllTools() []struct {
	Name        string
	Description string
	InputSchema map[string]any
} {
	mcpTools := b.manager.GetAllTools()
	result := make([]struct {
		Name        string
		Description string
		InputSchema map[string]any
	}, len(mcpTools))
	for i, t := range mcpTools {
		schema := make(map[string]any)
		for k, v := range t.InputSchema {
			schema[k] = v
		}
		result[i] = struct {
			Name        string
			Description string
			InputSchema map[string]any
		}{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schema,
		}
	}
	return result
}

func (b *mcpClientBridge) CallTool(name string, args map[string]any) (string, error) {
	return b.manager.CallTool(name, args)
}
