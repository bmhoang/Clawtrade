import Anthropic from "@anthropic-ai/sdk";
import type { LLMAdapter, LLMMessage, LLMResponse, LLMConfig } from "./adapter";

export class ClaudeAdapter implements LLMAdapter {
  private client: Anthropic;
  private defaultModel: string;
  private defaultMaxTokens: number;

  constructor(config: LLMConfig) {
    this.client = new Anthropic({ apiKey: config.apiKey });
    this.defaultModel = config.model || "claude-sonnet-4-20250514";
    this.defaultMaxTokens = config.maxTokens || 4096;
  }

  name(): string {
    return "claude";
  }

  async chat(messages: LLMMessage[], config?: Partial<LLMConfig>): Promise<LLMResponse> {
    const systemMsg = messages.find(m => m.role === "system");
    const chatMessages = messages
      .filter(m => m.role !== "system")
      .map(m => ({ role: m.role as "user" | "assistant", content: m.content }));

    const response = await this.client.messages.create({
      model: config?.model || this.defaultModel,
      max_tokens: config?.maxTokens || this.defaultMaxTokens,
      temperature: config?.temperature ?? 0.7,
      system: systemMsg?.content,
      messages: chatMessages,
    });

    const textBlock = response.content.find(b => b.type === "text");

    return {
      content: textBlock?.text || "",
      model: response.model,
      usage: {
        inputTokens: response.usage.input_tokens,
        outputTokens: response.usage.output_tokens,
      },
    };
  }
}
