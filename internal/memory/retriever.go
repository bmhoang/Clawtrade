package memory

import (
	"fmt"
	"strings"
)

// MemoryContext holds retrieved memories relevant to a query.
type MemoryContext struct {
	Episodes      []Episode         `json:"episodes"`
	Rules         []Rule            `json:"rules"`
	Profile       map[string]string `json:"profile,omitempty"`
	Conversations []Message         `json:"conversations"`
	TokenEstimate int               `json:"token_estimate"`
}

// Retriever searches memory layers for relevant context.
type Retriever struct {
	store     *Store
	maxTokens int
}

// NewRetriever creates a Retriever with a token budget. If maxTokens <= 0, defaults to 2000.
func NewRetriever(store *Store, maxTokens int) *Retriever {
	if maxTokens <= 0 {
		maxTokens = 2000
	}
	return &Retriever{store: store, maxTokens: maxTokens}
}

// Retrieve searches all memory layers for content matching the query keywords.
// It fills results in priority order (profile, rules, episodes, conversations)
// until the token budget is exhausted.
func (r *Retriever) Retrieve(query string) (*MemoryContext, error) {
	ctx := &MemoryContext{}
	keywords := extractKeywords(query)
	tokenBudget := r.maxTokens

	// Layer 1: User profile (always include if available)
	profile, err := r.store.GetAllProfile()
	if err == nil && len(profile) > 0 {
		cost := estimateTokens(formatProfile(profile))
		if cost <= tokenBudget {
			ctx.Profile = profile
			tokenBudget -= cost
		}
	}

	// Layer 2: Semantic rules (highest priority trading knowledge)
	rules, err := r.store.QueryRules("", 50)
	if err == nil {
		for _, rule := range rules {
			if matchesKeywords(rule.Content+" "+rule.Category, keywords) {
				cost := estimateTokens(rule.Content)
				if cost > tokenBudget {
					break
				}
				ctx.Rules = append(ctx.Rules, rule)
				tokenBudget -= cost
			}
		}
	}

	// Layer 3: Episodes (recent trading experiences)
	episodes, err := r.store.QueryEpisodes("", 50)
	if err == nil {
		for _, ep := range episodes {
			text := ep.Symbol + " " + ep.Side + " " + ep.Strategy + " " + ep.Reasoning + " " + ep.Outcome + " " + ep.PostMortem
			if matchesKeywords(text, keywords) {
				cost := estimateTokens(text)
				if cost > tokenBudget {
					break
				}
				ctx.Episodes = append(ctx.Episodes, ep)
				tokenBudget -= cost
			}
		}
	}

	// Layer 4: Recent conversations
	conversations, err := r.store.GetRecentMessages(20)
	if err == nil {
		for _, msg := range conversations {
			if matchesKeywords(msg.Content, keywords) {
				cost := estimateTokens(msg.Content)
				if cost > tokenBudget {
					break
				}
				ctx.Conversations = append(ctx.Conversations, msg)
				tokenBudget -= cost
			}
		}
	}

	ctx.TokenEstimate = r.maxTokens - tokenBudget
	return ctx, nil
}

// extractKeywords splits query into lowercase keywords, filtering words shorter than 3 chars.
func extractKeywords(query string) []string {
	words := strings.Fields(strings.ToLower(query))
	var keywords []string
	for _, w := range words {
		if len(w) >= 3 {
			keywords = append(keywords, w)
		}
	}
	return keywords
}

// matchesKeywords checks if text contains any of the keywords.
func matchesKeywords(text string, keywords []string) bool {
	if len(keywords) == 0 {
		return true
	}
	lower := strings.ToLower(text)
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// estimateTokens roughly estimates token count (1 token ~ 4 chars).
func estimateTokens(text string) int {
	n := len(text) / 4
	if n == 0 && len(text) > 0 {
		n = 1
	}
	return n
}

// formatProfile converts profile map to a string for token estimation.
func formatProfile(profile map[string]string) string {
	var parts []string
	for k, v := range profile {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(parts, " ")
}
