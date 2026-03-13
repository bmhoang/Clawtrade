package memory

import (
	"testing"
	"time"
)

func TestTemporalAnalyzer_ByHour(t *testing.T) {
	ta := NewTemporalAnalyzer()

	// Add trades at different hours
	base := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC) // Monday
	for i := 0; i < 10; i++ {
		ta.AddTrade(TradeRecord{Timestamp: base.Add(time.Duration(9) * time.Hour), PnL: 100}) // 9am wins
	}
	for i := 0; i < 10; i++ {
		ta.AddTrade(TradeRecord{Timestamp: base.Add(time.Duration(15) * time.Hour), PnL: -50}) // 3pm losses
	}

	stats := ta.ByHour()
	if len(stats) != 2 {
		t.Errorf("expected 2 hour slots, got %d", len(stats))
	}
}

func TestTemporalAnalyzer_ByDayOfWeek(t *testing.T) {
	ta := NewTemporalAnalyzer()

	// Monday wins
	monday := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		ta.AddTrade(TradeRecord{Timestamp: monday, PnL: 100})
	}

	// Friday losses
	friday := time.Date(2024, 1, 19, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		ta.AddTrade(TradeRecord{Timestamp: friday, PnL: -80})
	}

	stats := ta.ByDayOfWeek()
	if len(stats) != 2 {
		t.Errorf("expected 2 day slots, got %d", len(stats))
	}
}

func TestTemporalAnalyzer_FindPatterns(t *testing.T) {
	ta := NewTemporalAnalyzer()

	monday := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		ta.AddTrade(TradeRecord{Timestamp: monday, PnL: 100})
	}

	friday := time.Date(2024, 1, 19, 15, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		ta.AddTrade(TradeRecord{Timestamp: friday, PnL: -50})
	}

	patterns := ta.FindPatterns(5)
	if len(patterns) == 0 {
		t.Error("expected at least one pattern")
	}

	hasBest := false
	hasWorst := false
	for _, p := range patterns {
		if p.Type == "best_hour" || p.Type == "best_day" {
			hasBest = true
		}
		if p.Type == "worst_hour" || p.Type == "worst_day" {
			hasWorst = true
		}
	}
	if !hasBest {
		t.Error("expected a best pattern")
	}
	if !hasWorst {
		t.Error("expected a worst pattern")
	}
}

func TestTemporalAnalyzer_TradeCount(t *testing.T) {
	ta := NewTemporalAnalyzer()
	ta.AddTrade(TradeRecord{Timestamp: time.Now(), PnL: 50})
	ta.AddTrade(TradeRecord{Timestamp: time.Now(), PnL: -30})
	if ta.TradeCount() != 2 {
		t.Errorf("expected 2 trades, got %d", ta.TradeCount())
	}
}
