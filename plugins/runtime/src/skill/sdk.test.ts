import { describe, expect, test } from "bun:test";
import { defineSkill, Skill, ToolRegistry } from "./sdk";
import type { SkillContext, SkillDefinition, ToolDefinition } from "./sdk";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeCtx(overrides: Partial<SkillContext> = {}): SkillContext {
  return {
    ipc: {} as SkillContext["ipc"],
    log: () => {},
    getMarketData: async () => ({
      symbol: "BTC/USD",
      price: 50000,
      bid: 49999,
      ask: 50001,
      volume: 100,
      timestamp: Date.now(),
    }),
    executeOrder: async () => ({
      orderId: "ord-1",
      status: "filled" as const,
      filledQuantity: 1,
      filledPrice: 50000,
      timestamp: Date.now(),
    }),
    getMemory: async () => null,
    callLLM: async () => ({
      content: "ok",
      model: "test",
      usage: { inputTokens: 0, outputTokens: 0 },
    }),
    ...overrides,
  };
}

function sampleDef(overrides: Partial<SkillDefinition> = {}): SkillDefinition {
  return {
    name: "test-skill",
    version: "1.0.0",
    description: "A test skill",
    permissions: [{ type: "market-data" }, { type: "trade-read", scope: "BTC/USD" }],
    tools: [
      {
        name: "get-price",
        description: "Get price for a symbol",
        parameters: [
          { name: "symbol", type: "string", description: "Ticker", required: true },
          { name: "verbose", type: "boolean", description: "Verbose", required: false, default: false },
        ],
        handler: async (params, _ctx) => ({ price: 42000, symbol: params.symbol }),
      },
    ],
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// defineSkill
// ---------------------------------------------------------------------------

describe("defineSkill", () => {
  test("creates a valid Skill instance", () => {
    const skill = defineSkill(sampleDef());
    expect(skill).toBeInstanceOf(Skill);
    expect(skill.name).toBe("test-skill");
    expect(skill.version).toBe("1.0.0");
    expect(skill.description).toBe("A test skill");
  });

  test("throws on missing name", () => {
    expect(() => defineSkill(sampleDef({ name: "" }))).toThrow("Skill name is required");
  });

  test("throws on missing version", () => {
    expect(() => defineSkill(sampleDef({ version: "" }))).toThrow("Skill version is required");
  });

  test("throws on missing description", () => {
    expect(() => defineSkill(sampleDef({ description: "" }))).toThrow(
      "Skill description is required",
    );
  });
});

// ---------------------------------------------------------------------------
// Skill - tool registration and lookup
// ---------------------------------------------------------------------------

describe("Skill tools", () => {
  test("listTools returns all registered tools", () => {
    const skill = defineSkill(sampleDef());
    const tools = skill.listTools();
    expect(tools).toHaveLength(1);
    expect(tools[0].name).toBe("get-price");
  });

  test("getTool finds a tool by name", () => {
    const skill = defineSkill(sampleDef());
    expect(skill.getTool("get-price")).toBeDefined();
    expect(skill.getTool("nonexistent")).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// Parameter validation
// ---------------------------------------------------------------------------

describe("parameter validation", () => {
  test("rejects missing required params", async () => {
    const skill = defineSkill(sampleDef());
    skill.setContext(makeCtx());

    await expect(skill.executeTool("get-price", {})).rejects.toThrow(
      "Missing required parameter 'symbol'",
    );
  });

  test("applies default values for optional params", async () => {
    const captured: Record<string, unknown>[] = [];
    const def = sampleDef({
      tools: [
        {
          name: "echo",
          description: "Echo params",
          parameters: [
            { name: "msg", type: "string", description: "Message", required: true },
            { name: "count", type: "number", description: "Count", required: false, default: 1 },
          ],
          handler: async (params) => {
            captured.push(params);
            return params;
          },
        },
      ],
    });
    const skill = defineSkill(def);
    skill.setContext(makeCtx());

    await skill.executeTool("echo", { msg: "hi" });
    expect(captured[0].count).toBe(1);
  });

  test("rejects wrong parameter type", async () => {
    const skill = defineSkill(sampleDef());
    skill.setContext(makeCtx());

    await expect(
      skill.executeTool("get-price", { symbol: 123 }),
    ).rejects.toThrow("expected string, got number");
  });

  test("throws when tool not found", async () => {
    const skill = defineSkill(sampleDef());
    skill.setContext(makeCtx());

    await expect(skill.executeTool("nope", {})).rejects.toThrow("Tool not found: nope");
  });
});

// ---------------------------------------------------------------------------
// Permissions
// ---------------------------------------------------------------------------

describe("permissions", () => {
  test("lists permissions from definition", () => {
    const skill = defineSkill(sampleDef());
    expect(skill.permissions).toHaveLength(2);
    expect(skill.permissions[0].type).toBe("market-data");
  });

  test("hasPermission checks type", () => {
    const skill = defineSkill(sampleDef());
    expect(skill.hasPermission("market-data")).toBe(true);
    expect(skill.hasPermission("trade-execute")).toBe(false);
  });

  test("hasPermission checks type and scope", () => {
    const skill = defineSkill(sampleDef());
    expect(skill.hasPermission("trade-read", "BTC/USD")).toBe(true);
    expect(skill.hasPermission("trade-read", "ETH/USD")).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Tool execution
// ---------------------------------------------------------------------------

describe("tool execution", () => {
  test("calls handler and returns result", async () => {
    const skill = defineSkill(sampleDef());
    skill.setContext(makeCtx());

    const result = (await skill.executeTool("get-price", { symbol: "BTC/USD" })) as {
      price: number;
      symbol: string;
    };

    expect(result.price).toBe(42000);
    expect(result.symbol).toBe("BTC/USD");
  });

  test("throws if context not set", async () => {
    const skill = defineSkill(sampleDef());

    await expect(skill.executeTool("get-price", { symbol: "BTC" })).rejects.toThrow(
      "Skill context not set",
    );
  });
});

// ---------------------------------------------------------------------------
// ToolRegistry
// ---------------------------------------------------------------------------

describe("ToolRegistry", () => {
  test("registers and lists tools from a skill", () => {
    const registry = new ToolRegistry();
    const skill = defineSkill(sampleDef());
    registry.registerSkill(skill);

    const tools = registry.listTools();
    expect(tools).toHaveLength(1);
    expect(tools[0].skillName).toBe("test-skill");
    expect(tools[0].tool.name).toBe("get-price");
  });

  test("looks up tool by qualified name", () => {
    const registry = new ToolRegistry();
    const skill = defineSkill(sampleDef());
    registry.registerSkill(skill);

    const entry = registry.getTool("test-skill.get-price");
    expect(entry).toBeDefined();
    expect(entry!.tool.name).toBe("get-price");
  });

  test("callTool executes via the owning skill", async () => {
    const registry = new ToolRegistry();
    const skill = defineSkill(sampleDef());
    skill.setContext(makeCtx());
    registry.registerSkill(skill);

    const result = (await registry.callTool("test-skill.get-price", {
      symbol: "ETH",
    })) as { price: number };
    expect(result.price).toBe(42000);
  });

  test("callTool throws for unknown tool", async () => {
    const registry = new ToolRegistry();
    await expect(registry.callTool("nope.nope", {})).rejects.toThrow(
      "Tool not found in registry",
    );
  });

  test("unregisterSkill removes all tools for a skill", () => {
    const registry = new ToolRegistry();
    const skill = defineSkill(sampleDef());
    registry.registerSkill(skill);

    registry.unregisterSkill("test-skill");
    expect(registry.listTools()).toHaveLength(0);
  });

  test("rejects duplicate tool registration", () => {
    const registry = new ToolRegistry();
    const skill = defineSkill(sampleDef());
    registry.registerSkill(skill);

    expect(() => registry.registerSkill(skill)).toThrow("Tool already registered");
  });
});

// ---------------------------------------------------------------------------
// Lifecycle hooks
// ---------------------------------------------------------------------------

describe("lifecycle hooks", () => {
  test("onLoad is called during load()", async () => {
    let loaded = false;
    const skill = defineSkill(
      sampleDef({ onLoad: async () => { loaded = true; } }),
    );
    await skill.load();
    expect(loaded).toBe(true);
  });

  test("onUnload is called during unload()", async () => {
    let unloaded = false;
    const skill = defineSkill(
      sampleDef({ onUnload: async () => { unloaded = true; } }),
    );
    await skill.unload();
    expect(unloaded).toBe(true);
  });
});
