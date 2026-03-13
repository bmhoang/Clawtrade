// AI Skill Generator - generate TypeScript skills from natural language descriptions

import type { LLMAdapter, LLMMessage } from "../llm/adapter";
import type {
  SkillDefinition,
  Permission,
  ToolDefinition,
  ParameterDef,
} from "./sdk";

// ---------------------------------------------------------------------------
// Interfaces
// ---------------------------------------------------------------------------

export interface GenerationRequest {
  description: string;
  name?: string;
  examples?: string[];
  constraints?: string[];
}

export interface GenerationResult {
  definition: SkillDefinition;
  code: string;
  explanation: string;
  suggestedTests: string[];
}

// Valid permission types for validation
const VALID_PERMISSION_TYPES: ReadonlyArray<Permission["type"]> = [
  "market-data",
  "trade-execute",
  "trade-read",
  "memory-read",
  "memory-write",
  "llm-access",
  "network",
];

// Stop words to remove when generating names
const STOP_WORDS = new Set([
  "a",
  "an",
  "the",
  "for",
  "and",
  "or",
  "but",
  "in",
  "on",
  "at",
  "to",
  "of",
  "is",
  "it",
  "by",
  "with",
  "from",
  "as",
  "into",
  "that",
  "this",
  "which",
]);

// ---------------------------------------------------------------------------
// SkillGenerator
// ---------------------------------------------------------------------------

export class SkillGenerator {
  private llm: LLMAdapter;

  constructor(llm: LLMAdapter) {
    this.llm = llm;
  }

  /**
   * Generate a skill definition from a natural language description.
   */
  async generate(request: GenerationRequest): Promise<GenerationResult> {
    const systemPrompt = this.buildSystemPrompt();
    const userPrompt = this.buildUserPrompt(request);

    const messages: LLMMessage[] = [
      { role: "system", content: systemPrompt },
      { role: "user", content: userPrompt },
    ];

    const response = await this.llm.chat(messages);
    const result = this.parseResponse(response.content);

    // Override name if the request specified one
    if (request.name) {
      result.definition.name = request.name;
    }

    const errors = this.validateDefinition(result.definition);
    if (errors.length > 0) {
      throw new Error(
        `Generated skill definition is invalid:\n${errors.join("\n")}`,
      );
    }

    return result;
  }

  /**
   * Build the system prompt that explains the Skill SDK to the LLM.
   */
  buildSystemPrompt(): string {
    return `You are an expert TypeScript developer specializing in the Clawtrade Skill SDK.

## Skill SDK API

Skills are defined using the \`defineSkill()\` function which accepts a \`SkillDefinition\`:

\`\`\`typescript
interface SkillDefinition {
  name: string;           // kebab-case skill name (e.g. "moving-average")
  version: string;        // semver (e.g. "1.0.0")
  description: string;    // human-readable description
  author?: string;        // optional author
  permissions: Permission[];
  tools: ToolDefinition[];
  onLoad?: () => Promise<void>;
  onUnload?: () => Promise<void>;
}
\`\`\`

### Permissions
Each permission has a \`type\` and optional \`scope\`:
- \`"market-data"\` - access to market data feeds
- \`"trade-execute"\` - execute trades
- \`"trade-read"\` - read trade/position data
- \`"memory-read"\` - read from persistent memory
- \`"memory-write"\` - write to persistent memory
- \`"llm-access"\` - call LLM APIs
- \`"network"\` - external network access

### Tools
\`\`\`typescript
interface ToolDefinition {
  name: string;
  description: string;
  parameters: ParameterDef[];
  handler: (params: Record<string, unknown>, ctx: SkillContext) => Promise<unknown>;
}

interface ParameterDef {
  name: string;
  type: "string" | "number" | "boolean" | "object";
  description: string;
  required?: boolean;
  default?: unknown;
}
\`\`\`

### SkillContext (available in handlers)
- \`ctx.log(msg)\` - log a message
- \`ctx.getMarketData(symbol)\` - get market data
- \`ctx.executeOrder(order)\` - execute a trade order
- \`ctx.getMemory(query)\` - query persistent memory
- \`ctx.callLLM(messages)\` - call the LLM

## Response Format

Respond with a JSON object wrapped in a \`\`\`json code block containing:
- \`definition\`: a valid SkillDefinition (with handler as a string placeholder "async (params, ctx) => { ... }")
- \`code\`: the full TypeScript source code for the skill
- \`explanation\`: a brief explanation of how the skill works
- \`suggestedTests\`: an array of test case descriptions`;
  }

  /**
   * Build the user prompt from the generation request.
   */
  buildUserPrompt(request: GenerationRequest): string {
    const name =
      request.name ?? SkillGenerator.generateName(request.description);
    let prompt = `Generate a Clawtrade skill with the following details:\n\nName: ${name}\nDescription: ${request.description}`;

    if (request.examples && request.examples.length > 0) {
      prompt += `\n\nExamples:\n${request.examples.map((e) => `- ${e}`).join("\n")}`;
    }

    if (request.constraints && request.constraints.length > 0) {
      prompt += `\n\nConstraints:\n${request.constraints.map((c) => `- ${c}`).join("\n")}`;
    }

    prompt +=
      "\n\nRespond with a JSON object in a ```json code block as described in the system prompt.";

    return prompt;
  }

