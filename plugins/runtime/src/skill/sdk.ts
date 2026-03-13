// Skill SDK - defineSkill() API, permission model, and tool registration

import type { IPCClient } from "../ipc";
import type { LLMMessage, LLMResponse } from "../llm/adapter";
import type { MarketData, OrderRequest, OrderResult } from "./types";

// ---------------------------------------------------------------------------
// Core interfaces
// ---------------------------------------------------------------------------

export interface Permission {
  type:
    | "market-data"
    | "trade-execute"
    | "trade-read"
    | "memory-read"
    | "memory-write"
    | "llm-access"
    | "network";
  scope?: string;
}

export interface ParameterDef {
  name: string;
  type: "string" | "number" | "boolean" | "object";
  description: string;
  required?: boolean;
  default?: unknown;
}

export interface ToolDefinition {
  name: string;
  description: string;
  parameters: ParameterDef[];
  handler: (params: Record<string, unknown>, ctx: SkillContext) => Promise<unknown>;
}

export interface SkillDefinition {
  name: string;
  version: string;
  description: string;
  author?: string;
  permissions: Permission[];
  tools: ToolDefinition[];
  onLoad?: () => Promise<void>;
  onUnload?: () => Promise<void>;
}

// ---------------------------------------------------------------------------
// SkillContext - injected into tool handlers for platform access
// ---------------------------------------------------------------------------

export interface SkillContext {
  ipc: IPCClient;
  log: (msg: string) => void;
  getMarketData: (symbol: string) => Promise<MarketData>;
  executeOrder: (order: OrderRequest) => Promise<OrderResult>;
  getMemory: (query: string) => Promise<unknown>;
  callLLM: (messages: LLMMessage[]) => Promise<LLMResponse>;
}

// ---------------------------------------------------------------------------
// Skill class - runtime representation of a loaded skill
// ---------------------------------------------------------------------------

export class Skill {
  readonly name: string;
  readonly version: string;
  readonly description: string;
  readonly author: string | undefined;
  readonly permissions: ReadonlyArray<Permission>;

  private tools: Map<string, ToolDefinition> = new Map();
  private _onLoad?: () => Promise<void>;
  private _onUnload?: () => Promise<void>;
  private _context: SkillContext | null = null;

  constructor(def: SkillDefinition) {
    this.name = def.name;
    this.version = def.version;
    this.description = def.description;
    this.author = def.author;
    this.permissions = Object.freeze([...def.permissions]);
    this._onLoad = def.onLoad;
    this._onUnload = def.onUnload;

    for (const tool of def.tools) {
      this.tools.set(tool.name, tool);
    }
  }

  /** Attach a context (usually done once after construction). */
  setContext(ctx: SkillContext): void {
    this._context = ctx;
  }

  get context(): SkillContext | null {
    return this._context;
  }

  /** Return all tool definitions registered by this skill. */
  listTools(): ToolDefinition[] {
    return Array.from(this.tools.values());
  }

  /** Look up a single tool by name. */
  getTool(name: string): ToolDefinition | undefined {
    return this.tools.get(name);
  }

  /** Check whether the skill declared a given permission type. */
  hasPermission(type: Permission["type"], scope?: string): boolean {
    return this.permissions.some(
      (p) => p.type === type && (scope === undefined || p.scope === scope),
    );
  }

  /**
   * Execute a tool by name.
   *
   * 1. Validates required parameters are present.
   * 2. Applies defaults for missing optional parameters.
   * 3. Performs basic type checking.
   * 4. Calls the handler with the validated params and context.
   */
  async executeTool(
    toolName: string,
    rawParams: Record<string, unknown>,
  ): Promise<unknown> {
    const tool = this.tools.get(toolName);
    if (!tool) {
      throw new Error(`Tool not found: ${toolName}`);
    }

    const validated = this.validateParams(tool, rawParams);

    if (!this._context) {
      throw new Error("Skill context not set; call setContext() before executing tools");
    }

    return tool.handler(validated, this._context);
  }

