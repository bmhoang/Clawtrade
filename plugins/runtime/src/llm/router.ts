import type { LLMAdapter, LLMConfig, LLMMessage, LLMResponse } from "./adapter";
import { ClaudeAdapter } from "./claude";
import { OpenAIAdapter } from "./openai";

export class LLMRouter {
  private adapters: Map<string, LLMAdapter> = new Map();
  private primaryProvider: string;

  constructor(configs: LLMConfig[]) {
    for (const config of configs) {
      const adapter = this.createAdapter(config);
      this.adapters.set(config.provider, adapter);
    }
    this.primaryProvider = configs[0]?.provider || "claude";
  }

  private createAdapter(config: LLMConfig): LLMAdapter {
    switch (config.provider) {
      case "claude":
        return new ClaudeAdapter(config);
      case "openai":
        return new OpenAIAdapter(config);
      default:
        throw new Error(`Unknown provider: ${config.provider}`);
    }
  }

  async chat(messages: LLMMessage[], provider?: string): Promise<LLMResponse> {
    const targetProvider = provider || this.primaryProvider;
    const adapter = this.adapters.get(targetProvider);

    if (!adapter) {
      // Fallback to any available adapter
      const fallback = this.adapters.values().next().value;
      if (!fallback) {
        throw new Error("No LLM adapters configured");
      }
      console.error(`[llm] Provider ${targetProvider} not found, falling back to ${fallback.name()}`);
      return fallback.chat(messages);
    }

    try {
      return await adapter.chat(messages);
    } catch (err) {
      // Try fallback on error
      for (const [name, fallbackAdapter] of this.adapters) {
        if (name !== targetProvider) {
          console.error(`[llm] ${targetProvider} failed, falling back to ${name}`);
          return fallbackAdapter.chat(messages);
        }
      }
      throw err;
    }
  }

  getAvailableProviders(): string[] {
    return Array.from(this.adapters.keys());
  }
}
