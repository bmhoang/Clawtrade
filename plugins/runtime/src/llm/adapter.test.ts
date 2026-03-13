import { describe, test, expect } from "bun:test";
import { LLMRouter } from "./router";
import type { LLMConfig } from "./adapter";

describe("LLMRouter", () => {
  test("getAvailableProviders returns configured providers", () => {
    const configs: LLMConfig[] = [
      { provider: "claude", apiKey: "test-key" },
    ];
    const router = new LLMRouter(configs);
    expect(router.getAvailableProviders()).toEqual(["claude"]);
  });

  test("supports multiple providers", () => {
    const configs: LLMConfig[] = [
      { provider: "claude", apiKey: "test-key-1" },
      { provider: "openai", apiKey: "test-key-2" },
    ];
    const router = new LLMRouter(configs);
    expect(router.getAvailableProviders()).toEqual(["claude", "openai"]);
  });

  test("throws on unknown provider", () => {
    expect(() => {
      new LLMRouter([{ provider: "unknown" as any, apiKey: "test" }]);
    }).toThrow("Unknown provider");
  });

  test("chat fails gracefully without valid key", async () => {
    const router = new LLMRouter([
      { provider: "claude", apiKey: "invalid-key" },
    ]);
    // Should throw because the API key is invalid
    await expect(
      router.chat([{ role: "user", content: "test" }])
    ).rejects.toThrow();
  });
});
