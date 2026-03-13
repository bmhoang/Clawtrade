import OpenAI from "openai";
import type { LLMAdapter, LLMMessage, LLMResponse, LLMConfig } from "./adapter";

export class OpenAIAdapter implements LLMAdapter {
  private client: OpenAI;
  private defaultModel: string;
  private defaultMaxTokens: number;

  constructor(config: LLMConfig) {
    this.client = new OpenAI({ apiKey: config.apiKey });
    this.defaultModel = config.model || "gpt-4o";
    this.defaultMaxTokens = config.maxTokens || 4096;
  }

  name(): string {
    return "openai";
  }

  async chat(messages: LLMMessage[], config?: Partial<LLMConfig>): Promise<LLMResponse> {
    const response = await this.client.chat.completions.create({
      model: config?.model || this.defaultModel,
      max_tokens: config?.maxTokens || this.defaultMaxTokens,
      temperature: config?.temperature ?? 0.7,
      messages: messages.map(m => ({ role: m.role, content: m.content })),
    });

    const choice = response.choices[0];

    return {
      content: choice?.message?.content || "",
      model: response.model,
      usage: {
        inputTokens: response.usage?.prompt_tokens || 0,
        outputTokens: response.usage?.completion_tokens || 0,
      },
    };
  }
}