  /**
   * Parse an LLM response string into a GenerationResult.
   * Handles both raw JSON and markdown-fenced code blocks.
   */
  parseResponse(response: string): GenerationResult {
    // Try to extract JSON from markdown code block first
    const codeBlockMatch = response.match(
      /```(?:json)?\s*\n?([\s\S]*?)\n?\s*```/,
    );
    const jsonStr = codeBlockMatch ? codeBlockMatch[1].trim() : response.trim();

    let parsed: Record<string, unknown>;
    try {
      parsed = JSON.parse(jsonStr);
    } catch {
      throw new Error(`Failed to parse LLM response as JSON: ${jsonStr.slice(0, 200)}`);
    }

    const def = parsed.definition as Record<string, unknown> | undefined;
    if (!def || typeof def !== "object") {
      throw new Error("Response missing 'definition' field");
    }

    // Reconstruct SkillDefinition with placeholder handlers
    const tools: ToolDefinition[] = [];
    if (Array.isArray(def.tools)) {
      for (const t of def.tools as Array<Record<string, unknown>>) {
        tools.push({
          name: String(t.name ?? ""),
          description: String(t.description ?? ""),
          parameters: Array.isArray(t.parameters)
            ? (t.parameters as ParameterDef[])
            : [],
          handler: async () => {
            throw new Error("Generated handler placeholder - not yet implemented");
          },
        });
      }
    }

    const permissions: Permission[] = [];
    if (Array.isArray(def.permissions)) {
      for (const p of def.permissions as Array<Record<string, unknown>>) {
        const perm: Permission = { type: String(p.type ?? "") as Permission["type"] };
        if (p.scope) perm.scope = String(p.scope);
        permissions.push(perm);
      }
    }

    const definition: SkillDefinition = {
      name: String(def.name ?? ""),
      version: String(def.version ?? "1.0.0"),
      description: String(def.description ?? ""),
      author: def.author ? String(def.author) : undefined,
      permissions,
      tools,
    };

    return {
      definition,
      code: String(parsed.code ?? ""),
      explanation: String(parsed.explanation ?? ""),
      suggestedTests: Array.isArray(parsed.suggestedTests)
        ? (parsed.suggestedTests as string[])
        : [],
    };
  }

  /**
   * Validate a generated skill definition. Returns a list of error strings.
   * An empty array means the definition is valid.
   */
  validateDefinition(def: unknown): string[] {
    const errors: string[] = [];

    if (!def || typeof def !== "object") {
      errors.push("Definition must be an object");
      return errors;
    }

    const d = def as Record<string, unknown>;

    // Required string fields
    if (!d.name || typeof d.name !== "string") {
      errors.push("Missing or invalid 'name'");
    }
    if (!d.version || typeof d.version !== "string") {
      errors.push("Missing or invalid 'version'");
    }
    if (!d.description || typeof d.description !== "string") {
      errors.push("Missing or invalid 'description'");
    }

    // Tools
    if (!Array.isArray(d.tools)) {
      errors.push("'tools' must be an array");
    } else if (d.tools.length === 0) {
      errors.push("Skill must define at least one tool");
    } else {
      for (let i = 0; i < d.tools.length; i++) {
        const tool = d.tools[i] as Record<string, unknown>;
        if (!tool.name || typeof tool.name !== "string") {
          errors.push(`Tool ${i}: missing or invalid 'name'`);
        }
        if (!tool.description || typeof tool.description !== "string") {
          errors.push(`Tool ${i}: missing or invalid 'description'`);
        }
        if (typeof tool.handler !== "function") {
          errors.push(`Tool ${i}: missing handler function`);
        }
      }
    }

    // Permissions
    if (!Array.isArray(d.permissions)) {
      errors.push("'permissions' must be an array");
    } else {
      for (let i = 0; i < d.permissions.length; i++) {
        const perm = d.permissions[i] as Record<string, unknown>;
        if (
          !perm.type ||
          !VALID_PERMISSION_TYPES.includes(perm.type as Permission["type"])
        ) {
          errors.push(
            `Permission ${i}: invalid type '${String(perm.type)}'. Must be one of: ${VALID_PERMISSION_TYPES.join(", ")}`,
          );
        }
      }
    }

    return errors;
  }

  /**
   * Generate a kebab-case skill name from a natural language description.
   * Removes stop words, lowercases, and truncates to 30 characters.
   */
  static generateName(description: string): string {
    const words = description
      .toLowerCase()
      .replace(/[^a-z0-9\s]/g, "")
      .split(/\s+/)
      .filter((w) => w.length > 0 && !STOP_WORDS.has(w));

    if (words.length === 0) {
      return "unnamed-skill";
    }

    let name = words.join("-");

    // Truncate to 30 chars on word boundary
    if (name.length > 30) {
      name = name.slice(0, 30);
      // Don't end on a partial word (trailing hyphen)
      const lastHyphen = name.lastIndexOf("-");
      if (lastHyphen > 0 && name.endsWith("-")) {
        name = name.slice(0, lastHyphen);
      }
      // Remove trailing hyphen
      name = name.replace(/-$/, "");
    }

    return name;
  }
}
