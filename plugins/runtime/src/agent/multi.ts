// Multi-Agent System: Conductor + specialist agents

import type { LLMAdapter, LLMMessage } from '../llm/adapter';

// Agent role definitions
export type AgentRole = 'conductor' | 'analyst' | 'trader' | 'risk_manager';

// Specialist agent with a specific role and system prompt
export interface SpecialistConfig {
  role: AgentRole;
  name: string;
  systemPrompt: string;
  capabilities: string[];
}

// Message between agents
export interface AgentMessage {
  from: AgentRole;
  to: AgentRole | 'all';
  content: string;
  type: 'request' | 'response' | 'broadcast';
  metadata?: Record<string, unknown>;
}

// Specialist agent that handles a specific domain
export class SpecialistAgent {
  private config: SpecialistConfig;
  private llm: LLMAdapter;
  private history: LLMMessage[];

  constructor(config: SpecialistConfig, llm: LLMAdapter) {
    this.config = config;
    this.llm = llm;
    this.history = [];
  }

  async process(message: AgentMessage): Promise<AgentMessage> {
    const messages: LLMMessage[] = [
      { role: 'system', content: this.config.systemPrompt },
      ...this.history,
      { role: 'user', content: message.content },
    ];

    const response = await this.llm.chat(messages);

    this.history.push({ role: 'user', content: message.content });
    this.history.push({ role: 'assistant', content: response.content });

    return {
      from: this.config.role,
      to: message.from,
      content: response.content,
      type: 'response',
      metadata: {
        model: response.model,
        usage: response.usage,
      },
    };
  }

  getRole(): AgentRole {
    return this.config.role;
  }

  getName(): string {
    return this.config.name;
  }

  getCapabilities(): string[] {
    return [...this.config.capabilities];
  }

  clearHistory(): void {
    this.history = [];
  }
}

// Routing keyword maps
const ROUTING_KEYWORDS: Record<AgentRole, string[]> = {
  analyst: ['analysis', 'chart', 'indicator', 'trend', 'rsi', 'macd', 'pattern'],
  trader: ['buy', 'sell', 'order', 'position', 'entry', 'exit', 'trade'],
  risk_manager: ['risk', 'stop loss', 'exposure', 'drawdown', 'position size'],
  conductor: [],
};

// Conductor orchestrates specialist agents
export class Conductor {
  private specialists: Map<AgentRole, SpecialistAgent> = new Map();
  private llm: LLMAdapter;
  private conversationLog: AgentMessage[] = [];

  constructor(llm: LLMAdapter) {
    this.llm = llm;
  }

  registerSpecialist(agent: SpecialistAgent): void {
    this.specialists.set(agent.getRole(), agent);
  }

  async processRequest(userMessage: string): Promise<string> {
    // 1. Determine which specialists to consult
    const roles = this.determineRouting(userMessage);

    // 2. Create request messages and route to specialists
    const responses: AgentMessage[] = [];
    for (const role of roles) {
      const request: AgentMessage = {
        from: 'conductor',
        to: role,
        content: userMessage,
        type: 'request',
      };
      this.conversationLog.push(request);

      const response = await this.routeTo(role, request);
      this.conversationLog.push(response);
      responses.push(response);
    }

    // 3. Synthesize responses
    if (responses.length === 1) {
      return responses[0].content;
    }

    // Multiple specialist responses - ask conductor LLM to synthesize
    const synthesisPrompt = responses
      .map((r) => `[${r.from}]: ${r.content}`)
      .join('\n\n');

    const messages: LLMMessage[] = [
      {
        role: 'system',
        content:
          'You are a conductor agent that synthesizes responses from specialist agents into a coherent answer. Combine insights from all specialists into a unified response.',
      },
      {
        role: 'user',
        content: `User asked: "${userMessage}"\n\nSpecialist responses:\n${synthesisPrompt}\n\nPlease synthesize these into a single coherent response.`,
      },
    ];

    const synthesis = await this.llm.chat(messages);
    return synthesis.content;
  }

  private async routeTo(role: AgentRole, message: AgentMessage): Promise<AgentMessage> {
    const specialist = this.specialists.get(role);
    if (!specialist) {
      return {
        from: role,
        to: 'conductor',
        content: `No specialist registered for role: ${role}`,
        type: 'response',
      };
    }
    return specialist.process(message);
  }

  async broadcast(message: AgentMessage): Promise<AgentMessage[]> {
    const responses: AgentMessage[] = [];
    for (const [, specialist] of this.specialists) {
      const broadcastMsg: AgentMessage = {
        ...message,
        to: specialist.getRole(),
        type: 'broadcast',
      };
      const response = await specialist.process(broadcastMsg);
      responses.push(response);
    }
    return responses;
  }

  determineRouting(message: string): AgentRole[] {
    const lower = message.toLowerCase();
    const matched: AgentRole[] = [];

    for (const role of ['analyst', 'trader', 'risk_manager'] as AgentRole[]) {
      const keywords = ROUTING_KEYWORDS[role];
      if (keywords.some((kw) => lower.includes(kw))) {
        matched.push(role);
      }
    }

    // Default to analyst if no keywords matched
    if (matched.length === 0) {
      matched.push('analyst');
    }

    return matched;
  }

  getConversationLog(): AgentMessage[] {
    return [...this.conversationLog];
  }

  clearLog(): void {
    this.conversationLog = [];
  }

  listSpecialists(): SpecialistConfig[] {
    const configs: SpecialistConfig[] = [];
    for (const [, specialist] of this.specialists) {
      configs.push({
        role: specialist.getRole(),
        name: specialist.getName(),
        systemPrompt: '',
        capabilities: specialist.getCapabilities(),
      });
    }
    return configs;
  }
}

// Factory function to create default specialist setup
export function createDefaultAgents(llm: LLMAdapter): Conductor {
  const conductor = new Conductor(llm);

  conductor.registerSpecialist(
    new SpecialistAgent(
      {
        role: 'analyst',
        name: 'Market Analyst',
        systemPrompt:
          'You are a market analyst specializing in technical and fundamental analysis. Provide detailed market insights, identify trends, and analyze chart patterns. Use indicators like RSI, MACD, and moving averages to support your analysis. Always present balanced views with both bullish and bearish scenarios.',
        capabilities: ['technical_analysis', 'fundamental_analysis', 'market_overview'],
      },
      llm
    )
  );

  conductor.registerSpecialist(
    new SpecialistAgent(
      {
        role: 'trader',
        name: 'Trade Executor',
        systemPrompt:
          'You are a trade execution specialist. Help with order placement, position management, and entry/exit strategies. Provide specific trade setups with clear entry points, targets, and stop losses. Focus on execution quality and timing.',
        capabilities: ['order_placement', 'position_management', 'entry_exit_strategy'],
      },
      llm
    )
  );

  conductor.registerSpecialist(
    new SpecialistAgent(
      {
        role: 'risk_manager',
        name: 'Risk Manager',
        systemPrompt:
          'You are a risk management specialist. Evaluate position sizing, portfolio exposure, and drawdown limits. Ensure all trades comply with risk parameters. Recommend stop loss levels and position sizes based on account risk tolerance. Always prioritize capital preservation.',
        capabilities: ['position_sizing', 'risk_assessment', 'portfolio_risk'],
      },
      llm
    )
  );

  return conductor;
}
