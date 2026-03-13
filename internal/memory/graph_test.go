package memory

import (
	"path/filepath"
	"testing"

	"github.com/clawtrade/clawtrade/internal/database"
)

func setupGraphTestDB(t *testing.T) *KnowledgeGraph {
	t.Helper()
	dir := t.TempDir()
	db, err := database.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NewKnowledgeGraph(db)
}

func TestKnowledgeGraph_AddAndGetNode(t *testing.T) {
	g := setupGraphTestDB(t)

	id, err := g.AddNode("symbol", "BTC", map[string]string{"exchange": "binance"})
	if err != nil {
		t.Fatal(err)
	}
	if id <= 0 {
		t.Error("expected positive ID")
	}

	node, err := g.GetNode("symbol", "BTC")
	if err != nil {
		t.Fatal(err)
	}
	if node.Name != "BTC" {
		t.Errorf("expected BTC, got %s", node.Name)
	}
	if node.Properties["exchange"] != "binance" {
		t.Errorf("expected exchange=binance, got %v", node.Properties)
	}
}

func TestKnowledgeGraph_UpsertNode(t *testing.T) {
	g := setupGraphTestDB(t)

	id1, err := g.AddNode("symbol", "BTC", map[string]string{"exchange": "binance"})
	if err != nil {
		t.Fatal(err)
	}

	id2, err := g.AddNode("symbol", "BTC", map[string]string{"exchange": "coinbase"})
	if err != nil {
		t.Fatal(err)
	}
	if id2 != id1 {
		t.Errorf("upsert should return same ID: got %d and %d", id1, id2)
	}

	node, err := g.GetNode("symbol", "BTC")
	if err != nil {
		t.Fatal(err)
	}
	if node.Properties["exchange"] != "coinbase" {
		t.Errorf("expected updated exchange=coinbase, got %v", node.Properties)
	}
}

func TestKnowledgeGraph_AddEdgeAndGetRelated(t *testing.T) {
	g := setupGraphTestDB(t)

	btcID, _ := g.AddNode("symbol", "BTC", nil)
	ethID, _ := g.AddNode("symbol", "ETH", nil)
	rsiID, _ := g.AddNode("indicator", "RSI", nil)

	if err := g.AddEdge(btcID, ethID, "correlates_with", 0.85); err != nil {
		t.Fatal(err)
	}
	if err := g.AddEdge(btcID, rsiID, "signals", 0.7); err != nil {
		t.Fatal(err)
	}

	related, err := g.GetRelated(btcID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(related) != 2 {
		t.Errorf("expected 2 related nodes, got %d", len(related))
	}

	// Filter by relation
	correlated, err := g.GetRelated(btcID, "correlates_with")
	if err != nil {
		t.Fatal(err)
	}
	if len(correlated) != 1 || correlated[0].Name != "ETH" {
		t.Error("expected ETH as correlated")
	}
}

func TestKnowledgeGraph_Traverse(t *testing.T) {
	g := setupGraphTestDB(t)

	btcID, _ := g.AddNode("symbol", "BTC", nil)
	ethID, _ := g.AddNode("symbol", "ETH", nil)
	defiID, _ := g.AddNode("pattern", "DeFi", nil)

	g.AddEdge(btcID, ethID, "correlates_with", 0.8)
	g.AddEdge(ethID, defiID, "part_of", 0.9)

	nodes, err := g.Traverse(btcID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) < 2 {
		t.Errorf("expected at least 2 nodes in traversal, got %d", len(nodes))
	}

	// Verify both ETH and DeFi are found
	found := map[string]bool{}
	for _, n := range nodes {
		found[n.Name] = true
	}
	if !found["ETH"] || !found["DeFi"] {
		t.Errorf("expected ETH and DeFi in traversal, got %v", found)
	}
}

func TestKnowledgeGraph_GetNodesByType(t *testing.T) {
	g := setupGraphTestDB(t)

	g.AddNode("symbol", "BTC", nil)
	g.AddNode("symbol", "ETH", nil)
	g.AddNode("indicator", "RSI", nil)

	symbols, err := g.GetNodesByType("symbol")
	if err != nil {
		t.Fatal(err)
	}
	if len(symbols) != 2 {
		t.Errorf("expected 2 symbols, got %d", len(symbols))
	}

	indicators, err := g.GetNodesByType("indicator")
	if err != nil {
		t.Fatal(err)
	}
	if len(indicators) != 1 {
		t.Errorf("expected 1 indicator, got %d", len(indicators))
	}
}

func TestKnowledgeGraph_GetNodeNotFound(t *testing.T) {
	g := setupGraphTestDB(t)

	_, err := g.GetNode("symbol", "NONEXISTENT")
	if err == nil {
		t.Error("expected error for nonexistent node")
	}
}

func TestKnowledgeGraph_TraverseDepthOne(t *testing.T) {
	g := setupGraphTestDB(t)

	aID, _ := g.AddNode("symbol", "A", nil)
	bID, _ := g.AddNode("symbol", "B", nil)
	g.AddNode("symbol", "C", nil)

	g.AddEdge(aID, bID, "correlates_with", 0.8)
	// B -> C edge exists but depth=1 should not reach C
	cID, _ := g.AddNode("symbol", "C", nil)
	g.AddEdge(bID, cID, "correlates_with", 0.7)

	nodes, err := g.Traverse(aID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Errorf("expected 1 node at depth 1, got %d", len(nodes))
	}
	if len(nodes) > 0 && nodes[0].Name != "B" {
		t.Errorf("expected B, got %s", nodes[0].Name)
	}
}
