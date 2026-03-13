package memory

import (
	"testing"
	"time"
)

func TestEmotionalTracker_CalmByDefault(t *testing.T) {
	tracker := NewEmotionalTracker(60)
	state := tracker.GetDominantState()
	if state.State != StateCAlm {
		t.Errorf("expected CALM, got %s", state.State)
	}
}

func TestEmotionalTracker_DetectFOMO(t *testing.T) {
	tracker := NewEmotionalTracker(60)
	now := time.Now()

	tracker.RecordSignalAt("rapid_entry", 1.0, now.Add(-5*time.Minute))
	tracker.RecordSignalAt("rapid_entry", 1.0, now.Add(-4*time.Minute))
	tracker.RecordSignalAt("rapid_entry", 1.0, now.Add(-3*time.Minute))
	tracker.RecordSignalAt("size_increase", 2.0, now.Add(-2*time.Minute))
	tracker.RecordSignalAt("size_increase", 2.0, now.Add(-1*time.Minute))

	state := tracker.GetDominantState()
	if state.State != StateFOMO {
		t.Errorf("expected FOMO, got %s (confidence: %.2f)", state.State, state.Confidence)
	}
	if state.Confidence < 0.5 {
		t.Errorf("expected high FOMO confidence, got %.2f", state.Confidence)
	}
}

func TestEmotionalTracker_DetectTILT(t *testing.T) {
	tracker := NewEmotionalTracker(60)
	now := time.Now()

	tracker.RecordSignalAt("loss", 100.0, now.Add(-10*time.Minute))
	tracker.RecordSignalAt("rapid_entry", 1.0, now.Add(-9*time.Minute))
	tracker.RecordSignalAt("loss", 150.0, now.Add(-8*time.Minute))
	tracker.RecordSignalAt("rapid_entry", 1.0, now.Add(-7*time.Minute))
	tracker.RecordSignalAt("loss", 200.0, now.Add(-6*time.Minute))

	state := tracker.GetDominantState()
	if state.State != StateTILT {
		t.Errorf("expected TILT, got %s (confidence: %.2f)", state.State, state.Confidence)
	}
}

func TestEmotionalTracker_DetectFEAR(t *testing.T) {
	tracker := NewEmotionalTracker(60)
	now := time.Now()

	tracker.RecordSignalAt("cancel", 1.0, now.Add(-5*time.Minute))
	tracker.RecordSignalAt("cancel", 1.0, now.Add(-4*time.Minute))
	tracker.RecordSignalAt("cancel", 1.0, now.Add(-3*time.Minute))
	tracker.RecordSignalAt("size_decrease", 0.5, now.Add(-2*time.Minute))
	tracker.RecordSignalAt("size_decrease", 0.5, now.Add(-1*time.Minute))

	state := tracker.GetDominantState()
	if state.State != StateFEAR {
		t.Errorf("expected FEAR, got %s (confidence: %.2f)", state.State, state.Confidence)
	}
}

func TestEmotionalTracker_SignalCount(t *testing.T) {
	tracker := NewEmotionalTracker(60)
	tracker.RecordSignal("trade", 1.0)
	tracker.RecordSignal("trade", 1.0)
	if tracker.SignalCount() != 2 {
		t.Errorf("expected 2 signals, got %d", tracker.SignalCount())
	}
}
