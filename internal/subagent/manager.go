package subagent

import (
	"context"
	"sync"
)

// AgentManager handles lifecycle management for all registered sub-agents.
// It starts, stops, and reports status for the agent fleet.
type AgentManager struct {
	agents map[string]SubAgent
	bus    *EventBus
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
}

// NewAgentManager creates an AgentManager wired to the given EventBus.
func NewAgentManager(bus *EventBus) *AgentManager {
	return &AgentManager{
		agents: make(map[string]SubAgent),
		bus:    bus,
	}
}

// Register adds a sub-agent to the manager, keyed by its Name().
func (m *AgentManager) Register(agent SubAgent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.agents[agent.Name()] = agent
}

// StartAll launches every registered agent in its own goroutine.
// The provided ctx is wrapped in a child context so StopAll can cancel it.
func (m *AgentManager) StartAll(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	childCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel

	for _, agent := range m.agents {
		m.wg.Add(1)
		go func(a SubAgent) {
			defer m.wg.Done()
			_ = a.Start(childCtx)
		}(agent)
	}
}

// StopAll cancels the shared context, calls Stop() on every agent,
// and waits for all goroutines to finish.
func (m *AgentManager) StopAll() {
	m.mu.RLock()
	cancel := m.cancel
	agents := make([]SubAgent, 0, len(m.agents))
	for _, a := range m.agents {
		agents = append(agents, a)
	}
	m.mu.RUnlock()

	if cancel != nil {
		cancel()
	}
	for _, a := range agents {
		_ = a.Stop()
	}
	m.wg.Wait()
}

// Statuses returns the current status of every registered agent.
func (m *AgentManager) Statuses() []SubAgentStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]SubAgentStatus, 0, len(m.agents))
	for _, a := range m.agents {
		out = append(out, a.Status())
	}
	return out
}

// Bus exposes the EventBus so callers can subscribe or publish events.
func (m *AgentManager) Bus() *EventBus {
	return m.bus
}
