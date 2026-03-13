package memory

import (
	"path/filepath"
	"testing"

	"github.com/clawtrade/clawtrade/internal/database"
)

func setupRetrieverTestDB(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	db, err := database.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NewStore(db)
}

func TestRetriever_RetrieveMatchingEpisodes(t *testing.T) {
	store := setupRetrieverTestDB(t)
	retriever := NewRetriever(store, 2000)

	// Seed data
	store.SaveEpisode(Episode{Symbol: "BTC/USDT", Side: "BUY", Reasoning: "BTC pump to 70k", Outcome: "profit 5%", Strategy: "momentum"})
	store.SaveEpisode(Episode{Symbol: "ETH/USDT", Side: "SELL", Reasoning: "ETH crash to 2k", Outcome: "avoided loss", Strategy: "risk-off"})
	store.SaveEpisode(Episode{Symbol: "SOL/USDT", Side: "HOLD", Reasoning: "SOL sideways", Outcome: "no change", Strategy: "wait"})

	ctx, err := retriever.Retrieve("BTC price movement")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ctx.Episodes) == 0 {
		t.Error("expected at least one matching episode for BTC query")
	}

	found := false
	for _, ep := range ctx.Episodes {
		if ep.Symbol == "BTC/USDT" {
			found = true
		}
	}
	if !found {
		t.Error("expected BTC episode in results")
	}
}

func TestRetriever_BudgetLimit(t *testing.T) {
	store := setupRetrieverTestDB(t)
	retriever := NewRetriever(store, 50) // very small budget

	// Seed lots of data
	for i := 0; i < 20; i++ {
		store.SaveEpisode(Episode{
			Symbol:    "BTC/USDT",
			Side:      "BUY",
			Reasoning: "trading episode with lots of detail about market conditions and price action",
			Outcome:   "result observed after careful analysis of the trade performance metrics",
			Strategy:  "momentum-scalp",
		})
	}

	ctx, err := retriever.Retrieve("trading")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ctx.Episodes) >= 20 {
		t.Errorf("expected budget to limit episodes, got %d", len(ctx.Episodes))
	}

	if ctx.TokenEstimate > 50 {
		t.Errorf("token estimate %d exceeds budget 50", ctx.TokenEstimate)
	}
}

func TestRetriever_IncludesProfile(t *testing.T) {
	store := setupRetrieverTestDB(t)
	retriever := NewRetriever(store, 2000)

	store.SetProfile("risk_tolerance", "aggressive")
	store.SetProfile("trading_style", "scalping")
	store.SetProfile("preferred_pairs", "BTC,ETH")

	ctx, err := retriever.Retrieve("what is my trading style")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ctx.Profile == nil || len(ctx.Profile) == 0 {
		t.Error("expected profile in context")
	}
	if ctx.Profile["trading_style"] != "scalping" {
		t.Errorf("expected trading_style=scalping, got %s", ctx.Profile["trading_style"])
	}
}

func TestRetriever_MatchesRules(t *testing.T) {
	store := setupRetrieverTestDB(t)
	retriever := NewRetriever(store, 2000)

	store.SaveRule(Rule{Content: "BTC dumps 70% of the time before FOMC", Category: "macro", Confidence: 0.8})
	store.SaveRule(Rule{Content: "Never add to a losing position", Category: "risk", Confidence: 0.9})

	ctx, err := retriever.Retrieve("FOMC macro events")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ctx.Rules) == 0 {
		t.Error("expected at least one matching rule")
	}

	found := false
	for _, r := range ctx.Rules {
		if r.Category == "macro" {
			found = true
		}
	}
	if !found {
		t.Error("expected macro rule in results")
	}
}

func TestRetriever_MatchesConversations(t *testing.T) {
	store := setupRetrieverTestDB(t)
	retriever := NewRetriever(store, 2000)

	store.SaveMessage("user", "Should I long BTC here?")
	store.SaveMessage("assistant", "RSI is oversold, consider a small position")
	store.SaveMessage("user", "What about ETH staking yields?")

	ctx, err := retriever.Retrieve("RSI oversold signal")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ctx.Conversations) == 0 {
		t.Error("expected at least one matching conversation")
	}
}

func TestRetriever_DefaultMaxTokens(t *testing.T) {
	store := setupRetrieverTestDB(t)
	retriever := NewRetriever(store, 0)

	if retriever.maxTokens != 2000 {
		t.Errorf("expected default maxTokens=2000, got %d", retriever.maxTokens)
	}
}

func TestRetriever_EmptyQuery(t *testing.T) {
	store := setupRetrieverTestDB(t)
	retriever := NewRetriever(store, 2000)

	store.SaveEpisode(Episode{Symbol: "BTC/USDT", Side: "BUY", Reasoning: "test", Outcome: "win"})
	store.SaveRule(Rule{Content: "test rule", Category: "test", Confidence: 0.5})

	// Empty query should match everything (no keyword filter)
	ctx, err := retriever.Retrieve("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ctx.Episodes) == 0 {
		t.Error("expected episodes with empty query (match all)")
	}
	if len(ctx.Rules) == 0 {
		t.Error("expected rules with empty query (match all)")
	}
}

func TestExtractKeywords(t *testing.T) {
	keywords := extractKeywords("Should I buy BTC now?")
	// "I" is too short, should be filtered
	for _, kw := range keywords {
		if len(kw) < 3 {
			t.Errorf("keyword %q should have been filtered (len < 3)", kw)
		}
	}
	if len(keywords) == 0 {
		t.Error("expected at least one keyword")
	}
}

func TestMatchesKeywords(t *testing.T) {
	if !matchesKeywords("Bitcoin price is rising", []string{"bitcoin"}) {
		t.Error("expected match for 'bitcoin'")
	}
	if matchesKeywords("Ethereum is stable", []string{"bitcoin"}) {
		t.Error("unexpected match for 'bitcoin' in ETH text")
	}
	if !matchesKeywords("anything", nil) {
		t.Error("nil keywords should match everything")
	}
	if !matchesKeywords("anything", []string{}) {
		t.Error("empty keywords should match everything")
	}
}

func TestEstimateTokens(t *testing.T) {
	tokens := estimateTokens("abcdefgh") // 8 chars -> 2 tokens
	if tokens != 2 {
		t.Errorf("expected 2 tokens for 8 chars, got %d", tokens)
	}
	tokens = estimateTokens("ab") // 2 chars -> 1 token (minimum)
	if tokens != 1 {
		t.Errorf("expected 1 token for short text, got %d", tokens)
	}
	tokens = estimateTokens("")
	if tokens != 0 {
		t.Errorf("expected 0 tokens for empty text, got %d", tokens)
	}
}
