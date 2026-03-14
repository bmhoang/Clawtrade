package subagent

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDevilsAdvocate_Name(t *testing.T) {
	da := NewDevilsAdvocate(DevilsAdvocateConfig{})
	if da.Name() != "devils-advocate" {
		t.Errorf("expected 'devils-advocate', got %q", da.Name())
	}
}

func TestDevilsAdvocate_BuildPrompt(t *testing.T) {
	da := NewDevilsAdvocate(DevilsAdvocateConfig{})
	thesis := "BTC is bullish because of strong 4h structure"
	prompt := da.buildCounterPrompt(thesis)
	if !strings.Contains(prompt, "WRONG") {
		t.Error("prompt should instruct to find reasons thesis is wrong")
	}
	if !strings.Contains(prompt, thesis) {
		t.Error("prompt should contain the original thesis")
	}
}

func TestDevilsAdvocate_Status(t *testing.T) {
	da := NewDevilsAdvocate(DevilsAdvocateConfig{})
	status := da.Status()

	if status.Name != "devils-advocate" {
		t.Errorf("expected status name 'devils-advocate', got %q", status.Name)
	}
	if status.Running {
		t.Error("expected running to be false before Start")
	}
	if status.RunCount != 0 {
		t.Errorf("expected run count 0, got %d", status.RunCount)
	}
}

func TestDevilsAdvocate_StopBeforeStart(t *testing.T) {
	da := NewDevilsAdvocate(DevilsAdvocateConfig{})
	err := da.Stop()
	if err != nil {
		t.Errorf("Stop before Start should not error, got %v", err)
	}
}

func TestDevilsAdvocate_StartStop(t *testing.T) {
	bus := NewEventBus()
	da := NewDevilsAdvocate(DevilsAdvocateConfig{Bus: bus})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- da.Start(ctx)
	}()

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	status := da.Status()
	if !status.Running {
		t.Error("expected running to be true after Start")
	}

	err := da.Stop()
	if err != nil {
		t.Errorf("Stop returned error: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Start returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after Stop")
	}

	status = da.Status()
	if status.Running {
		t.Error("expected running to be false after Stop")
	}
}

func TestDevilsAdvocate_SkipsEmptyThesis(t *testing.T) {
	bus := NewEventBus()
	counterCh := bus.Subscribe("counter_analysis")

	da := NewDevilsAdvocate(DevilsAdvocateConfig{Bus: bus})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go da.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	// Publish an analysis event with no synthesis field
	bus.Publish(Event{
		Type:   "analysis",
		Source: "market-analyst",
		Symbol: "BTC/USDT",
		Data:   map[string]any{},
	})

	// Should not produce a counter_analysis event
	select {
	case <-counterCh:
		t.Error("should not publish counter_analysis for empty thesis")
	case <-time.After(200 * time.Millisecond):
		// expected
	}

	da.Stop()
}

func TestDevilsAdvocate_ProcessesAnalysisEvent(t *testing.T) {
	bus := NewEventBus()
	counterCh := bus.Subscribe("counter_analysis")

	// Create a fake LLM caller that will fail (no real server),
	// but we can test the wiring by checking error count.
	// Instead, we test prompt building and event handling separately.
	// For a full integration test we'd mock the LLM.

	da := NewDevilsAdvocate(DevilsAdvocateConfig{
		Bus: bus,
		LLM: nil, // nil LLM means we skip calling
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go da.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	bus.Publish(Event{
		Type:   "analysis",
		Source: "market-analyst",
		Symbol: "BTC/USDT",
		Data: map[string]any{
			"synthesis": "BTC looks bullish with strong support at 60k",
		},
	})

	// With nil LLM, we expect an error to be recorded but no counter_analysis event
	select {
	case <-counterCh:
		t.Error("should not publish counter_analysis with nil LLM")
	case <-time.After(200 * time.Millisecond):
		// expected
	}

	status := da.Status()
	if status.ErrorCount == 0 {
		t.Error("expected error count > 0 when LLM is nil")
	}

	da.Stop()
}

func TestDevilsAdvocate_BuildPromptContainsRequiredSections(t *testing.T) {
	da := NewDevilsAdvocate(DevilsAdvocateConfig{})
	thesis := "ETH breaking out above 4000 with massive volume"
	prompt := da.buildCounterPrompt(thesis)

	requiredParts := []string{
		"Devil's Advocate",
		"WRONG",
		thesis,
		"confirmation bias",
		"counter_confidence",
		"risks",
		"failure_scenarios",
		"verdict",
	}

	for _, part := range requiredParts {
		if !strings.Contains(prompt, part) {
			t.Errorf("prompt missing required part: %q", part)
		}
	}
}
