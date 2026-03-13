import { describe, it, expect } from 'bun:test';
import type { LLMAdapter } from '../llm/adapter';
import {
  SpecialistAgent,
  Conductor,
  createDefaultAgents,
  type AgentMessage,
  type SpecialistConfig,
} from './multi';

const mockLLM: LLMAdapter = {
  async chat(messages) {
    const last = messages[messages.length - 1];
    return {
      content: `Response to: ${last.content}`,
      model: 'mock',
      usage: { inputTokens: 10, outputTokens: 20 },
    };
  },
  name() {
    return 'mock';
  },
};

describe('SpecialistAgent', () => {
  const config: SpecialistConfig = {
    role: 'analyst',
    name: 'Market Analyst',
    systemPrompt: 'You are a market analyst.',
    capabilities: ['technical_analysis', 'fundamental_analysis'],
  };

  it('returns response with correct role', async () => {
    const agent = new SpecialistAgent(config, mockLLM);
    const message: AgentMessage = {
      from: 'conductor',
      to: 'analyst',
      content: 'Analyze BTC trend',
      type: 'request',
    };

    const response = await agent.process(message);

    expect(response.from).toBe('analyst');
    expect(response.to).toBe('conductor');
    expect(response.type).toBe('response');
    expect(response.content).toContain('Response to:');
  });

  it('processes message and includes system prompt context', async () => {
    let capturedMessages: any[] = [];
    const capturingLLM: LLMAdapter = {
      async chat(messages) {
        capturedMessages = messages;
        return { content: 'ok', model: 'mock', usage: { inputTokens: 10, outputTokens: 20 } };
      },
      name() {
        return 'mock';
      },
    };

    const agent = new SpecialistAgent(config, capturingLLM);
    const message: AgentMessage = {
      from: 'conductor',
      to: 'analyst',
      content: 'Analyze BTC',
      type: 'request',
    };

    await agent.process(message);

    expect(capturedMessages[0].role).toBe('system');
    expect(capturedMessages[0].content).toBe('You are a market analyst.');
    expect(capturedMessages[capturedMessages.length - 1].content).toBe('Analyze BTC');
  });

  it('exposes role, name, and capabilities', () => {
    const agent = new SpecialistAgent(config, mockLLM);
    expect(agent.getRole()).toBe('analyst');
    expect(agent.getName()).toBe('Market Analyst');
    expect(agent.getCapabilities()).toEqual(['technical_analysis', 'fundamental_analysis']);
  });

  it('clearHistory resets conversation', async () => {
    let messageCount = 0;
    const countingLLM: LLMAdapter = {
      async chat(messages) {
        messageCount = messages.length;
        return { content: 'ok', model: 'mock', usage: { inputTokens: 10, outputTokens: 20 } };
      },
      name() {
        return 'mock';
      },
    };

    const agent = new SpecialistAgent(config, countingLLM);
    const msg: AgentMessage = { from: 'conductor', to: 'analyst', content: 'test', type: 'request' };

    await agent.process(msg);
    await agent.process(msg);
    // system + 2 history entries (user+assistant) + current user = 4 messages
    expect(messageCount).toBe(4);

    agent.clearHistory();
    await agent.process(msg);
    // system + current user = 2 messages
    expect(messageCount).toBe(2);
  });
});

describe('Conductor', () => {
  function makeConductor(): Conductor {
    const conductor = new Conductor(mockLLM);
    conductor.registerSpecialist(
      new SpecialistAgent(
        { role: 'analyst', name: 'Analyst', systemPrompt: 'analyst', capabilities: ['analysis'] },
        mockLLM
      )
    );
    conductor.registerSpecialist(
      new SpecialistAgent(
        { role: 'trader', name: 'Trader', systemPrompt: 'trader', capabilities: ['trading'] },
        mockLLM
      )
    );
    conductor.registerSpecialist(
      new SpecialistAgent(
        {
          role: 'risk_manager',
          name: 'Risk Manager',
          systemPrompt: 'risk',
          capabilities: ['risk'],
        },
        mockLLM
      )
    );
    return conductor;
  }

  it('registerSpecialist and listSpecialists', () => {
    const conductor = makeConductor();
    const specialists = conductor.listSpecialists();

    expect(specialists.length).toBe(3);
    const roles = specialists.map((s) => s.role);
    expect(roles).toContain('analyst');
    expect(roles).toContain('trader');
    expect(roles).toContain('risk_manager');
  });

  it('determineRouting - analyst keywords', () => {
    const conductor = makeConductor();
    expect(conductor.determineRouting('Show me the RSI analysis')).toContain('analyst');
    expect(conductor.determineRouting('What is the MACD trend?')).toContain('analyst');
    expect(conductor.determineRouting('chart pattern')).toContain('analyst');
  });

  it('determineRouting - trader keywords', () => {
    const conductor = makeConductor();
    expect(conductor.determineRouting('Buy BTC now')).toContain('trader');
    expect(conductor.determineRouting('Sell my position')).toContain('trader');
    expect(conductor.determineRouting('Place an order')).toContain('trader');
  });

  it('determineRouting - risk keywords', () => {
    const conductor = makeConductor();
    expect(conductor.determineRouting('What is my risk exposure?')).toContain('risk_manager');
    expect(conductor.determineRouting('Set a stop loss')).toContain('risk_manager');
    expect(conductor.determineRouting('Check drawdown')).toContain('risk_manager');
  });

  it('determineRouting - defaults to analyst for ambiguous', () => {
    const conductor = makeConductor();
    const roles = conductor.determineRouting('Hello, how are you?');
    expect(roles).toEqual(['analyst']);
  });

  it('processRequest routes to correct specialist', async () => {
    const conductor = makeConductor();
    const result = await conductor.processRequest('Show me the RSI analysis');

    expect(result).toContain('Response to:');
    expect(result).toContain('RSI analysis');
  });

  it('broadcast sends to all specialists', async () => {
    const conductor = makeConductor();
    const message: AgentMessage = {
      from: 'conductor',
      to: 'all',
      content: 'Market update',
      type: 'broadcast',
    };

    const responses = await conductor.broadcast(message);

    expect(responses.length).toBe(3);
    const fromRoles = responses.map((r) => r.from);
    expect(fromRoles).toContain('analyst');
    expect(fromRoles).toContain('trader');
    expect(fromRoles).toContain('risk_manager');
  });

  it('getConversationLog tracks all messages', async () => {
    const conductor = makeConductor();
    expect(conductor.getConversationLog().length).toBe(0);

    await conductor.processRequest('Show me chart analysis');

    const log = conductor.getConversationLog();
    expect(log.length).toBeGreaterThanOrEqual(2); // at least request + response
    expect(log[0].type).toBe('request');
    expect(log[1].type).toBe('response');
  });

  it('clearLog resets conversation log', async () => {
    const conductor = makeConductor();
    await conductor.processRequest('Buy BTC');
    expect(conductor.getConversationLog().length).toBeGreaterThan(0);

    conductor.clearLog();
    expect(conductor.getConversationLog().length).toBe(0);
  });
});

describe('createDefaultAgents', () => {
  it('creates all 3 specialists', () => {
    const conductor = createDefaultAgents(mockLLM);
    const specialists = conductor.listSpecialists();

    expect(specialists.length).toBe(3);
    const roles = specialists.map((s) => s.role);
    expect(roles).toContain('analyst');
    expect(roles).toContain('trader');
    expect(roles).toContain('risk_manager');

    const names = specialists.map((s) => s.name);
    expect(names).toContain('Market Analyst');
    expect(names).toContain('Trade Executor');
    expect(names).toContain('Risk Manager');
  });
});
