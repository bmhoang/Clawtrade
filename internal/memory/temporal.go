package memory

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// TradeRecord stores a trade outcome with timing info
type TradeRecord struct {
	Timestamp time.Time
	Symbol    string
	PnL       float64 // profit/loss
	Duration  time.Duration
}

// TimeSlotStats holds aggregated stats for a time slot
type TimeSlotStats struct {
	Label    string  `json:"label"`
	Trades   int     `json:"trades"`
	WinRate  float64 `json:"win_rate"`
	AvgPnL   float64 `json:"avg_pnl"`
	TotalPnL float64 `json:"total_pnl"`
}

// TemporalPattern represents a discovered pattern
type TemporalPattern struct {
	Type       string  `json:"type"` // "best_hour", "worst_hour", "best_day", "worst_day"
	Label      string  `json:"label"`
	WinRate    float64 `json:"win_rate"`
	AvgPnL     float64 `json:"avg_pnl"`
	SampleSize int     `json:"sample_size"`
	Confidence float64 `json:"confidence"` // based on sample size
}

// TemporalAnalyzer tracks and analyzes time-based trading patterns
type TemporalAnalyzer struct {
	mu      sync.RWMutex
	records []TradeRecord
}

func NewTemporalAnalyzer() *TemporalAnalyzer {
	return &TemporalAnalyzer{
		records: make([]TradeRecord, 0),
	}
}

// AddTrade records a trade outcome
func (ta *TemporalAnalyzer) AddTrade(record TradeRecord) {
	ta.mu.Lock()
	defer ta.mu.Unlock()
	ta.records = append(ta.records, record)
}

// ByHour returns performance stats grouped by hour of day (0-23)
func (ta *TemporalAnalyzer) ByHour() []TimeSlotStats {
	ta.mu.RLock()
	defer ta.mu.RUnlock()

	hourStats := make(map[int]*accumulator)
	for i := 0; i < 24; i++ {
		hourStats[i] = &accumulator{}
	}

	for _, r := range ta.records {
		hour := r.Timestamp.Hour()
		hourStats[hour].add(r.PnL)
	}

	results := make([]TimeSlotStats, 0, 24)
	for h := 0; h < 24; h++ {
		acc := hourStats[h]
		if acc.count == 0 {
			continue
		}
		results = append(results, TimeSlotStats{
			Label:    formatHour(h),
			Trades:   acc.count,
			WinRate:  float64(acc.wins) / float64(acc.count),
			AvgPnL:   acc.totalPnL / float64(acc.count),
			TotalPnL: acc.totalPnL,
		})
	}
	return results
}

// ByDayOfWeek returns performance stats grouped by day of week
func (ta *TemporalAnalyzer) ByDayOfWeek() []TimeSlotStats {
	ta.mu.RLock()
	defer ta.mu.RUnlock()

	dayStats := make(map[time.Weekday]*accumulator)
	for d := time.Sunday; d <= time.Saturday; d++ {
		dayStats[d] = &accumulator{}
	}

	for _, r := range ta.records {
		day := r.Timestamp.Weekday()
		dayStats[day].add(r.PnL)
	}

	results := make([]TimeSlotStats, 0, 7)
	for d := time.Sunday; d <= time.Saturday; d++ {
		acc := dayStats[d]
		if acc.count == 0 {
			continue
		}
		results = append(results, TimeSlotStats{
			Label:    d.String(),
			Trades:   acc.count,
			WinRate:  float64(acc.wins) / float64(acc.count),
			AvgPnL:   acc.totalPnL / float64(acc.count),
			TotalPnL: acc.totalPnL,
		})
	}
	return results
}

// FindPatterns discovers significant temporal patterns
func (ta *TemporalAnalyzer) FindPatterns(minTrades int) []TemporalPattern {
	if minTrades <= 0 {
		minTrades = 5
	}

	var patterns []TemporalPattern

	// Analyze hours
	hourStats := ta.ByHour()
	if len(hourStats) > 0 {
		sort.Slice(hourStats, func(i, j int) bool {
			return hourStats[i].AvgPnL > hourStats[j].AvgPnL
		})

		for _, s := range hourStats {
			if s.Trades >= minTrades {
				confidence := calculateConfidence(s.Trades)
				if s.WinRate > 0.6 {
					patterns = append(patterns, TemporalPattern{
						Type: "best_hour", Label: s.Label,
						WinRate: s.WinRate, AvgPnL: s.AvgPnL,
						SampleSize: s.Trades, Confidence: confidence,
					})
				}
				if s.WinRate < 0.4 {
					patterns = append(patterns, TemporalPattern{
						Type: "worst_hour", Label: s.Label,
						WinRate: s.WinRate, AvgPnL: s.AvgPnL,
						SampleSize: s.Trades, Confidence: confidence,
					})
				}
			}
		}
	}

	// Analyze days
	dayStats := ta.ByDayOfWeek()
	if len(dayStats) > 0 {
		for _, s := range dayStats {
			if s.Trades >= minTrades {
				confidence := calculateConfidence(s.Trades)
				if s.WinRate > 0.6 {
					patterns = append(patterns, TemporalPattern{
						Type: "best_day", Label: s.Label,
						WinRate: s.WinRate, AvgPnL: s.AvgPnL,
						SampleSize: s.Trades, Confidence: confidence,
					})
				}
				if s.WinRate < 0.4 {
					patterns = append(patterns, TemporalPattern{
						Type: "worst_day", Label: s.Label,
						WinRate: s.WinRate, AvgPnL: s.AvgPnL,
						SampleSize: s.Trades, Confidence: confidence,
					})
				}
			}
		}
	}

	return patterns
}

// TradeCount returns total number of recorded trades
func (ta *TemporalAnalyzer) TradeCount() int {
	ta.mu.RLock()
	defer ta.mu.RUnlock()
	return len(ta.records)
}

type accumulator struct {
	count    int
	wins     int
	totalPnL float64
}

func (a *accumulator) add(pnl float64) {
	a.count++
	a.totalPnL += pnl
	if pnl > 0 {
		a.wins++
	}
}

func formatHour(h int) string {
	return fmt.Sprintf("%02d:00", h)
}

func calculateConfidence(sampleSize int) float64 {
	if sampleSize >= 100 {
		return 0.95
	}
	if sampleSize >= 50 {
		return 0.85
	}
	if sampleSize >= 20 {
		return 0.7
	}
	if sampleSize >= 10 {
		return 0.5
	}
	return 0.3
}
