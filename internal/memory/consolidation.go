package memory

import (
	"fmt"
	"math"
	"strings"
)

// CandidateRule represents a rule extracted from episodic memory patterns.
type CandidateRule struct {
	Condition    string  // e.g., "RSI oversold on BTC/USDT"
	Action       string  // e.g., "BUY tends to win"
	Confidence   float64 // 0.0 to 1.0
	SourceCount  int     // number of episodes that contributed
	Category     string  // rule category for storage
	WinRate      float64 // observed win rate for this pattern
	AvgPnL       float64 // average PnL across matching episodes
}

// Consolidator scans episodic memories and extracts semantic rules.
type Consolidator struct {
	store        *Store
	minEpisodes  int     // minimum episodes to form a rule
	minWinRate   float64 // minimum win rate to generate a rule
}

// NewConsolidator creates a Consolidator with sensible defaults.
func NewConsolidator(store *Store) *Consolidator {
	return &Consolidator{
		store:       store,
		minEpisodes: 3,
		minWinRate:  0.6,
	}
}

// WithMinEpisodes sets the minimum number of episodes required to form a rule.
func (c *Consolidator) WithMinEpisodes(n int) *Consolidator {
	c.minEpisodes = n
	return c
}

// WithMinWinRate sets the minimum win rate threshold for rule generation.
func (c *Consolidator) WithMinWinRate(rate float64) *Consolidator {
	c.minWinRate = rate
	return c
}

// episodeGroup is an internal grouping of episodes by a key.
type episodeGroup struct {
	key      string
	episodes []Episode
}

// Run scans recent episodes, groups them by patterns, extracts candidate rules,
// and stores qualifying rules back into the memory store.
func (c *Consolidator) Run() ([]CandidateRule, error) {
	episodes, err := c.store.QueryEpisodes("", 500)
	if err != nil {
		return nil, fmt.Errorf("consolidation: query episodes: %w", err)
	}
	if len(episodes) == 0 {
		return nil, nil
	}

	var candidates []CandidateRule

	// Strategy 1: group by symbol + side
	candidates = append(candidates, c.extractBySymbolSide(episodes)...)

	// Strategy 2: group by strategy name
	candidates = append(candidates, c.extractByStrategy(episodes)...)

	// Strategy 3: group by reasoning keywords
	candidates = append(candidates, c.extractByReasoningKeyword(episodes)...)

	// Strategy 4: group by symbol + outcome pattern (consecutive losses/wins)
	candidates = append(candidates, c.extractByOutcomeStreak(episodes)...)

	// Deduplicate candidates with similar content
	candidates = dedup(candidates)

	// Persist qualifying rules
	for _, cr := range candidates {
		rule := Rule{
			Content:       fmt.Sprintf("%s -> %s", cr.Condition, cr.Action),
			Category:      cr.Category,
			Confidence:    cr.Confidence,
			EvidenceCount: cr.SourceCount,
			Effectiveness: cr.AvgPnL,
			Source:        "consolidation",
		}
		if _, err := c.store.SaveRule(rule); err != nil {
			return candidates, fmt.Errorf("consolidation: save rule: %w", err)
		}
	}

	return candidates, nil
}

// extractBySymbolSide groups episodes by symbol+side and looks for win-rate patterns.
func (c *Consolidator) extractBySymbolSide(episodes []Episode) []CandidateRule {
	groups := make(map[string][]Episode)
	for _, ep := range episodes {
		key := ep.Symbol + "|" + ep.Side
		groups[key] = append(groups[key], ep)
	}

	var rules []CandidateRule
	for key, eps := range groups {
		if len(eps) < c.minEpisodes {
			continue
		}
		parts := strings.SplitN(key, "|", 2)
		symbol, side := parts[0], parts[1]

		wins, totalPnL := countWinsAndPnL(eps)
		winRate := float64(wins) / float64(len(eps))
		avgPnL := totalPnL / float64(len(eps))

		if winRate >= c.minWinRate {
			rules = append(rules, CandidateRule{
				Condition:   fmt.Sprintf("%s on %s", side, symbol),
				Action:      fmt.Sprintf("%s tends to win (%.0f%% win rate)", side, winRate*100),
				Confidence:  winRate,
				SourceCount: len(eps),
				Category:    "symbol-side",
				WinRate:     winRate,
				AvgPnL:      avgPnL,
			})
		} else if winRate <= (1 - c.minWinRate) {
			rules = append(rules, CandidateRule{
				Condition:   fmt.Sprintf("%s on %s", side, symbol),
				Action:      fmt.Sprintf("%s tends to lose (%.0f%% loss rate)", side, (1-winRate)*100),
				Confidence:  1 - winRate,
				SourceCount: len(eps),
				Category:    "symbol-side",
				WinRate:     winRate,
				AvgPnL:      avgPnL,
			})
		}
	}
	return rules
}

