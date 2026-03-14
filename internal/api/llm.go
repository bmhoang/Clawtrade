package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/clawtrade/clawtrade/internal/agent"
)

// ChatRequest is the JSON body from the frontend.
type ChatRequest struct {
	Messages []ChatMessage `json:"messages"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatResponse is what we return to the frontend.
type ChatResponse struct {
	Content   string            `json:"content"`
	Model     string            `json:"model"`
	ToolsUsed []agent.ToolCall   `json:"tools_used,omitempty"`
	Usage     *ChatUsage        `json:"usage,omitempty"`
}

type ChatUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// LLMHandler proxies chat requests through the AI agent.
type LLMHandler struct {
	agent *agent.Agent
}

func NewLLMHandler(ag *agent.Agent) *LLMHandler {
	return &LLMHandler{agent: ag}
}

func (h *LLMHandler) HandleChat(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if len(req.Messages) == 0 {
		http.Error(w, `{"error":"messages required"}`, http.StatusBadRequest)
		return
	}

	// Convert to agent messages
	var msgs []agent.Message
	for _, m := range req.Messages {
		msgs = append(msgs, agent.Message{Role: m.Role, Content: m.Content})
	}

	// Run the agent (context injection + tool use loop)
	resp, err := h.agent.Run(r.Context(), msgs)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadGateway)
		return
	}

	chatResp := ChatResponse{
		Content:   resp.Content,
		Model:     resp.Model,
		ToolsUsed: resp.ToolsUsed,
	}
	if resp.Usage != nil {
		chatResp.Usage = &ChatUsage{
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(chatResp)
}