  /** Run the optional onLoad lifecycle hook. */
  async load(): Promise<void> {
    if (this._onLoad) await this._onLoad();
  }

  /** Run the optional onUnload lifecycle hook. */
  async unload(): Promise<void> {
    if (this._onUnload) await this._onUnload();
  }

  // ---- internal helpers ---------------------------------------------------

  private validateParams(
    tool: ToolDefinition,
    raw: Record<string, unknown>,
  ): Record<string, unknown> {
    const result: Record<string, unknown> = { ...raw };

    for (const paramDef of tool.parameters) {
      const value = result[paramDef.name];

      // Required check
      if (value === undefined || value === null) {
        if (paramDef.required) {
          throw new Error(
            `Missing required parameter '${paramDef.name}' for tool '${tool.name}'`,
          );
        }
        // Apply default if available
        if (paramDef.default !== undefined) {
          result[paramDef.name] = paramDef.default;
        }
        continue;
      }

      // Basic type check
      const actualType = typeof value;
      if (paramDef.type === "object") {
        if (actualType !== "object" || value === null) {
          throw new Error(
            `Parameter '${paramDef.name}' for tool '${tool.name}' expected object, got ${actualType}`,
          );
        }
      } else if (actualType !== paramDef.type) {
        throw new Error(
          `Parameter '${paramDef.name}' for tool '${tool.name}' expected ${paramDef.type}, got ${actualType}`,
        );
      }
    }

    return result;
  }
}

// ---------------------------------------------------------------------------
// ToolRegistry - global registry of tools across all loaded skills
// ---------------------------------------------------------------------------

export class ToolRegistry {
  /** Maps fully-qualified tool name ("skillName.toolName") to entry. */
  private entries: Map<string, { skill: Skill; tool: ToolDefinition }> = new Map();

  /** Register all tools from a skill. */
  registerSkill(skill: Skill): void {
    for (const tool of skill.listTools()) {
      const key = `${skill.name}.${tool.name}`;
      if (this.entries.has(key)) {
        throw new Error(`Tool already registered: ${key}`);
      }
      this.entries.set(key, { skill, tool });
    }
  }

  /** Unregister all tools belonging to a skill. */
  unregisterSkill(skillName: string): void {
    for (const key of [...this.entries.keys()]) {
      if (key.startsWith(`${skillName}.`)) {
        this.entries.delete(key);
      }
    }
  }

  /** List every registered tool with its owning skill name. */
  listTools(): Array<{ skillName: string; tool: ToolDefinition }> {
    return Array.from(this.entries.values()).map((e) => ({
      skillName: e.skill.name,
      tool: e.tool,
    }));
  }

  /** Look up a tool by its fully-qualified name. */
  getTool(qualifiedName: string): { skill: Skill; tool: ToolDefinition } | undefined {
    return this.entries.get(qualifiedName);
  }

  /** Execute a tool by qualified name, delegating to the owning skill. */
  async callTool(
    qualifiedName: string,
    params: Record<string, unknown>,
  ): Promise<unknown> {
    const entry = this.entries.get(qualifiedName);
    if (!entry) {
      throw new Error(`Tool not found in registry: ${qualifiedName}`);
    }
    return entry.skill.executeTool(entry.tool.name, params);
  }
}

// ---------------------------------------------------------------------------
// defineSkill() - main entry point
// ---------------------------------------------------------------------------

export function defineSkill(def: SkillDefinition): Skill {
  // Basic validation
  if (!def.name || typeof def.name !== "string") {
    throw new Error("Skill name is required and must be a string");
  }
  if (!def.version || typeof def.version !== "string") {
    throw new Error("Skill version is required and must be a string");
  }
  if (!def.description || typeof def.description !== "string") {
    throw new Error("Skill description is required and must be a string");
  }
  if (!Array.isArray(def.permissions)) {
    throw new Error("Skill permissions must be an array");
  }
  if (!Array.isArray(def.tools)) {
    throw new Error("Skill tools must be an array");
  }

  return new Skill(def);
}
