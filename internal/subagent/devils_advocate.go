package subagent

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// DevilsAdvocateConfig holds the configuration for the Devil's Advocate sub-agent.
type DevilsAdvocateConfig struct {
	LLM *LLMCaller
	Bus *EventBus
}

// DevilsAdvocate is a sub-agent that subscribes to "analysis" events from the
// Market Analyst and counter-argues each thesis, looking for reasons why the
// trade could fail. It publishes "counter_analysis" events with its findings.
type DevilsAdvocate struct {
	cfg        DevilsAdvocateConfig
	running    bool
	lastRun    time.Time
	runCount   int
	errorCount int
	lastError  string
	cancel     context.CancelFunc
	mu         sync.RWMutex
}

// NewDevilsAdvocate creates a new DevilsAdvocate with the given configuration.
func NewDevilsAdvocate(cfg DevilsAdvocateConfig) *DevilsAdvocate {
	return &DevilsAdvocate{
		cfg: cfg,
	}
}

// Name returns the sub-agent name.
func (da *DevilsAdvocate) Name() string {
	return "devils-advocate"
}

// Start begins the Devil's Advocate event loop. It subscribes to "analysis"
// events on the EventBus and processes each one by counter-arguing the thesis.
// It blocks until the context is canceled or Stop is called.
func (da *DevilsAdvocate) Start(ctx context.Context) error {
	if da.cfg.Bus == nil {
		return fmt.Errorf("no event bus configured")
	}

	childCtx, cancel := context.WithCancel(ctx)

	da.mu.Lock()
	da.cancel = cancel
	da.running = true
	da.mu.Unlock()

	defer func() {
		da.mu.Lock()
		da.running = false
		da.mu.Unlock()
	}()

	analysisCh := da.cfg.Bus.Subscribe("analysis")

	for {
		select {
		case <-childCtx.Done():
			return nil
		case ev := <-analysisCh:
			da.handleAnalysis(childCtx, ev)
		}
	}
}

// Stop cancels the event loop and marks the agent as not running.
func (da *DevilsAdvocate) Stop() error {
	da.mu.Lock()
	defer da.mu.Unlock()
	if da.cancel != nil {
		da.cancel()
	}
	da.running = false
	return nil
}

// Status returns the current status of the Devil's Advocate sub-agent.
func (da *DevilsAdvocate) Status() SubAgentStatus {
	da.mu.RLock()
	defer da.mu.RUnlock()
	return SubAgentStatus{
		Name:       "devils-advocate",
		Running:    da.running,
		LastRun:    da.lastRun,
		RunCount:   da.runCount,
		ErrorCount: da.errorCount,
		LastError:  da.lastError,
	}
}

// handleAnalysis processes a single "analysis" event by extracting the thesis,
// calling the LLM with a counter-argument prompt, and publishing the result.
func (da *DevilsAdvocate) handleAnalysis(ctx context.Context, ev Event) {
	da.mu.Lock()
	da.runCount++
	da.lastRun = time.Now()
	da.mu.Unlock()

	// Extract thesis from event data
	synthesis, ok := ev.Data["synthesis"].(string)
	if !ok || synthesis == "" {
		return
	}

	if da.cfg.LLM == nil {
		da.mu.Lock()
		da.errorCount++
		da.lastError = "no LLM caller configured"
		da.mu.Unlock()
		return
	}

	prompt := da.buildCounterPrompt(synthesis)

	response, err := da.cfg.LLM.Call(ctx, prompt, synthesis)
	if err != nil {
		da.mu.Lock()
		da.errorCount++
		da.lastError = fmt.Sprintf("LLM call failed: %v", err)
		da.mu.Unlock()
		return
	}

	if da.cfg.Bus != nil {
		da.cfg.Bus.Publish(Event{
			Type:   "counter_analysis",
			Source: "devils-advocate",
			Symbol: ev.Symbol,
			Data: map[string]any{
				"counter": response,
			},
			Time: time.Now(),
		})
	}
}

// buildCounterPrompt returns the Devil's Advocate system prompt for a given thesis.
func (da *DevilsAdvocate) buildCounterPrompt(thesis string) string {
	return fmt.Sprintf(`You are a Devil's Advocate trader. Your ONLY job is to find reasons why the following trade thesis is WRONG.

## The Thesis
%s

## Your Task
1. Find every reason this trade could fail
2. Identify risks the analyst missed
3. Check for confirmation bias in the analysis
4. Look for contradicting evidence in the data
5. Rate your counter-argument strength: 0-100%%

## Rules
- You MUST argue against the thesis, even if you agree with it
- Be specific: cite price levels, patterns, data points
- Consider: macro conditions, funding rates, liquidation levels, historical failure rate of similar setups
- If the thesis is very strong, say so but still present risks

## Output as JSON
{"counter_bias":"...", "counter_confidence":N, "risks":["..."], "failure_scenarios":["..."], "verdict":"weak_thesis|moderate_thesis|strong_thesis"}`, thesis)
}