// extractByStrategy groups episodes by strategy and finds which strategies perform well.
func (c *Consolidator) extractByStrategy(episodes []Episode) []CandidateRule {
	groups := make(map[string][]Episode)
	for _, ep := range episodes {
		if ep.Strategy == "" {
			continue
		}
		groups[ep.Strategy] = append(groups[ep.Strategy], ep)
	}

	var rules []CandidateRule
	for strategy, eps := range groups {
		if len(eps) < c.minEpisodes {
			continue
		}
		wins, totalPnL := countWinsAndPnL(eps)
		winRate := float64(wins) / float64(len(eps))
		avgPnL := totalPnL / float64(len(eps))

		if winRate >= c.minWinRate {
			rules = append(rules, CandidateRule{
				Condition:   fmt.Sprintf("strategy %s", strategy),
				Action:      fmt.Sprintf("strategy %s is profitable (%.0f%% win rate)", strategy, winRate*100),
				Confidence:  winRate,
				SourceCount: len(eps),
				Category:    "strategy",
				WinRate:     winRate,
				AvgPnL:      avgPnL,
			})
		} else if winRate <= (1 - c.minWinRate) {
			rules = append(rules, CandidateRule{
				Condition:   fmt.Sprintf("strategy %s", strategy),
				Action:      fmt.Sprintf("strategy %s is unprofitable (%.0f%% loss rate)", strategy, (1-winRate)*100),
				Confidence:  1 - winRate,
				SourceCount: len(eps),
				Category:    "strategy",
				WinRate:     winRate,
				AvgPnL:      avgPnL,
			})
		}
	}
	return rules
}

// extractByReasoningKeyword scans reasoning text for common indicator keywords
// and groups episodes by those keywords to find patterns.
func (c *Consolidator) extractByReasoningKeyword(episodes []Episode) []CandidateRule {
	keywords := []string{"RSI", "MACD", "EMA", "SMA", "bollinger", "volume", "breakout", "support", "resistance", "oversold", "overbought"}

	groups := make(map[string][]Episode)
	for _, ep := range episodes {
		if ep.Reasoning == "" {
			continue
		}
		lower := strings.ToLower(ep.Reasoning)
		for _, kw := range keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				key := strings.ToLower(kw) + "|" + ep.Outcome
				groups[key] = append(groups[key], ep)
			}
		}
	}

	// Now group by keyword only (across outcomes) to compute win rates
	kwGroups := make(map[string][]Episode)
	for _, ep := range episodes {
		if ep.Reasoning == "" {
			continue
		}
		lower := strings.ToLower(ep.Reasoning)
		for _, kw := range keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				kwGroups[strings.ToLower(kw)] = append(kwGroups[strings.ToLower(kw)], ep)
			}
		}
	}

	var rules []CandidateRule
	for kw, eps := range kwGroups {
		if len(eps) < c.minEpisodes {
			continue
		}
		wins, totalPnL := countWinsAndPnL(eps)
		winRate := float64(wins) / float64(len(eps))
		avgPnL := totalPnL / float64(len(eps))

		if winRate >= c.minWinRate {
			rules = append(rules, CandidateRule{
				Condition:   fmt.Sprintf("reasoning mentions %s", kw),
				Action:      fmt.Sprintf("trades based on %s tend to win (%.0f%%)", kw, winRate*100),
				Confidence:  winRate,
				SourceCount: len(eps),
				Category:    "indicator",
				WinRate:     winRate,
				AvgPnL:      avgPnL,
			})
		} else if winRate <= (1 - c.minWinRate) {
			rules = append(rules, CandidateRule{
				Condition:   fmt.Sprintf("reasoning mentions %s", kw),
				Action:      fmt.Sprintf("trades based on %s tend to lose (%.0f%%)", kw, (1-winRate)*100),
				Confidence:  1 - winRate,
				SourceCount: len(eps),
				Category:    "indicator",
				WinRate:     winRate,
				AvgPnL:      avgPnL,
			})
		}
	}
	return rules
}

// extractByOutcomeStreak detects symbols where recent trades show a consistent
// winning or losing pattern, suggesting momentum or mean-reversion signals.
func (c *Consolidator) extractByOutcomeStreak(episodes []Episode) []CandidateRule {
	// Group by symbol, episodes are already ordered by opened_at DESC
	groups := make(map[string][]Episode)
	for _, ep := range episodes {
		groups[ep.Symbol] = append(groups[ep.Symbol], ep)
	}

	var rules []CandidateRule
	for symbol, eps := range groups {
		if len(eps) < c.minEpisodes {
			continue
		}
		// Check for consecutive outcome streaks (from most recent)
		streak := 1
		firstOutcome := eps[0].Outcome
		for i := 1; i < len(eps); i++ {
			if eps[i].Outcome == firstOutcome {
				streak++
			} else {
				break
			}
		}
		if streak >= c.minEpisodes {
			confidence := math.Min(float64(streak)/float64(streak+2), 0.95) // bounded confidence
			if firstOutcome == "win" {
				rules = append(rules, CandidateRule{
					Condition:   fmt.Sprintf("%s has %d consecutive wins", symbol, streak),
					Action:      fmt.Sprintf("%s is on a winning streak", symbol),
					Confidence:  confidence,
					SourceCount: streak,
					Category:    "streak",
					WinRate:     1.0,
				})
			} else if firstOutcome == "loss" {
				rules = append(rules, CandidateRule{
					Condition:   fmt.Sprintf("%s has %d consecutive losses", symbol, streak),
					Action:      fmt.Sprintf("%s is on a losing streak, consider pausing", symbol),
					Confidence:  confidence,
					SourceCount: streak,
					Category:    "streak",
					WinRate:     0.0,
				})
			}
		}
	}
	return rules
}

// countWinsAndPnL counts wins and total PnL for a slice of episodes.
func countWinsAndPnL(eps []Episode) (wins int, totalPnL float64) {
	for _, ep := range eps {
		if ep.Outcome == "win" {
			wins++
		}
		totalPnL += ep.PnL
	}
	return
}

// dedup removes candidate rules that have identical Condition+Action.
func dedup(candidates []CandidateRule) []CandidateRule {
	seen := make(map[string]bool)
	var result []CandidateRule
	for _, cr := range candidates {
		key := cr.Condition + "|" + cr.Action
		if !seen[key] {
			seen[key] = true
			result = append(result, cr)
		}
	}
	return result
}
