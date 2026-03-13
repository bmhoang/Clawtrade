package memory

import (
	"path/filepath"
	"testing"

	"github.com/clawtrade/clawtrade/internal/database"
)

func TestMemoryStore_SaveAndQueryEpisodes(t *testing.T) {
	dir := t.TempDir()
	db, _ := database.Open(filepath.Join(dir, "test.db"))
	defer db.Close()

	store := NewStore(db)

	err := store.SaveEpisode(Episode{
		Symbol: "BTC/USDT", Side: "BUY", EntryPrice: 67000, ExitPrice: 68000,
		Size: 0.1, PnL: 100, Exchange: "binance", Strategy: "rsi-scalp",
		Reasoning: "RSI oversold at 28", Outcome: "win",
	})
	if err != nil {
		t.Fatal(err)
	}

	episodes, err := store.QueryEpisodes("BTC/USDT", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(episodes) != 1 {
		t.Fatalf("expected 1 episode, got %d", len(episodes))
	}
	if episodes[0].PnL != 100 {
		t.Errorf("expected PnL 100, got %f", episodes[0].PnL)
	}
}

func TestMemoryStore_SemanticRules(t *testing.T) {
	dir := t.TempDir()
	db, _ := database.Open(filepath.Join(dir, "test.db"))
	defer db.Close()

	store := NewStore(db)

	id, err := store.SaveRule(Rule{
		Content: "BTC dumps 70% of the time before FOMC",
		Category: "macro", Confidence: 0.8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Error("expected non-zero id")
	}

	rules, err := store.QueryRules("macro", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].Confidence != 0.8 {
		t.Errorf("expected confidence 0.8, got %f", rules[0].Confidence)
	}
}

func TestMemoryStore_UserProfile(t *testing.T) {
	dir := t.TempDir()
	db, _ := database.Open(filepath.Join(dir, "test.db"))
	defer db.Close()

	store := NewStore(db)

	store.SetProfile("risk_tolerance", "2%")
	store.SetProfile("preferred_pairs", "BTC,ETH,SOL")

	val, _ := store.GetProfile("risk_tolerance")
	if val != "2%" {
		t.Errorf("expected 2%%, got %s", val)
	}

	all, _ := store.GetAllProfile()
	if len(all) != 2 {
		t.Errorf("expected 2 profile entries, got %d", len(all))
	}
}

func TestMemoryStore_Conversations(t *testing.T) {
	dir := t.TempDir()
	db, _ := database.Open(filepath.Join(dir, "test.db"))
	defer db.Close()

	store := NewStore(db)

	store.SaveMessage("user", "Should I long BTC?")
	store.SaveMessage("assistant", "Based on RSI analysis...")

	msgs, _ := store.GetRecentMessages(10)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("expected first message from user, got %s", msgs[0].Role)
	}
}
