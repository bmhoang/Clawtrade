package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Node represents an entity in the knowledge graph.
// Entities are identified by a type:name key (e.g. "symbol:BTC").
type Node struct {
	ID         int64             `json:"id"`
	Type       string            `json:"type"`       // "symbol", "indicator", "pattern", "event", "strategy"
	Name       string            `json:"name"`       // unique within type
	Properties map[string]string `json:"properties,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
}

// Edge represents a relation between two entities.
type Edge struct {
	ID        int64     `json:"id"`
	FromID    int64     `json:"from_id"`
	ToID      int64     `json:"to_id"`
	Relation  string    `json:"relation"` // "correlates_with", "signals", "part_of", "causes"
	Weight    float64   `json:"weight"`   // strength of relation 0-1
	Evidence  string    `json:"evidence"`
	CreatedAt time.Time `json:"created_at"`
}

// KnowledgeGraph provides entity-relation storage and traversal
// using the existing knowledge_graph table plus a kg_nodes table for
// entity metadata.
type KnowledgeGraph struct {
	db *sql.DB
}

// NewKnowledgeGraph creates a KnowledgeGraph backed by the given database.
// It creates the kg_nodes table for entity metadata (the knowledge_graph
// table for edges is already created by database migrations).
func NewKnowledgeGraph(db *sql.DB) *KnowledgeGraph {
	db.Exec(`CREATE TABLE IF NOT EXISTS kg_nodes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		type TEXT NOT NULL,
		name TEXT NOT NULL,
		properties TEXT DEFAULT '{}',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(type, name)
	)`)

	db.Exec(`CREATE INDEX IF NOT EXISTS idx_kg_nodes_type ON kg_nodes(type)`)

	return &KnowledgeGraph{db: db}
}

// entityKey returns the composite key used in the knowledge_graph table.
func entityKey(nodeType, name string) string {
	return nodeType + ":" + name
}

// AddNode adds or updates an entity node. Returns the node ID.
func (g *KnowledgeGraph) AddNode(nodeType, name string, properties map[string]string) (int64, error) {
	propsJSON := "{}"
	if len(properties) > 0 {
		b, err := json.Marshal(properties)
		if err != nil {
			return 0, fmt.Errorf("marshal properties: %w", err)
		}
		propsJSON = string(b)
	}

	result, err := g.db.Exec(
		`INSERT INTO kg_nodes (type, name, properties) VALUES (?, ?, ?)
		 ON CONFLICT(type, name) DO UPDATE SET properties = excluded.properties`,
		nodeType, name, propsJSON,
	)
	if err != nil {
		return 0, fmt.Errorf("insert node: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil || id == 0 {
		// On upsert, LastInsertId may be 0; fetch the existing ID.
		row := g.db.QueryRow(`SELECT id FROM kg_nodes WHERE type = ? AND name = ?`, nodeType, name)
		if err := row.Scan(&id); err != nil {
			return 0, fmt.Errorf("fetch node id: %w", err)
		}
	}
	return id, nil
}

// AddEdge creates a relation between two nodes. It writes to both
// the kg_nodes-based lookup and the existing knowledge_graph table.
func (g *KnowledgeGraph) AddEdge(fromID, toID int64, relation string, weight float64) error {
	// Look up entity keys for the knowledge_graph table.
	fromKey, err := g.nodeKey(fromID)
	if err != nil {
		return fmt.Errorf("from node: %w", err)
	}
	toKey, err := g.nodeKey(toID)
	if err != nil {
		return fmt.Errorf("to node: %w", err)
	}

	// Write to the existing knowledge_graph table.
	_, err = g.db.Exec(
		`INSERT INTO knowledge_graph (entity_from, relation, entity_to, weight)
		 VALUES (?, ?, ?, ?)`,
		fromKey, relation, toKey, weight,
	)
	if err != nil {
		return fmt.Errorf("insert edge: %w", err)
	}
	return nil
}

// nodeKey returns the "type:name" key for a node ID.
func (g *KnowledgeGraph) nodeKey(id int64) (string, error) {
	var t, n string
	err := g.db.QueryRow(`SELECT type, name FROM kg_nodes WHERE id = ?`, id).Scan(&t, &n)
	if err != nil {
		return "", err
	}
	return entityKey(t, n), nil
}

// GetNode retrieves a node by type and name.
func (g *KnowledgeGraph) GetNode(nodeType, name string) (*Node, error) {
	row := g.db.QueryRow(
		`SELECT id, type, name, properties, created_at FROM kg_nodes WHERE type = ? AND name = ?`,
		nodeType, name,
	)
	var node Node
	var props string
	err := row.Scan(&node.ID, &node.Type, &node.Name, &props, &node.CreatedAt)
	if err != nil {
		return nil, err
	}
	if props != "" && props != "{}" {
		_ = json.Unmarshal([]byte(props), &node.Properties)
	}
	return &node, nil
}

// GetRelated returns nodes connected from the given node (outgoing edges).
// If relation is non-empty, only edges with that relation type are returned.
func (g *KnowledgeGraph) GetRelated(nodeID int64, relation string) ([]Node, error) {
	fromKey, err := g.nodeKey(nodeID)
	if err != nil {
		return nil, fmt.Errorf("node key: %w", err)
	}

	query := `SELECT kg.entity_to, kg.weight FROM knowledge_graph kg WHERE kg.entity_from = ?`
	args := []any{fromKey}

	if relation != "" {
		query += ` AND kg.relation = ?`
		args = append(args, relation)
	}
	query += ` ORDER BY kg.weight DESC`

	rows, err := g.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		var toKey string
		var w float64
		if err := rows.Scan(&toKey, &w); err != nil {
			continue
		}
		// Parse "type:name" and look up the node.
		node, err := g.nodeFromKey(toKey)
		if err != nil {
			continue
		}
		nodes = append(nodes, *node)
	}
	return nodes, nil
}

// nodeFromKey parses a "type:name" key and fetches the corresponding node.
func (g *KnowledgeGraph) nodeFromKey(key string) (*Node, error) {
	// Split on first colon.
	for i := 0; i < len(key); i++ {
		if key[i] == ':' {
			return g.GetNode(key[:i], key[i+1:])
		}
	}
	return nil, fmt.Errorf("invalid entity key: %s", key)
}

// GetNodesByType returns all nodes of a given type.
func (g *KnowledgeGraph) GetNodesByType(nodeType string) ([]Node, error) {
	rows, err := g.db.Query(
		`SELECT id, type, name, properties, created_at FROM kg_nodes WHERE type = ? ORDER BY name`,
		nodeType,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		var n Node
		var props string
		if err := rows.Scan(&n.ID, &n.Type, &n.Name, &props, &n.CreatedAt); err != nil {
			continue
		}
		if props != "" && props != "{}" {
			_ = json.Unmarshal([]byte(props), &n.Properties)
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

// Traverse performs a BFS traversal up to maxDepth from a starting node,
// returning all discovered nodes (excluding the start node itself).
func (g *KnowledgeGraph) Traverse(startID int64, maxDepth int) ([]Node, error) {
	visited := map[int64]bool{startID: true}
	var result []Node

	queue := []int64{startID}

	for depth := 0; depth < maxDepth && len(queue) > 0; depth++ {
		var nextQueue []int64
		for _, nodeID := range queue {
			related, err := g.GetRelated(nodeID, "")
			if err != nil {
				continue
			}
			for _, n := range related {
				if !visited[n.ID] {
					visited[n.ID] = true
					result = append(result, n)
					nextQueue = append(nextQueue, n.ID)
				}
			}
		}
		queue = nextQueue
	}

	return result, nil
}
