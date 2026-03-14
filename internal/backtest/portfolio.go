package backtest

import (
	"fmt"
	"time"
)

const (
	SideLong  = "long"
	SideShort = "short"
)

// SimPosition represents an open position in the portfolio.
type SimPosition struct {
	Symbol     string
	Side       string
	Size       float64
	EntryPrice float64
	EntryCost  float64
	StopLoss   float64
	TakeProfit float64
	OpenedAt   time.Time
}

// TradeRecord represents a completed (closed) trade.
type TradeRecord struct {
	Symbol     string
	Side       string
	Size       float64
	EntryPrice float64
	ExitPrice  float64
	PnL        float64
	OpenedAt   time.Time
	ClosedAt   time.Time
}

// Portfolio tracks cash, open positions, and closed trade history during a backtest.
type Portfolio struct {
	InitialCash float64
	Cash        float64
	MakerFee    float64
	TakerFee    float64
	Slippage    float64
	Positions   []SimPosition
	Trades      []TradeRecord
}

// NewPortfolio creates a portfolio with the given starting capital and fee/slippage rates.
func NewPortfolio(capital, makerFee, takerFee, slippage float64) *Portfolio {
	return &Portfolio{
		InitialCash: capital,
		Cash:        capital,
		MakerFee:    makerFee,
		TakerFee:    takerFee,
		Slippage:    slippage,
	}
}

// OpenPosition opens a new position, deducting cost + slippage + fee from cash.
func (p *Portfolio) OpenPosition(symbol, side string, size, price float64, at time.Time) error {
	notional := size * price
	slip := notional * p.Slippage
	fee := (notional + slip) * p.TakerFee
	totalCost := notional + slip + fee

	if totalCost > p.Cash {
		return fmt.Errorf("insufficient cash: need %f, have %f", totalCost, p.Cash)
	}

	p.Cash -= totalCost

	p.Positions = append(p.Positions, SimPosition{
		Symbol:     symbol,
		Side:       side,
		Size:       size,
		EntryPrice: price,
		EntryCost:  totalCost,
		OpenedAt:   at,
	})

	return nil
}

// ClosePosition closes the position at the given index and returns the PnL.
func (p *Portfolio) ClosePosition(idx int, exitPrice float64, at time.Time) float64 {
	pos := p.Positions[idx]

	notional := pos.Size * exitPrice
	slip := notional * p.Slippage
	fee := (notional - slip) * p.TakerFee
	netReceived := notional - slip - fee

	var pnl float64
	if pos.Side == SideLong {
		pnl = netReceived - pos.EntryCost
	} else {
		pnl = pos.EntryCost - netReceived
	}

	p.Cash += netReceived

	p.Trades = append(p.Trades, TradeRecord{
		Symbol:     pos.Symbol,
		Side:       pos.Side,
		Size:       pos.Size,
		EntryPrice: pos.EntryPrice,
		ExitPrice:  exitPrice,
		PnL:        pnl,
		OpenedAt:   pos.OpenedAt,
		ClosedAt:   at,
	})

	// Remove position by swapping with last element
	last := len(p.Positions) - 1
	p.Positions[idx] = p.Positions[last]
	p.Positions = p.Positions[:last]

	return pnl
}

// Tick checks all open positions for stop-loss and take-profit triggers at the
// given current price. Returns any trades that were auto-closed.
func (p *Portfolio) Tick(currentPrice float64, at time.Time) []TradeRecord {
	var closed []TradeRecord

	// Iterate backwards so removal doesn't shift indices we haven't visited.
	for i := len(p.Positions) - 1; i >= 0; i-- {
		pos := p.Positions[i]
		var triggered bool
		var triggerPrice float64

		if pos.Side == SideLong {
			if pos.StopLoss > 0 && currentPrice <= pos.StopLoss {
				triggered = true
				triggerPrice = pos.StopLoss
			} else if pos.TakeProfit > 0 && currentPrice >= pos.TakeProfit {
				triggered = true
				triggerPrice = pos.TakeProfit
			}
		} else { // short
			if pos.StopLoss > 0 && currentPrice >= pos.StopLoss {
				triggered = true
				triggerPrice = pos.StopLoss
			} else if pos.TakeProfit > 0 && currentPrice <= pos.TakeProfit {
				triggered = true
				triggerPrice = pos.TakeProfit
			}
		}

		if triggered {
			pnl := p.ClosePosition(i, triggerPrice, at)
			tr := p.Trades[len(p.Trades)-1]
			_ = pnl
			closed = append(closed, tr)
		}
	}

	return closed
}

// Equity returns the total portfolio value: cash plus marked-to-market positions.
func (p *Portfolio) Equity(currentPrice float64) float64 {
	equity := p.Cash
	for _, pos := range p.Positions {
		equity += pos.Size * currentPrice
	}
	return equity
}
