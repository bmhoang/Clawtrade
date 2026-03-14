package subagent

import (
	"context"
	"testing"
	"time"
)

type mockSubAgent struct {
	name    string
	started bool
	stopped bool
}

func (m *mockSubAgent) Name() string { return m.name }
func (m *mockSubAgent) Start(ctx context.Context) error {
	m.started = true
	<-ctx.Done()
	return nil
}
func (m *mockSubAgent) Stop() error {
	m.stopped = true
	return nil
}
func (m *mockSubAgent) Status() SubAgentStatus {
	return SubAgentStatus{Name: m.name, Running: m.started && !m.stopped}
}

func TestManager_RegisterAndStart(t *testing.T) {
	mgr := NewAgentManager(NewEventBus())
	agent := &mockSubAgent{name: "test-agent"}
	mgr.Register(agent)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr.StartAll(ctx)
	time.Sleep(50 * time.Millisecond)

	statuses := mgr.Statuses()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if !agent.started {
		t.Error("agent should be started")
	}

	mgr.StopAll()
	time.Sleep(50 * time.Millisecond)
}

func TestManager_GetEventBus(t *testing.T) {
	bus := NewEventBus()
	mgr := NewAgentManager(bus)
	if mgr.Bus() != bus {
		t.Error("expected same bus")
	}
}
