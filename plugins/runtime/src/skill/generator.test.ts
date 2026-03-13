import { describe, it, expect, mock } from "bun:test";
import { SkillGenerator } from "./generator";
import type { GenerationRequest, GenerationResult } from "./generator";
import type { LLMAdapter, LLMMessage, LLMResponse } from "../llm/adapter";

// ---------------------------------------------------------------------------
// Mock LLM adapter
// ---------------------------------------------------------------------------

function createMockLLM(responseContent?: string): LLMAdapter & { lastMessages: LLMMessage[] } {
  const defaultResponse = JSON.stringify({
    definition: {
      name: "price-alert",
      version: "1.0.0",
      description: "Monitors crypto prices and sends alerts",
      permissions: [{ type: "market-data" }, { type: "network" }],
      tools: [
        {
          name: "set-alert",
          description: "Set a price alert for a trading pair",
          parameters: [
            {
              name: "symbol",
              type: "string",
              description: "Trading pair symbol",
              required: true,
            },
            {
              name: "threshold",
              type: "number",
              description: "Price threshold",
              required: true,
            },
          ],
          handler: "async (params, ctx) => { /* placeholder */ }",
        },
      ],
    },
    code: 'import { defineSkill } from "./sdk";\n\nexport default defineSkill({ /* ... */ });',
    explanation: "This skill monitors crypto prices and triggers alerts when thresholds are crossed.",
    suggestedTests: [
      "Should set an alert for BTC/USD",
      "Should trigger when price exceeds threshold",
      "Should handle invalid symbols gracefully",
    ],
  });

  const adapter: LLMAdapter & { lastMessages: LLMMessage[] } = {
    lastMessages: [],
    async chat(messages: LLMMessage[]): Promise<LLMResponse> {
      adapter.lastMessages = messages;
      return {
        content: responseContent ?? defaultResponse,
        model: "mock-model",
        usage: { inputTokens: 100, outputTokens: 200 },
      };
    },
    name() {
      return "mock";
    },
  };

  return adapter;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("SkillGenerator", () => {
  // -- generateName ---------------------------------------------------------

  describe("generateName", () => {
    it("creates valid kebab-case names", () => {
      const name = SkillGenerator.generateName("Calculate moving averages");
      expect(name).toBe("calculate-moving-averages");
    });

    it("removes stop words", () => {
      const name = SkillGenerator.generateName(
        "Calculate the moving averages for crypto",
      );
      expect(name).not.toContain("-the-");
      expect(name).not.toContain("-for-");
      // Should still contain the meaningful words
      expect(name).toContain("calculate");
      expect(name).toContain("moving");
      expect(name).toContain("averages");
    });

    it("truncates long descriptions to 30 chars or fewer", () => {
      const name = SkillGenerator.generateName(
        "Calculate exponential moving averages across multiple timeframes for cryptocurrency pairs",
      );
      expect(name.length).toBeLessThanOrEqual(30);
      // Should not end with a hyphen
      expect(name.endsWith("-")).toBe(false);
    });

    it("handles empty or stop-words-only descriptions", () => {
      const name = SkillGenerator.generateName("the and or");
      expect(name).toBe("unnamed-skill");
    });

    it("strips non-alphanumeric characters", () => {
      const name = SkillGenerator.generateName("Price! Alert? (v2)");
      expect(name).toBe("price-alert-v2");
    });
  });

  // -- buildSystemPrompt ----------------------------------------------------

  describe("buildSystemPrompt", () => {
    it("includes SDK documentation", () => {
      const gen = new SkillGenerator(createMockLLM());
      const prompt = gen.buildSystemPrompt();
      expect(prompt).toContain("defineSkill");
      expect(prompt).toContain("SkillDefinition");
      expect(prompt).toContain("ToolDefinition");
      expect(prompt).toContain("ParameterDef");
      expect(prompt).toContain("Permission");
      expect(prompt).toContain("market-data");
      expect(prompt).toContain("trade-execute");
      expect(prompt).toContain("SkillContext");
    });
  });

  // -- buildUserPrompt ------------------------------------------------------

  describe("buildUserPrompt", () => {
    it("includes description", () => {
      const gen = new SkillGenerator(createMockLLM());
      const prompt = gen.buildUserPrompt({
        description: "Monitor BTC price",
      });
      expect(prompt).toContain("Monitor BTC price");
    });

    it("includes examples when provided", () => {
      const gen = new SkillGenerator(createMockLLM());
      const prompt = gen.buildUserPrompt({
        description: "Monitor BTC price",
        examples: ["Input: BTC/USD, Output: alert when > 50000"],
      });
      expect(prompt).toContain("Examples:");
      expect(prompt).toContain("Input: BTC/USD, Output: alert when > 50000");
    });

    it("includes constraints when provided", () => {
      const gen = new SkillGenerator(createMockLLM());
      const prompt = gen.buildUserPrompt({
        description: "Monitor BTC price",
        constraints: ["Must handle rate limiting", "Max 10 alerts"],
      });
      expect(prompt).toContain("Constraints:");
      expect(prompt).toContain("Must handle rate limiting");
      expect(prompt).toContain("Max 10 alerts");
    });

    it("uses provided name", () => {
      const gen = new SkillGenerator(createMockLLM());
      const prompt = gen.buildUserPrompt({
        description: "Monitor BTC price",
        name: "btc-watcher",
      });
      expect(prompt).toContain("Name: btc-watcher");
    });
  });

  // -- parseResponse --------------------------------------------------------

  describe("parseResponse", () => {
    it("extracts valid SkillDefinition from JSON response", () => {
      const gen = new SkillGenerator(createMockLLM());
      const json = JSON.stringify({
        definition: {
          name: "test-skill",
          version: "1.0.0",
          description: "A test skill",
          permissions: [{ type: "market-data" }],
          tools: [
            {
              name: "do-thing",
              description: "Does a thing",
              parameters: [],
              handler: "async (params, ctx) => {}",
            },
          ],
        },
        code: "// code here",
        explanation: "It does things",
        suggestedTests: ["test 1"],
      });

      const result = gen.parseResponse(json);
      expect(result.definition.name).toBe("test-skill");
      expect(result.definition.version).toBe("1.0.0");
      expect(result.definition.tools).toHaveLength(1);
      expect(result.definition.tools[0].name).toBe("do-thing");
      expect(result.definition.permissions[0].type).toBe("market-data");
      expect(result.code).toBe("// code here");
      expect(result.explanation).toBe("It does things");
      expect(result.suggestedTests).toEqual(["test 1"]);
    });

    it("handles markdown code blocks", () => {
      const gen = new SkillGenerator(createMockLLM());
      const json = JSON.stringify({
        definition: {
          name: "md-skill",
          version: "1.0.0",
          description: "Parsed from markdown",
          permissions: [],
          tools: [
            {
              name: "run",
              description: "Runs",
              parameters: [],
              handler: "async () => {}",
            },
          ],
        },
        code: "",
        explanation: "Extracted from code block",
        suggestedTests: [],
      });

      const wrapped = `Here is the skill:\n\n\`\`\`json\n${json}\n\`\`\`\n\nHope that helps!`;
      const result = gen.parseResponse(wrapped);
      expect(result.definition.name).toBe("md-skill");
      expect(result.explanation).toBe("Extracted from code block");
    });

    it("handles invalid JSON gracefully", () => {
      const gen = new SkillGenerator(createMockLLM());
      expect(() => gen.parseResponse("this is not json {{{")).toThrow(
        /Failed to parse LLM response as JSON/,
      );
    });

    it("throws when definition field is missing", () => {
      const gen = new SkillGenerator(createMockLLM());
      expect(() => gen.parseResponse(JSON.stringify({ code: "x" }))).toThrow(
        /missing 'definition'/,
      );
    });
  });

  // -- validateDefinition ---------------------------------------------------

  describe("validateDefinition", () => {
    it("catches missing name", () => {
      const gen = new SkillGenerator(createMockLLM());
      const errors = gen.validateDefinition({
        version: "1.0.0",
        description: "desc",
        permissions: [],
        tools: [
          { name: "t", description: "d", handler: async () => {}, parameters: [] },
        ],
      });
      expect(errors).toContain("Missing or invalid 'name'");
    });

    it("catches missing tools", () => {
      const gen = new SkillGenerator(createMockLLM());
      const errors = gen.validateDefinition({
        name: "test",
        version: "1.0.0",
        description: "desc",
        permissions: [],
        tools: [],
      });
      expect(errors).toContain("Skill must define at least one tool");
    });

    it("catches invalid permission types", () => {
      const gen = new SkillGenerator(createMockLLM());
      const errors = gen.validateDefinition({
        name: "test",
        version: "1.0.0",
        description: "desc",
        permissions: [{ type: "invalid-perm" }],
        tools: [
          { name: "t", description: "d", handler: async () => {}, parameters: [] },
        ],
      });
      expect(errors.some((e) => e.includes("invalid type 'invalid-perm'"))).toBe(
        true,
      );
    });

    it("passes valid definition", () => {
      const gen = new SkillGenerator(createMockLLM());
      const errors = gen.validateDefinition({
        name: "valid-skill",
        version: "1.0.0",
        description: "A valid skill",
        permissions: [{ type: "market-data" }],
        tools: [
          {
            name: "do-thing",
            description: "Does something",
            parameters: [],
            handler: async () => {},
          },
        ],
      });
      expect(errors).toHaveLength(0);
    });

    it("rejects non-object input", () => {
      const gen = new SkillGenerator(createMockLLM());
      const errors = gen.validateDefinition(null);
      expect(errors).toContain("Definition must be an object");
    });
  });

  // -- generate (integration with mock LLM) --------------------------------

  describe("generate", () => {
    it("calls LLM with correct messages", async () => {
      const mockLLM = createMockLLM();
      const gen = new SkillGenerator(mockLLM);

      const result = await gen.generate({
        description: "Monitor crypto prices and send alerts",
      });

      // Verify LLM was called with system + user messages
      expect(mockLLM.lastMessages).toHaveLength(2);
      expect(mockLLM.lastMessages[0].role).toBe("system");
      expect(mockLLM.lastMessages[1].role).toBe("user");

      // System prompt should contain SDK docs
      expect(mockLLM.lastMessages[0].content).toContain("defineSkill");

      // User prompt should contain the description
      expect(mockLLM.lastMessages[1].content).toContain(
        "Monitor crypto prices and send alerts",
      );

      // Result should have a valid definition
      expect(result.definition.name).toBe("price-alert");
      expect(result.definition.tools).toHaveLength(1);
      expect(result.code).toBeTruthy();
      expect(result.explanation).toBeTruthy();
      expect(result.suggestedTests.length).toBeGreaterThan(0);
    });

    it("uses the provided name override", async () => {
      const mockLLM = createMockLLM();
      const gen = new SkillGenerator(mockLLM);

      const result = await gen.generate({
        description: "Monitor crypto prices",
        name: "my-custom-name",
      });

      expect(result.definition.name).toBe("my-custom-name");
    });
  });
});
