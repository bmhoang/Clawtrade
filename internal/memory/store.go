package memory

import (
	"database/sql"
	"time"
)

type Episode struct {
	ID         int64     `json:"id"`
	Symbol     string    `json:"symbol"`
	Side       string    `json:"side"`
	EntryPrice float64   `json:"entry_price"`
	ExitPrice  float64   `json:"exit_price"`
	Size       float64   `json:"size"`
	PnL        float64   `json:"pnl"`
	Exchange   string    `json:"exchange"`
	Strategy   string    `json:"strategy"`
	Reasoning  string    `json:"reasoning"`
	Outcome    string    `json:"outcome"`
	EmotionTag string    `json:"emotion_tag"`
	Confidence float64   `json:"confidence"`
	PostMortem string    `json:"post_mortem"`
	OpenedAt   time.Time `json:"opened_at"`
	ClosedAt   time.Time `json:"closed_at"`
}

type Rule struct {
	ID            int64   `json:"id"`
	Content       string  `json:"content"`
	Category      string  `json:"category"`
	Confidence    float64 `json:"confidence"`
	EvidenceCount int     `json:"evidence_count"`
	Effectiveness float64 `json:"effectiveness"`
	Source        string  `json:"source"`
}

type Message struct {
	ID        int64     `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) SaveEpisode(ep Episode) error {
	if ep.OpenedAt.IsZero() {
		ep.OpenedAt = time.Now()
	}
	_, err := s.db.Exec(
		`INSERT INTO trade_episodes (symbol, side, entry_price, exit_price, size, pnl, exchange, strategy, reasoning, outcome, emotion_tag, confidence, post_mortem, opened_at, closed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ep.Symbol, ep.Side, ep.EntryPrice, ep.ExitPrice, ep.Size, ep.PnL,
		ep.Exchange, ep.Strategy, ep.Reasoning, ep.Outcome, ep.EmotionTag,
		ep.Confidence, ep.PostMortem, ep.OpenedAt, ep.ClosedAt,
	)
	return err
}

func (s *Store) QueryEpisodes(symbol string, limit int) ([]Episode, error) {
	query := "SELECT id, symbol, side, entry_price, exit_price, size, pnl, exchange, strategy, reasoning, outcome, emotion_tag, confidence, post_mortem, opened_at, closed_at FROM trade_episodes"
	var args []any
	if symbol != "" {
		query += " WHERE symbol = ?"
		args = append(args, symbol)
	}
	query += " ORDER BY opened_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var episodes []Episode
	for rows.Next() {
		var ep Episode
		err := rows.Scan(&ep.ID, &ep.Symbol, &ep.Side, &ep.EntryPrice, &ep.ExitPrice,
			&ep.Size, &ep.PnL, &ep.Exchange, &ep.Strategy, &ep.Reasoning,
			&ep.Outcome, &ep.EmotionTag, &ep.Confidence, &ep.PostMortem,
			&ep.OpenedAt, &ep.ClosedAt)
		if err != nil {
			return nil, err
		}
		episodes = append(episodes, ep)
	}
	return episodes, nil
}

func (s *Store) SaveRule(rule Rule) (int64, error) {
	result, err := s.db.Exec(
		`INSERT INTO semantic_rules (content, category, confidence, evidence_count, effectiveness, source)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		rule.Content, rule.Category, rule.Confidence, rule.EvidenceCount, rule.Effectiveness, rule.Source,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) QueryRules(category string, limit int) ([]Rule, error) {
	query := "SELECT id, content, category, confidence, evidence_count, effectiveness, source FROM semantic_rules"
	var args []any
	if category != "" {
		query += " WHERE category = ?"
		args = append(args, category)
	}
	query += " ORDER BY confidence DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []Rule
	for rows.Next() {
		var r Rule
		err := rows.Scan(&r.ID, &r.Content, &r.Category, &r.Confidence,
			&r.EvidenceCount, &r.Effectiveness, &r.Source)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, nil
}

func (s *Store) UpdateRuleConfidence(id int64, confidence float64, evidenceCount int) error {
	_, err := s.db.Exec(
		"UPDATE semantic_rules SET confidence = ?, evidence_count = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		confidence, evidenceCount, id,
	)
	return err
}

func (s *Store) SetProfile(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO user_profile (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`,
		key, value,
	)
	return err
}

func (s *Store) GetProfile(key string) (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM user_profile WHERE key = ?", key).Scan(&value)
	return value, err
}

func (s *Store) GetAllProfile() (map[string]string, error) {
	rows, err := s.db.Query("SELECT key, value FROM user_profile")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	profile := make(map[string]string)
	for rows.Next() {
		var k, v string
		rows.Scan(&k, &v)
		profile[k] = v
	}
	return profile, nil
}

func (s *Store) SaveMessage(role, content string) error {
	_, err := s.db.Exec("INSERT INTO conversations (role, content) VALUES (?, ?)", role, content)
	return err
}

func (s *Store) GetRecentMessages(limit int) ([]Message, error) {
	rows, err := s.db.Query(
		"SELECT id, role, content, created_at FROM conversations ORDER BY id ASC LIMIT ?", limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []Message
	for rows.Next() {
		var m Message
		rows.Scan(&m.ID, &m.Role, &m.Content, &m.CreatedAt)
		messages = append(messages, m)
	}
	return messages, nil
}
