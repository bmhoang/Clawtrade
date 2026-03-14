package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/clawtrade/clawtrade/internal/config"
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
	Content string `json:"content"`
	Model   string `json:"model"`
	Usage   *Usage `json:"usage,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// LLMHandler holds the config needed to proxy LLM requests.
type LLMHandler struct {
	cfg *config.Config
}

func NewLLMHandler(cfg *config.Config) *LLMHandler {
	return &LLMHandler{cfg: cfg}
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

	provider := h.cfg.Agent.Model.Provider()
	apiKey := h.cfg.Agent.Model.ResolveAPIKey()
	model := h.cfg.Agent.Model.ModelName()

	if provider == "" || model == "" {
		http.Error(w, `{"error":"no LLM model configured. Run: clawtrade models setup"}`, http.StatusServiceUnavailable)
		return
	}
	if apiKey == "" && provider != "ollama" {
		http.Error(w, fmt.Sprintf(`{"error":"no API key found for %s. Set env var or run: clawtrade models setup"}`, provider), http.StatusServiceUnavailable)
		return
	}

	var resp *ChatResponse
	var err error

	switch provider {
	case "anthropic":
		resp, err = h.callAnthropic(apiKey, model, req.Messages)
	case "openai", "deepseek":
		baseURL := "https://api.openai.com/v1"
		if provider == "deepseek" {
			baseURL = "https://api.deepseek.com/v1"
		}
		resp, err = h.callOpenAICompatible(baseURL, apiKey, model, req.Messages)
	case "openrouter":
		resp, err = h.callOpenAICompatible("https://openrouter.ai/api/v1", apiKey, model, req.Messages)
	case "google":
		resp, err = h.callGoogle(apiKey, model, req.Messages)
	case "ollama":
		resp, err = h.callOpenAICompatible("http://localhost:11434/v1", apiKey, model, req.Messages)
	default:
		http.Error(w, fmt.Sprintf(`{"error":"unsupported provider: %s"}`, provider), http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ─── Anthropic (Claude) ─────────────────────────────────────────────

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Model string `json:"model"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (h *LLMHandler) callAnthropic(apiKey, model string, messages []ChatMessage) (*ChatResponse, error) {
	var system string
	var msgs []anthropicMessage
	for _, m := range messages {
		if m.Role == "system" {
			system = m.Content
			continue
		}
		msgs = append(msgs, anthropicMessage{Role: m.Role, Content: m.Content})
	}

	// Prepend system prompt for trading context
	if system == "" {
		system = "You are Clawtrade AI, an expert trading copilot. Help users with market analysis, strategy, risk management, and trade execution. Be concise and data-driven."
	}

	body := anthropicRequest{
		Model:     model,
		MaxTokens: h.cfg.Agent.Model.MaxTokens,
		System:    system,
		Messages:  msgs,
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("anthropic API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var ar anthropicResponse
	if err := json.Unmarshal(respBody, &ar); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if ar.Error != nil {
		return nil, fmt.Errorf("anthropic: %s", ar.Error.Message)
	}

	content := ""
	if len(ar.Content) > 0 {
		content = ar.Content[0].Text
	}

	return &ChatResponse{
		Content: content,
		Model:   ar.Model,
		Usage:   &Usage{InputTokens: ar.Usage.InputTokens, OutputTokens: ar.Usage.OutputTokens},
	}, nil
}

// ─── OpenAI-compatible (OpenAI, DeepSeek, OpenRouter, Ollama) ───────

type openaiRequest struct {
	Model       string           `json:"model"`
	Messages    []openaiMessage  `json:"messages"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
}

type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (h *LLMHandler) callOpenAICompatible(baseURL, apiKey, model string, messages []ChatMessage) (*ChatResponse, error) {
	var msgs []openaiMessage

	// Add system prompt if not present
	hasSystem := false
	for _, m := range messages {
		if m.Role == "system" {
			hasSystem = true
			break
		}
	}
	if !hasSystem {
		msgs = append(msgs, openaiMessage{
			Role:    "system",
			Content: "You are Clawtrade AI, an expert trading copilot. Help users with market analysis, strategy, risk management, and trade execution. Be concise and data-driven.",
		})
	}

	for _, m := range messages {
		msgs = append(msgs, openaiMessage{Role: m.Role, Content: m.Content})
	}

	body := openaiRequest{
		Model:       model,
		Messages:    msgs,
		MaxTokens:   h.cfg.Agent.Model.MaxTokens,
		Temperature: h.cfg.Agent.Model.Temperature,
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", baseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var or openaiResponse
	if err := json.Unmarshal(respBody, &or); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if or.Error != nil {
		return nil, fmt.Errorf("%s", or.Error.Message)
	}

	content := ""
	if len(or.Choices) > 0 {
		content = or.Choices[0].Message.Content
	}

	return &ChatResponse{
		Content: content,
		Model:   or.Model,
		Usage:   &Usage{InputTokens: or.Usage.PromptTokens, OutputTokens: or.Usage.CompletionTokens},
	}, nil
}

// ─── Google (Gemini) ────────────────────────────────────────────────

type googleRequest struct {
	Contents         []googleContent        `json:"contents"`
	SystemInstruction *googleContent        `json:"systemInstruction,omitempty"`
	GenerationConfig  googleGenerationConfig `json:"generationConfig,omitempty"`
}

type googleContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []googlePart `json:"parts"`
}

type googlePart struct {
	Text string `json:"text"`
}

type googleGenerationConfig struct {
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
	Temperature     float64 `json:"temperature,omitempty"`
}

type googleResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (h *LLMHandler) callGoogle(apiKey, model string, messages []ChatMessage) (*ChatResponse, error) {
	var contents []googleContent
	var systemInstr *googleContent

	for _, m := range messages {
		if m.Role == "system" {
			systemInstr = &googleContent{Parts: []googlePart{{Text: m.Content}}}
			continue
		}
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		contents = append(contents, googleContent{
			Role:  role,
			Parts: []googlePart{{Text: m.Content}},
		})
	}

	if systemInstr == nil {
		systemInstr = &googleContent{Parts: []googlePart{{Text: "You are Clawtrade AI, an expert trading copilot. Help users with market analysis, strategy, risk management, and trade execution. Be concise and data-driven."}}}
	}

	body := googleRequest{
		Contents:          contents,
		SystemInstruction: systemInstr,
		GenerationConfig: googleGenerationConfig{
			MaxOutputTokens: h.cfg.Agent.Model.MaxTokens,
			Temperature:     h.cfg.Agent.Model.Temperature,
		},
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)
	req, err := http.NewRequest("POST", url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("google API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var gr googleResponse
	if err := json.Unmarshal(respBody, &gr); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if gr.Error != nil {
		return nil, fmt.Errorf("google: %s", gr.Error.Message)
	}

	content := ""
	if len(gr.Candidates) > 0 && len(gr.Candidates[0].Content.Parts) > 0 {
		content = gr.Candidates[0].Content.Parts[0].Text
	}

	return &ChatResponse{
		Content: content,
		Model:   model,
		Usage:   &Usage{InputTokens: gr.UsageMetadata.PromptTokenCount, OutputTokens: gr.UsageMetadata.CandidatesTokenCount},
	}, nil
}
