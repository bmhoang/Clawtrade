package memory

import (
	"sync"
	"time"
)

// EmotionalState represents detected trading psychology states
type EmotionalState string

const (
	StateCAlm  EmotionalState = "CALM"
	StateFOMO  EmotionalState = "FOMO"
	StateFEAR  EmotionalState = "FEAR"
	StateTILT  EmotionalState = "TILT"
	StateGREED EmotionalState = "GREED"
)

// BehaviorSignal represents a trading behavior event
type BehaviorSignal struct {
	Type      string    `json:"type"`      // "trade", "cancel", "size_increase", "rapid_entry", "loss"
	Value     float64   `json:"value"`     // magnitude
	Timestamp time.Time `json:"timestamp"`
}

// StateScore holds a state and its confidence
type StateScore struct {
	State      EmotionalState `json:"state"`
	Confidence float64        `json:"confidence"` // 0.0 - 1.0
}

// EmotionalTracker monitors trading behavior for emotional patterns
type EmotionalTracker struct {
	mu             sync.RWMutex
	signals        []BehaviorSignal
	windowDuration time.Duration
	currentState   EmotionalState
}

func NewEmotionalTracker(windowMinutes int) *EmotionalTracker {
	if windowMinutes <= 0 {
		windowMinutes = 60
	}
	return &EmotionalTracker{
		signals:        make([]BehaviorSignal, 0),
		windowDuration: time.Duration(windowMinutes) * time.Minute,
		currentState:   StateCAlm,
	}
}

// RecordSignal adds a behavior signal
func (e *EmotionalTracker) RecordSignal(signalType string, value float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.signals = append(e.signals, BehaviorSignal{
		Type:      signalType,
		Value:     value,
		Timestamp: time.Now(),
	})

	e.pruneOldSignals()
}

// RecordSignalAt adds a signal at a specific time (for testing)
func (e *EmotionalTracker) RecordSignalAt(signalType string, value float64, t time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.signals = append(e.signals, BehaviorSignal{
		Type:      signalType,
		Value:     value,
		Timestamp: t,
	})
}

// Analyze returns all emotional states with confidence scores
func (e *EmotionalTracker) Analyze() []StateScore {
	e.mu.RLock()
	defer e.mu.RUnlock()

	now := time.Now()
	recent := e.getRecentSignals(now)

	scores := []StateScore{
		{State: StateFOMO, Confidence: e.detectFOMO(recent)},
		{State: StateFEAR, Confidence: e.detectFEAR(recent)},
		{State: StateTILT, Confidence: e.detectTILT(recent)},
		{State: StateGREED, Confidence: e.detectGREED(recent)},
	}

	// If no strong emotion detected, it's CALM
	maxConf := 0.0
	for _, s := range scores {
		if s.Confidence > maxConf {
			maxConf = s.Confidence
		}
	}

	calmConf := 1.0 - maxConf
	if calmConf < 0 {
		calmConf = 0
	}
	scores = append(scores, StateScore{State: StateCAlm, Confidence: calmConf})

	return scores
}

// GetDominantState returns the strongest emotional state
func (e *EmotionalTracker) GetDominantState() StateScore {
	scores := e.Analyze()
	best := StateScore{State: StateCAlm, Confidence: 0}
	for _, s := range scores {
		if s.Confidence > best.Confidence {
			best = s
		}
	}
	return best
}

// SignalCount returns the number of signals in the current window
func (e *EmotionalTracker) SignalCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.signals)
}

func (e *EmotionalTracker) getRecentSignals(now time.Time) []BehaviorSignal {
	cutoff := now.Add(-e.windowDuration)
	var recent []BehaviorSignal
	for _, s := range e.signals {
		if s.Timestamp.After(cutoff) {
			recent = append(recent, s)
		}
	}
	return recent
}

func (e *EmotionalTracker) pruneOldSignals() {
	cutoff := time.Now().Add(-e.windowDuration * 2) // keep 2x window for analysis
	var pruned []BehaviorSignal
	for _, s := range e.signals {
		if s.Timestamp.After(cutoff) {
			pruned = append(pruned, s)
		}
	}
	e.signals = pruned
}

// FOMO: rapid entries, size increases after price rises
func (e *EmotionalTracker) detectFOMO(signals []BehaviorSignal) float64 {
	rapidEntries := 0
	sizeIncreases := 0

	for _, s := range signals {
		switch s.Type {
		case "rapid_entry":
			rapidEntries++
		case "size_increase":
			sizeIncreases++
		}
	}

	score := 0.0
	if rapidEntries >= 3 {
		score += 0.4
	} else if rapidEntries >= 1 {
		score += float64(rapidEntries) * 0.15
	}

	if sizeIncreases >= 2 {
		score += 0.3
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}

// FEAR: excessive cancels, reduced position sizes, no new trades
func (e *EmotionalTracker) detectFEAR(signals []BehaviorSignal) float64 {
	cancels := 0
	sizeDecreases := 0

	for _, s := range signals {
		switch s.Type {
		case "cancel":
			cancels++
		case "size_decrease":
			sizeDecreases++
		}
	}

	score := 0.0
	if cancels >= 3 {
		score += 0.5
	} else if cancels >= 1 {
		score += float64(cancels) * 0.15
	}

	if sizeDecreases >= 2 {
		score += 0.3
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}

// TILT: trades immediately after losses, increasing size after losses
func (e *EmotionalTracker) detectTILT(signals []BehaviorSignal) float64 {
	consecutiveLosses := 0
	lossFollowedByTrade := 0
	maxConsecutive := 0

	for i, s := range signals {
		if s.Type == "loss" {
			consecutiveLosses++
			if consecutiveLosses > maxConsecutive {
				maxConsecutive = consecutiveLosses
			}
			// Check if next signal is a rapid trade
			if i+1 < len(signals) && signals[i+1].Type == "rapid_entry" {
				lossFollowedByTrade++
			}
		} else if s.Type != "rapid_entry" {
			consecutiveLosses = 0
		}
	}

	score := 0.0
	if maxConsecutive >= 3 {
		score += 0.5
	} else if maxConsecutive >= 2 {
		score += 0.3
	}

	if lossFollowedByTrade >= 2 {
		score += 0.4
	} else if lossFollowedByTrade >= 1 {
		score += 0.2
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}

// GREED: holding too long, not taking profits, increasing leverage
func (e *EmotionalTracker) detectGREED(signals []BehaviorSignal) float64 {
	sizeIncreases := 0
	leverageIncreases := 0

	for _, s := range signals {
		switch s.Type {
		case "size_increase":
			if s.Value > 1.5 { // significant size increase
				sizeIncreases++
			}
		case "leverage_increase":
			leverageIncreases++
		}
	}

	score := 0.0
	if sizeIncreases >= 2 {
		score += 0.4
	}
	if leverageIncreases >= 2 {
		score += 0.4
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}
