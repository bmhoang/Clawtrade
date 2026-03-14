# Dashboard Redesign Design

**Goal:** Redesign the frontend dashboard to be real-time first, using TradingView widget for charts, WebSocket-driven data for all panels, and a right sidebar with AI chat + agent insights.

**Approach:** Full rewrite of key components with Tailwind-only styling, WebSocket custom hook for centralized real-time data, TradingView embed widget replacing custom SVG charts.

---

## Layout

```
┌──────────────────────────────────────────────────────────────┐
│  Header: Logo | Dashboard | Chat | Positions | Analytics |   │
│          Strategies | Settings              [exchange status] │
├────────────────────────────────────────┬─────────────────────┤
│                                        │  Right Sidebar      │
│  Main Area                             │                     │
│  ┌──────────────────────────────────┐  │  ┌───────────────┐  │
│  │  TradingView Widget (embed)     │  │  │ AI Chat       │  │
│  │  Full candlestick chart         │  │  │ (ChatPanel)   │  │
│  │  Symbol selector + timeframe    │  │  │               │  │
│  └──────────────────────────────────┘  │  └───────────────┘  │
│  ┌──────────┬───────────────────────┐  │  ┌───────────────┐  │
│  │Portfolio │  Market Overview      │  │  │ Agent Insights│  │
│  │Summary   │  (live prices via WS) │  │  │ (live stream) │  │
│  │(live WS) │                       │  │  │ analysis,     │  │
│  └──────────┴───────────────────────┘  │  │ counter,      │  │
│  ┌──────────────────────────────────┐  │  │ narrative,    │  │
│  │  Recent Trades (live WS)        │  │  │ correlation   │  │
│  │  + Trade notifications toast    │  │  └───────────────┘  │
│  └──────────────────────────────────┘  │                     │
├────────────────────────────────────────┴─────────────────────┤
│  Status bar: WS connected | 3 agents running | last update   │
└──────────────────────────────────────────────────────────────┘
```

## Key Decisions

1. **TradingView Widget (embed)** — not Lightweight Charts. Uses TradingView's built-in data feeds, full-featured charting with indicators, drawing tools, etc. Zero maintenance for chart rendering.

2. **Right sidebar** — AI Chat + Agent Insights always visible alongside main content. No tab switching needed to see agent analysis while viewing charts.

3. **WebSocket-first** — all live data (prices, portfolio, trades, agent insights) flows through a centralized `useWebSocket()` hook. Components subscribe to event types they need.

4. **Tailwind-only styling** — remove all inline styles, use Tailwind classes consistently. Keep existing dark theme CSS variables (`--bg-0` through `--bg-3`, `--accent`, etc.) as Tailwind custom colors.

5. **Keep existing dark theme** — zinc/indigo color scheme (`#09090b` bg, `#6366f1` accent). Green/red for PnL.

## Data Flow

```
Backend EventBus ──→ WebSocket Hub ──→ Browser WS ──→ useWebSocket() hook
                                                          │
                                    ┌─────────────────────┼──────────────────┐
                                    │                     │                  │
                              price.update          agent.*          portfolio.update
                                    │                     │                  │
                              MarketOverview      AgentInsights      PortfolioSummary
                                                                           │
                                                                     trade.executed
                                                                           │
                                                                    RecentTrades + Toast
```

## WebSocket Event Subscriptions

| Component | Subscribes To | Data Used |
|-----------|--------------|-----------|
| MarketOverview | `price.update` | symbol, last, bid, ask, volume_24h, change_pct |
| PortfolioSummary | `portfolio.update` | balances, positions, total_pnl |
| RecentTrades | `trade.executed` | symbol, side, size, price, status |
| TradeToast | `trade.executed` | symbol, side, price (toast notification) |
| AgentInsights | `agent.analysis`, `agent.counter`, `agent.narrative`, `agent.reflection`, `agent.correlation` | source, symbol, summary, data |
| StatusBar | connection state | connected/disconnected, last event time |

## Components

### New Components

| Component | Purpose |
|-----------|---------|
| `useWebSocket.ts` | Custom hook: connect, subscribe to event types, auto-reconnect, connection state |
| `AgentInsights.tsx` | Live stream of sub-agent events with summary display + expandable details |
| `TradeToast.tsx` | Toast notification popup when trade.executed fires |
| `StatusBar.tsx` | Footer: WS connection status, active agents count, last update timestamp |
| `TradingViewChart.tsx` | TradingView Advanced Chart widget embed wrapper |

### Rewritten Components

| Component | Changes |
|-----------|---------|
| `App.tsx` | New 2-column layout (main + right sidebar), Tailwind classes |
| `PriceChart.tsx` | Replace custom SVG → TradingView widget embed |
| `MarketOverview.tsx` | Live prices via useWebSocket instead of polling |
| `PortfolioSummary.tsx` | Live updates via useWebSocket instead of fetch |
| `RecentTrades.tsx` | Live trade feed via useWebSocket |
| `Header.tsx` | Tailwind styling, exchange connection indicator |

### Unchanged Components

| Component | Reason |
|-----------|--------|
| `ChatPanel.tsx` | Already functional, just move to sidebar |
| `SettingsPanel.tsx` | Configuration UI, no real-time needs |
| `SkillBuilder.tsx` | Strategy builder, standalone |
| `ReplayMode.tsx` | Backtest playback, standalone |
| `AgentStatus.tsx` | Already shows agent status |

## useWebSocket Hook API

```typescript
const { subscribe, unsubscribe, connected, lastEvent } = useWebSocket()

// Subscribe to event type
useEffect(() => {
  const unsub = subscribe('price.update', (data) => {
    // handle price update
  })
  return unsub
}, [])
```

- Auto-connects on mount
- Auto-reconnects with exponential backoff
- Sends subscribe messages to server for each event type
- Returns connection state for StatusBar
- Provides lastEvent timestamp

## Agent Insights Panel

Displays a scrollable feed of sub-agent events:

```
┌─ Agent Insights ──────────────────┐
│                                    │
│ 🔍 market-analyst: BTC/USDT       │
│    Bullish 78% — SMC + PA at 64k  │
│    [expand for details]      2m ago│
│                                    │
│ ⚔️ devils-advocate: BTC/USDT      │
│    Bearish divergence on RSI...    │
│    [expand]                  2m ago│
│                                    │
│ 📖 narrative: BTC/USDT            │
│    "Institutional accumulation"    │
│    Phase: growing            5m ago│
│                                    │
│ 🔗 correlation:                    │
│    BTC-ETH 0.92, regime: trending  │
│                             10m ago│
└────────────────────────────────────┘
```

- Each event shows: icon, source, symbol, summary
- Click to expand and see full raw data
- Auto-scrolls to latest
- Max 50 events in memory (oldest dropped)
