// internal/subagent/llm_test.go
package subagent

import (
	"testing"
)

func TestBuildAnthropicRequest(t *testing.T) {
	caller := &LLMCaller{
		Provider:  "anthropic",
		Model:     "claude-haiku-4-5-20251001",
		APIKey:    "test-key",
		MaxTokens: 2048,
	}
	body := caller.buildAnthropicBody("system prompt", "user message")
	if body["model"] != "claude-haiku-4-5-20251001" {
		t.Error("wrong model")
	}
	if body["system"] != "system prompt" {
		t.Error("wrong system prompt")
	}
	if body["max_tokens"] != 2048 {
		t.Errorf("wrong max_tokens: got %v", body["max_tokens"])
	}
	msgs, ok := body["messages"].([]map[string]any)
	if !ok || len(msgs) != 1 {
		t.Fatal("expected 1 message")
	}
	if msgs[0]["role"] != "user" || msgs[0]["content"] != "user message" {
		t.Error("wrong user message")
	}
}

func TestBuildOpenAIRequest(t *testing.T) {
	caller := &LLMCaller{
		Provider:  "openai",
		Model:     "gpt-4o-mini",
		APIKey:    "test-key",
		MaxTokens: 2048,
	}
	body := caller.buildOpenAIBody("system prompt", "user message")
	if body["model"] != "gpt-4o-mini" {
		t.Error("wrong model")
	}
	msgs, ok := body["messages"].([]map[string]any)
	if !ok || len(msgs) != 2 {
		t.Fatal("expected 2 messages (system + user)")
	}
	if msgs[0]["role"] != "system" || msgs[0]["content"] != "system prompt" {
		t.Error("wrong system message")
	}
	if msgs[1]["role"] != "user" || msgs[1]["content"] != "user message" {
		t.Error("wrong user message")
	}
}

func TestBuildGoogleRequest(t *testing.T) {
	caller := &LLMCaller{
		Provider:  "google",
		Model:     "gemini-2.0-flash",
		APIKey:    "test-key",
		MaxTokens: 2048,
	}
	body := caller.buildGoogleBody("system prompt", "user message")

	// Check system instruction
	sysInstr, ok := body["systemInstruction"].(map[string]any)
	if !ok {
		t.Fatal("missing systemInstruction")
	}
	parts, ok := sysInstr["parts"].([]map[string]any)
	if !ok || len(parts) != 1 || parts[0]["text"] != "system prompt" {
		t.Error("wrong system instruction")
	}

	// Check contents
	contents, ok := body["contents"].([]map[string]any)
	if !ok || len(contents) != 1 {
		t.Fatal("expected 1 content entry")
	}
	if contents[0]["role"] != "user" {
		t.Error("wrong role")
	}

	// Check generation config
	genCfg, ok := body["generationConfig"].(map[string]any)
	if !ok {
		t.Fatal("missing generationConfig")
	}
	if genCfg["maxOutputTokens"] != 2048 {
		t.Error("wrong maxOutputTokens")
	}
}

func TestNewLLMCallerFromConfig(t *testing.T) {
	caller := NewLLMCaller("anthropic/claude-haiku-4-5-20251001", "test-key", 2048)
	if caller.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got %q", caller.Provider)
	}
	if caller.Model != "claude-haiku-4-5-20251001" {
		t.Errorf("expected model 'claude-haiku-4-5-20251001', got %q", caller.Model)
	}
	if caller.BaseURL != "https://api.anthropic.com/v1/messages" {
		t.Errorf("unexpected BaseURL: %q", caller.BaseURL)
	}
}

func TestNewLLMCallerOpenAI(t *testing.T) {
	caller := NewLLMCaller("openai/gpt-4o-mini", "test-key", 1024)
	if caller.Provider != "openai" {
		t.Errorf("expected provider 'openai', got %q", caller.Provider)
	}
	if caller.Model != "gpt-4o-mini" {
		t.Errorf("expected model 'gpt-4o-mini', got %q", caller.Model)
	}
	if caller.BaseURL != "https://api.openai.com/v1/chat/completions" {
		t.Errorf("unexpected BaseURL: %q", caller.BaseURL)
	}
}

func TestNewLLMCallerDeepSeek(t *testing.T) {
	caller := NewLLMCaller("deepseek/deepseek-chat", "test-key", 1024)
	if caller.Provider != "deepseek" {
		t.Errorf("expected provider 'deepseek', got %q", caller.Provider)
	}
	if caller.BaseURL != "https://api.deepseek.com/v1/chat/completions" {
		t.Errorf("unexpected BaseURL: %q", caller.BaseURL)
	}
}

func TestNewLLMCallerOpenRouter(t *testing.T) {
	caller := NewLLMCaller("openrouter/meta-llama/llama-3-70b", "test-key", 1024)
	if caller.Provider != "openrouter" {
		t.Errorf("expected provider 'openrouter', got %q", caller.Provider)
	}
	if caller.Model != "meta-llama/llama-3-70b" {
		t.Errorf("expected model 'meta-llama/llama-3-70b', got %q", caller.Model)
	}
	if caller.BaseURL != "https://openrouter.ai/api/v1/chat/completions" {
		t.Errorf("unexpected BaseURL: %q", caller.BaseURL)
	}
}

func TestNewLLMCallerOllama(t *testing.T) {
	caller := NewLLMCaller("ollama/llama3", "test-key", 1024)
	if caller.Provider != "ollama" {
		t.Errorf("expected provider 'ollama', got %q", caller.Provider)
	}
	if caller.BaseURL != "http://localhost:11434/v1/chat/completions" {
		t.Errorf("unexpected BaseURL: %q", caller.BaseURL)
	}
}

func TestNewLLMCallerGoogle(t *testing.T) {
	caller := NewLLMCaller("google/gemini-2.0-flash", "test-key", 1024)
	if caller.Provider != "google" {
		t.Errorf("expected provider 'google', got %q", caller.Provider)
	}
	if caller.Model != "gemini-2.0-flash" {
		t.Errorf("expected model 'gemini-2.0-flash', got %q", caller.Model)
	}
	// Google URL is dynamic per-call, so BaseURL stores template base
	if caller.BaseURL != "https://generativelanguage.googleapis.com/v1beta" {
		t.Errorf("unexpected BaseURL: %q", caller.BaseURL)
	}
}
