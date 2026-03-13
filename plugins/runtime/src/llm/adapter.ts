// Unified LLM interface

export interface LLMMessage {
  role: "system" | "user" | "assistant";
  content: string;
}

export interface LLMResponse {
  content: string;
  model: string;
  usage: {
    inputTokens: number;
    outputTokens: number;
  };
}

export interface LLMConfig {
  provider: "claude" | "openai";
  apiKey: string;
  model?: string;
  maxTokens?: number;
  temperature?: number;
}

export interface LLMAdapter {
  chat(messages: LLMMessage[], config?: Partial<LLMConfig>): Promise<LLMResponse>;
  name(): string;
}
