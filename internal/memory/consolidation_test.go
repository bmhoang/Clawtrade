package memory

import (
	"path/filepath"
	"testing"

	"github.com/clawtrade/clawtrade/internal/database"
)

func newTestConsolidator(t *testing.T) (*Consolidator, *Store) {
	t.Helper()
	dir := t.TempDir()
	db, err := database.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := NewStore(db)
	return NewConsolidator(store), store
}

func TestConsolidator_NoEpisodes(t *testing.T) {
	c, _ := newTestConsolidator(t)
	rules, err := c.Run()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 rules from empty store, got %d", len(rules))
	}
}

func TestConsolidator_SymbolSideWinPattern(t *testing.T) {
	c, store := newTestConsolidator(t)
	c.WithMinEpisodes(3).WithMinWinRate(0.6)

	// Seed 4 winning BUY episodes on BTC/USDT
	for i := 0; i < 4; i++ {
		store.SaveEpisode(Episode{
			Symbol: "BTC/USDT", Side: "BUY",
			EntryPrice: 60000, ExitPrice: 61000,
			PnL: 100, Outcome: "win", Strategy: "momentum",
		})
	}
	// 1 losing BUY on BTC/USDT
	store.SaveEpisode(Episode{
		Symbol: "BTC/USDT", Side: "BUY",
		EntryPrice: 60000, ExitPrice: 59000,
		PnL: -100, Outcome: "loss", Strategy: "momentum",
	})

	rules, err := c.Run()
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, r := range rules {
		if r.Category == "symbol-side" && r.WinRate >= 0.6 {
			found = true
			if r.SourceCount != 5 {
				t.Errorf("expected 5 source episodes, got %d", r.SourceCount)
			}
			t.Logf("extracted rule: %s -> %s (confidence=%.2f, sources=%d)", r.Condition, r.Action, r.Confidence, r.SourceCount)
		}
	}
	if !found {
		t.Error("expected a symbol-side win rule to be extracted")
	}

	// Verify rules were stored
	stored, err := store.QueryRules("symbol-side", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) == 0 {
		t.Error("expected stored rules in database")
	}
}

func TestConsolidator_SymbolSideLossPattern(t *testing.T) {
	c, store := newTestConsolidator(t)
	c.WithMinEpisodes(3)

	// Seed 4 losing SELL episodes on ETH/USDT
	for i := 0; i < 4; i++ {
		store.SaveEpisode(Episode{
			Symbol: "ETH/USDT", Side: "SELL",
			EntryPrice: 3000, ExitPrice: 3100,
			PnL: -100, Outcome: "loss", Strategy: "reversal",
		})
	}
	// 1 winning
	store.SaveEpisode(Episode{
		Symbol: "ETH/USDT", Side: "SELL",
		EntryPrice: 3000, ExitPrice: 2900,
		PnL: 100, Outcome: "win", Strategy: "reversal",
	})

	rules, err := c.Run()
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, r := range rules {
		if r.Category == "symbol-side" && r.WinRate <= 0.4 {
			found = true
			t.Logf("extracted loss rule: %s -> %s (confidence=%.2f)", r.Condition, r.Action, r.Confidence)
		}
	}
	if !found {
		t.Error("expected a symbol-side loss rule to be extracted")
	}
}

func TestConsolidator_StrategyPattern(t *testing.T) {
	c, store := newTestConsolidator(t)
	c.WithMinEpisodes(3)

	// rsi-scalp strategy wins consistently
	for i := 0; i < 5; i++ {
		store.SaveEpisode(Episode{
			Symbol: "SOL/USDT", Side: "BUY",
			PnL: 50, Outcome: "win", Strategy: "rsi-scalp",
		})
	}

	rules, err := c.Run()
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, r := range rules {
		if r.Category == "strategy" {
			found = true
			if r.Confidence < 0.6 {
				t.Errorf("expected high confidence for winning strategy, got %.2f", r.Confidence)
			}
			t.Logf("strategy rule: %s -> %s", r.Condition, r.Action)
		}
	}
	if !found {
		t.Error("expected a strategy rule to be extracted")
	}
}

func TestConsolidator_ReasoningKeywordPattern(t *testing.T) {
	c, store := newTestConsolidator(t)
	c.WithMinEpisodes(3)

	// Episodes mentioning RSI that mostly win
	for i := 0; i < 4; i++ {
		store.SaveEpisode(Episode{
			Symbol: "BTC/USDT", Side: "BUY",
			PnL: 200, Outcome: "win",
			Reasoning: "RSI was oversold at 25, good entry",
			Strategy:  "indicator",
		})
	}
	store.SaveEpisode(Episode{
		Symbol: "BTC/USDT", Side: "BUY",
		PnL: -50, Outcome: "loss",
		Reasoning: "RSI signal was weak but entered anyway",
		Strategy:  "indicator",
	})

	rules, err := c.Run()
	if err != nil {
		t.Fatal(err)
	}

	foundRSI := false
	foundOversold := false
	for _, r := range rules {
		if r.Category == "indicator" {
			t.Logf("indicator rule: %s -> %s (confidence=%.2f)", r.Condition, r.Action, r.Confidence)
			if r.Condition == "reasoning mentions rsi" {
				foundRSI = true
			}
			if r.Condition == "reasoning mentions oversold" {
				foundOversold = true
			}
		}
	}
	if !foundRSI {
		t.Error("expected an RSI indicator rule")
	}
	if !foundOversold {
		t.Error("expected an oversold indicator rule")
	}
}

func TestConsolidator_OutcomeStreak(t *testing.T) {
	c, store := newTestConsolidator(t)
	c.WithMinEpisodes(3)

	// 4 consecutive losses on ETH/USDT
	for i := 0; i < 4; i++ {
		store.SaveEpisode(Episode{
			Symbol: "ETH/USDT", Side: "BUY",
			PnL: -50, Outcome: "loss", Strategy: "scalp",
		})
	}

	rules, err := c.Run()
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, r := range rules {
		if r.Category == "streak" {
			found = true
			if r.SourceCount < 3 {
				t.Errorf("expected streak count >= 3, got %d", r.SourceCount)
			}
			t.Logf("streak rule: %s -> %s (confidence=%.2f)", r.Condition, r.Action, r.Confidence)
		}
	}
	if !found {
		t.Error("expected a streak rule to be extracted")
	}
}

func TestConsolidator_BelowThreshold(t *testing.T) {
	c, store := newTestConsolidator(t)
	c.WithMinEpisodes(5) // high threshold

	// Only 3 episodes - below threshold
	for i := 0; i < 3; i++ {
		store.SaveEpisode(Episode{
			Symbol: "DOGE/USDT", Side: "BUY",
			PnL: 10, Outcome: "win", Strategy: "yolo",
		})
	}

	rules, err := c.Run()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 rules when below episode threshold, got %d", len(rules))
	}
}

func TestConsolidator_MixedOutcomesNoRule(t *testing.T) {
	c, store := newTestConsolidator(t)
	c.WithMinEpisodes(3).WithMinWinRate(0.7)

	// 50/50 win rate - should not generate rules at 0.7 threshold
	for i := 0; i < 5; i++ {
		outcome := "win"
		pnl := 100.0
		if i%2 == 0 {
			outcome = "loss"
			pnl = -100
		}
		store.SaveEpisode(Episode{
			Symbol: "ADA/USDT", Side: "BUY",
			PnL: pnl, Outcome: outcome, Strategy: "mixed",
		})
	}

	rules, err := c.Run()
	if err != nil {
		t.Fatal(err)
	}

	for _, r := range rules {
		if r.Category == "symbol-side" || r.Category == "strategy" {
			t.Errorf("should not generate rule for 50/50 outcomes, got: %s -> %s", r.Condition, r.Action)
		}
	}
}

func TestConsolidator_RulesPersistedToStore(t *testing.T) {
	c, store := newTestConsolidator(t)
	c.WithMinEpisodes(3)

	for i := 0; i < 5; i++ {
		store.SaveEpisode(Episode{
			Symbol: "BTC/USDT", Side: "BUY",
			PnL: 500, Outcome: "win", Strategy: "breakout",
			Reasoning: "Volume breakout confirmed",
		})
	}

	candidates, err := c.Run()
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) == 0 {
		t.Fatal("expected at least one candidate rule")
	}

	// Verify all candidates were persisted
	allRules, err := store.QueryRules("", 100)
	if err != nil {
		t.Fatal(err)
	}

	if len(allRules) < len(candidates) {
		t.Errorf("expected at least %d stored rules, got %d", len(candidates), len(allRules))
	}

	// Check that stored rules have source = "consolidation"
	for _, r := range allRules {
		if r.Source != "consolidation" {
			t.Errorf("expected source 'consolidation', got %q", r.Source)
		}
		t.Logf("persisted rule: %s (category=%s, confidence=%.2f, evidence=%d)", r.Content, r.Category, r.Confidence, r.EvidenceCount)
	}
}
