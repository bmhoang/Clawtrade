# Clawtrade - AI Trading Agent Platform Design

**Date:** 2026-03-13
**Status:** Approved
**Approach:** Plugin-core Architecture (Go core + TypeScript plugin runtime)

---

## 1. Vision

Clawtrade is an open-source, self-hosted AI Agent platform specialized for trading. Similar to OpenClaw but focused on financial markets. Users connect their own LLM (BYOK API key or OAuth) and trading platform APIs to get AI-powered trading assistance.

### Target Users
- **Retail traders** - AI-assisted analysis, signals, automated trading
- **Developers/Quants** - build and deploy trading bots via AI Agent

### Key Principles
- **Self-hosted** - runs on user's machine, full data ownership
- **Open-source** - community-driven, MIT license
- **Multi-platform** - crypto, forex (MQL5), stocks from day one
- **Memory-first** - AI that learns and improves over time
- **Security-first** - trading involves real money, safety is paramount

---

## 2. Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│                    CLAWTRADE                            │
│                                                         │
│  ┌─────────┐  ┌─────────┐  ┌─────────────────────────┐ │
│  │  CLI    │  │  Web UI │  │  REST/WS/GraphQL API   │ │
│  │  (TS)   │  │ (React) │  │  (Go)                   │ │
│  └────┬────┘  └────┬────┘  └────────────┬────────────┘ │
│       │            │                     │              │
│  ┌────▼────────────▼─────────────────────▼────────────┐ │
│  │              CORE ENGINE (Go)                      │ │
│  │  Agent Router | Trading Engine | Memory Engine     │ │
│  │  Adapter Mgr  | Security Module | Event Bus       │ │
│  │  Risk Engine  | Watchdog       | Audit System      │ │
│  └────────────────────┬───────────────────────────────┘ │
│                       │ JSON-RPC (IPC)                  │
│  ┌────────────────────▼───────────────────────────────┐ │
│  │          PLUGIN RUNTIME (TypeScript/Bun)           │ │
│  │  Skills Engine | MCP Servers | LLM Adapters        │ │
│  │  Multi-Agent Orchestrator | Community Plugins      │ │
│  └────────────────────────────────────────────────────┘ │
│                                                         │
│  ┌────────────────────────────────────────────────────┐ │
│  │          PLATFORM ADAPTERS (Go)                    │ │
│  │  CEX: Binance, Bybit, OKX, Coinbase               │ │
│  │  DEX: Uniswap, Jupiter, PancakeSwap, Hyperliquid  │ │
│  │  TradFi: MQL5/MT4/MT5, Alpaca, IBKR, SSI          │ │
│  │  Data: CoinGecko, News, On-chain, Sentiment        │ │
│  │  Signal: TradingView Webhook, Telegram, Copy Trade │ │
│  └────────────────────────────────────────────────────┘ │
│                                                         │
│  ┌────────────────────────────────────────────────────┐ │
│  │          STORAGE (Local)                           │ │
│  │  SQLite (data) | Encrypted Vault (keys)            │ │
│  │  Audit Hash Chain | Memory Files                   │ │
│  └────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
```

### Tech Stack
- **Core Engine:** Go (performance, concurrency, easy to compile)
- **Plugin Runtime:** TypeScript/Bun (community accessibility, npm ecosystem)
- **CLI:** TypeScript (Rich TUI + REPL + Voice)
- **Web Dashboard:** React + TailwindCSS + TradingView Lightweight Charts
- **Database:** SQLite (local-first, zero config)
- **Communication:** JSON-RPC over IPC (Go ↔ TS)

---

## 3. Memory Engine v3 - "Trading Brain"

### Architecture: 5-Layer + Cross-cutting Systems

#### Layers (by access speed):

**Perception Layer:**
- **Sensory Memory** (Ring Buffer, < 0.1ms) - tick-by-tick pattern matching, order book snapshots, volume spikes. Local pattern engine recognizes patterns BEFORE querying LLM.

**Active Layer:**
- **Working Memory** (RAM, < 1ms) - active positions, live P&L, current session context, emotional state tracker.

**Knowledge Layer:**
- **Episodic Memory** (SQLite) - trade episodes with full context: market_state, reasoning, action, outcome, emotion_tag, confidence_at_entry, post-mortem WHY tags.
- **Semantic Memory** (SQLite) - learned rules with confidence scores + evidence count. Market correlations, causal models, user profile.
- **Procedural Memory** (SQLite) - strategy playbooks: step-by-step execution plans that adapt over time based on episodic outcomes.

**Deep Layer:**
- **Archival Memory** (compressed, indexed) - full history, backtesting results, old conversations (LLM-summarized).

#### Cross-cutting Systems:

**Knowledge Graph:**
- Entities: events, assets, trades, strategies, indicators, timeframes, regimes
- Relations: causes, correlates, inverse, precedes, similar_to, used_in
- Auto-discovery: LLM finds new relations
- SQLite + graph model

**Emotional State Tracker:**
- Detects TILT, FOMO, FEAR, OVERCONFIDENCE from user behavior
- Signals: trade frequency, size changes, language sentiment, deviation from strategy
- Actions: warn, reduce size, suggest pause, enforce risk rules
- User can enable/disable

**Meta-Memory:**
- Tracks which memories actually helped make correct decisions
- Memory effectiveness score → auto-promote/demote
- Retriever gets smarter over time

**Temporal Pattern Engine:**
- Time-of-day, day-of-week, seasonal, event-relative patterns
- User-specific patterns: "you trade best 9-11AM"

**Collaborative Memory (Optional, opt-in):**
- Anonymized pattern sharing between users
- Community-validated rules
- Privacy-first, only aggregated data

#### Memory Processes:

**Consolidation ("Sleep & Learn"):**
- Runs periodic (every 50 trades or daily)
- Episodic → Semantic: extract new rules
- Episodic → Procedural: update playbooks
- Semantic + Meta → Prune: remove ineffective rules
- Knowledge Graph: discover new relations
- Temporal: update time-based patterns
- Reflection report

**Smart Retriever v2:**
```
score = relevance × recency × confidence
      × outcome_weight × meta_effectiveness
      × temporal_relevance
```
Pipeline: Intent detection → Graph traversal → Multi-layer scan → Emotional context → Temporal filter → Budget packing → Predictive prefetch

**Forgetting (Regime-Aware):**
- Regime detection: bull/bear/sideways/crisis
- Regime change → old regime memories weight -50% (not deleted)
- Meta-informed: only forget low-effectiveness memories
- User pin: pinned memories never forgotten

---

## 4. Adapter System v2 - "Trading Nervous System"

### Smart Order Router (SOR)
- Scan all connected exchanges
- Aggregate orderbooks across exchanges
- Optimal order splitting for best price
- Modes: BEST_PRICE, LOWEST_FEE, FASTEST_FILL, SINGLE, CUSTOM
- Arbitrage detection: price diff > threshold → alert

### Adapter Categories:

**CEX Adapters:** Binance, Bybit, OKX, Coinbase, Kraken, Gate.io

**DEX Adapters:** Uniswap (ETH), PancakeSwap (BSC), Jupiter (SOL), Hyperliquid (Perps)
- Wallet integration, gas estimation, slippage protection, MEV protection

**TradFi Adapters:** MQL5/MT4/MT5, Alpaca, IBKR, SSI, TradingView

**Data-Only Adapters:** CoinGecko, News APIs, Economic Calendar, On-chain Analytics, Social Sentiment

**Signal Adapters:** TradingView Webhook, Custom webhooks, Copy trading, Telegram/Discord signal parser

**Simulation Adapter (Universal Paper Trading):**
- Wraps any trading adapter
- Real market data + simulated execution
- Realistic fills: slippage, partial fill, latency, fees
- Historical replay for backtesting

### Infrastructure:

**Unified Portfolio Manager:** Cross-exchange portfolio view, risk calculation, rebalancing suggestions

**Failover & Redundancy:** Auto-failover when exchange down, configurable fallback chains, health scoring, circuit breaker

**Adapter Performance Monitor:** Latency, fill rate, scoring → feeds into SOR and Memory Engine

**MQL5 Bridge v2:** TCP/Pipe EA, bi-directional streaming, custom indicator bridge, strategy tester bridge, multi-account

### Unified Interface:
```go
type TradingAdapter interface {
    GetPrice(symbol) → Price
    GetOrderBook(symbol, depth) → OrderBook
    GetCandles(symbol, tf, limit) → []Candle
    SubscribePrice(symbol, callback) → Stream
    PlaceOrder(order) → OrderResult
    CancelOrder(id) → Result
    ModifyOrder(id, params) → Result
    GetOpenOrders() → []Order
    GetBalance() → Balance
    GetPositions() → []Position
    GetTradeHistory(filter) → []Trade
    Capabilities() → AdapterCaps
}

type DataAdapter interface {
    GetData(query) → DataResult
    Subscribe(query, cb) → Stream
    Schema() → DataSchema
}

type SignalAdapter interface {
    OnSignal(cb) → Stream
    ValidateSignal(signal) → Score
}
```

### Community Adapter SDK:
- 3 interface types: Trading, Data, Signal
- Template repos + docs + test harness
- Drop into plugins/adapters/ folder → auto-detect

---

## 5. Security & Risk Management v2 - "Fortress Trading"

### Encrypted Vault
- Key Isolation: vault runs in separate process, only short-lived tokens cross boundary
- AES-256-GCM encryption, Argon2 key derivation
- Master password never stored
- Key rotation reminders (90 days)
- Key permission audit: warn if key has withdraw rights
- Hardware security: YubiKey/FIDO2, TPM, biometric

### AI Guardrails

**Watchdog (Independent Process):**
- Rule-based only (no LLM) → deterministic, not injectable
- Detects: rapid-fire orders, strategy deviation, unfamiliar symbols, abnormal sizing, incoherent reasoning
- If Watchdog process dies → HALT all trading
- User configurable thresholds

**Prompt Injection Shield:**
- Input sanitization
- Context isolation: market data ≠ instructions
- System prompt hardening: risk rules cannot be overridden
- Output validation: AI response must pass Watchdog before execution
- Skill sandboxing: isolated runtime, no vault access

### Risk Engine (4 Layers)

**Layer 1: Pre-Trade Checks** - position size, risk per trade, daily loss, concurrent positions, correlation, leverage, blacklist

**Layer 2: Live Monitoring** - trailing stop, drawdown, liquidation proximity, volatility detection, time-based (pre-news), heartbeat (connection loss → emergency close)

**Layer 3: Circuit Breakers** - daily loss halt, weekly loss halt + manual unlock, consecutive losses pause, flash crash close all, exchange anomaly pause, TILT suggestion, PANIC BUTTON

**Layer 4: Post-Trade Analysis** - auto risk review, rule drift detection, weekly risk report (Sharpe, Sortino, drawdown)

### Advanced Features

**What-If Engine:** Simulate trade impact before execution. Monte Carlo for complex portfolios. Correlation-aware. Shows user before confirm.

**Multi-Signature:** N-of-M approval for team/fund management. Timeout auto-cancel.

**Time-Lock:** Cooling-off period for high-risk config changes (24h for risk params, 48h for full auto-trade, etc.)

**Anti-Manipulation Detection:** Pump & dump, wash trading, stop hunt, spoofing, rug pull (DeFi). Patterns fed to Memory Engine.

**Dead Man's Switch:**
1. Exchange-side SL/TP (server-side, always)
2. Heartbeat monitor (miss 3 → alert → tighten SL → close all)
3. Recovery protocol: reconcile on restart, no auto-trade until user confirms
4. Optional external failsafe VPS watchdog

### Cryptographic Audit Trail
- Hash chain: each entry hashes with previous
- Tamper-evident, verifiable
- Records: WHO, WHAT, WHY, CONTEXT, AI reasoning
- Export: tax reports, P&L, risk reports, SQL-like queries
- Optional: anchor hash on blockchain

### Permission System v2
- Granular scoping: per exchange × symbol × strategy × time × agent
- Permission inheritance: global → exchange → symbol → strategy → time
- Modes: AUTO, CONFIRM, 2FA_CONFIRM, BLOCKED, TIME_WINDOW
- Transfer/Withdraw ALWAYS BLOCKED by default

### Progressive Trust v2
- Promotion: Sandbox (backtest) → Paper → Live (limited) → Live (full)
- Auto-demotion when performance drops, drawdown exceeds limit, or market regime changes
- Configurable gates per stage

### Kill Switch Hierarchy (5 Levels)
1. SOFT PAUSE - no new orders, keep positions
2. DEFENSIVE MODE - tighten SL to breakeven
3. REDUCE EXPOSURE - close 50% at market
4. PANIC CLOSE - close ALL immediately
5. NUCLEAR - close all + revoke API sessions + shutdown

Triggers: manual hotkey/CLI/UI, auto circuit breaker, Telegram /kill, dead man's switch, optional hardware button

---

## 6. Plugin & Skills Ecosystem v2 - "Living Trading Brain"

### Multi-Agent Orchestration

**Conductor Agent** orchestrates specialist agents:
- Analyst Agent: analysis, signals, reports
- Trader Agent: execute orders, manage positions
- Risk Agent: monitor risk, VETO power
- Research Agent: news, events, macro
- Portfolio Agent: balance, hedge, allocate
- Sentiment Agent: social, fear/greed

Communication via Event Bus (Pub/Sub). Risk Agent ALWAYS has veto power. Each agent can use different LLM model (cost optimization). Solo mode: all roles in 1 agent (default).

### Event-Driven Skill System

Skills subscribe to events:
- Market: price.alert, volume.spike, volatility.change, candle.close, orderbook.imbalance, funding.rate.extreme
- Trade: order.filled, position.opened/closed, pnl.threshold, drawdown.warning
- System: schedule.cron, memory.consolidated, regime.change, news.breaking, skill.signal

### Skill Types
- **Analysis:** Technical, On-chain, Sentiment, Fundamental, Intermarket, Orderflow
- **Strategy:** Grid, DCA, Scalping, Swing, Arbitrage, Market Making, Mean Reversion, Momentum
- **Utility:** Portfolio Rebalancer, Tax Calculator, Risk Reporter, Alert Manager, Trade Journal, Backtesting, Performance Dashboard
- **Data:** News, Economic Calendar, Social Sentiment, On-chain, Market Screener

### AI-Generated Skills
- User describes in natural language → AI generates TypeScript skill
- Show code for review → auto-generate tests → sandbox (paper first) → user approve → deploy
- Default permissions: read-only

### Strategy Arena (A/B Testing)
- Run 2+ strategies in parallel on same market
- Paper mode for fair comparison
- Statistical significance check
- Winner auto-promote if user approves
- Historical arena: backtest comparison

### Self-Evolving Skills
- Optimization loop: Execute → Measure → Analyze → Adjust Params
- Only adjust parameters, NOT code
- User approve all changes
- Rollback: parameter history, 1-click revert
- A/B test old vs new params before commit

### Visual Skill Builder (No-Code)
- Drag & drop blocks in Web Dashboard
- Conditions, Logic, Actions, Notifications
- Auto-generate TypeScript from visual blocks
- Instant backtest preview
- Share/import visual skills

### Skill Observability
- Real-time: status, CPU, memory, calls/hour
- Debugging: live logs, execution trace, time-travel, profiling
- Auto-actions: crash restart (max 3), timeout kill, resource throttle, anomaly isolate

### Skill Lifecycle
- Develop → Test → Stage → Live → Retired
- Semver, breaking change detection
- Rollback, hot-swap, canary deploy (10% → 100%)

### MCP Integration
- **As Client:** consume external MCP servers (Browser, Database, custom)
- **As Server:** expose Clawtrade tools to any AI (ChatGPT, Claude, local LLM)

### LLM Adapter Layer
- BYOK: Claude, OpenAI, Gemini, Local LLM (Ollama, LM Studio)
- OAuth: Claude, ChatGPT (when providers support)
- Auto-routing per task type, fallback chain
- Cost optimizer: budget tracking, auto-downgrade near limit
- Prompt caching, compression, batching, streaming

### Community Registry
- Git-based: each skill is a repo
- Reputation system: stars, reviews, verified badge
- Live performance (anonymized, opt-in)
- Skill bounties: community posts bounties for needed skills
- Dependency management: graph, lock file, version resolution

### Skill SDK
```typescript
export default defineSkill({
  name: "skill-name",
  version: "1.0.0",
  permissions: { market_data: "read", trading: "none", memory: "read_write" },
  tools: [{ name: "tool_name", handler: async (input, ctx) => { ... } }],
  on: { "candle.close:BTC/USDT:4h": handler },  // event subscriptions
  widgets: [{ name: "widget", component: Widget }],  // custom UI
  prompts: [{ name: "prompt", template: "..." }],
})
```

Dev tools: `clawtrade dev` (hot-reload), `clawtrade test`, `clawtrade publish`, `clawtrade audit`

---

## 7. Interface Layer v2 - "Trader's Cockpit"

### CLI (TypeScript/Bun)
- **REPL:** natural language + structured commands, tab completion, inline sparkline charts
- **Rich TUI:** full dashboard in terminal (chart, portfolio, chat, alerts), mouse + keyboard, customizable layout, SSH-able
- **Voice:** local Whisper STT, hands-free trading, hotword "Hey Claw"
- **Command mode:** `clawtrade price BTC`, `clawtrade trade buy BTC 0.1`, `clawtrade kill --level 4`
- **Daemon mode:** background process, health check, log tailing
- **Pipe-friendly:** JSON output, composable with unix tools

### Web Dashboard (React + Tailwind)
- **Modular Widget System:** drag & drop, resize, save layout presets (Scalping, Swing, Monitor)
- **Built-in Widgets:** Chart, AI Chat, Portfolio, Positions, Order Book, Trade History, Heatmap, Correlation Matrix, Liquidation Map, Funding Rate, On-chain, Memory Explorer, Emotional Meter, AI Decision Log, Skill Monitor, News Feed, Exposure Map, Drawdown Chart, Circuit Breaker Status, Risk Simulation
- **Command Palette (Ctrl+K):** universal search + command + AI in one, fuzzy matching
- **Context-Aware UI:** adapts based on market state (high volatility → bigger chart, TILT → calming UI)
- **Replay Mode:** time machine to replay past sessions, AI post-analysis, training mode
- **Multi-Monitor:** pop-out widgets, picture-in-picture, save multi-monitor layouts
- **Plugin UI Framework:** skills render custom widgets (sandboxed iframe), theme-aware
- **Dark mode default**, TradingView Lightweight Charts, local-first (served by Go core)

### Mobile Access (PWA)
- Install on phone, native-like experience
- Secure tunnel to home Clawtrade
- Optimized mobile layout, swipe gestures
- Push notifications, biometric auth
- Home screen widget (P&L, price)

### Notification System
- Multi-channel: Telegram, Discord, Email, Push (PWA)
- Per-category priority + channel routing
- Smart throttling: AI decides importance, batch small alerts
- DND mode: only critical alerts pass through
- Learns from user: dismissed alerts → lower priority

### Telegram Bot v2
- 2-way: notifications + interactive commands
- Inline buttons: Approve/Reject/Modify orders
- Daily briefing: portfolio, yesterday P&L, today's outlook, AI recommendation, emotional state
- Commands: /kill, /status, /approve, /chat

### REST + WebSocket + GraphQL API
- REST: standard CRUD endpoints
- WebSocket: real-time streams (prices, orders, AI chat, alerts, skill events, memory updates)
- GraphQL: flexible queries, subscriptions
- Local auth with API tokens
- Optional tunnel for remote access (encrypted)
- Client SDKs: TypeScript, Python, Go
- OpenAPI spec auto-generated

### Accessibility
- Color blind modes (deuteranopia, protanopia, tritanopia)
- High contrast, font scaling, screen reader (ARIA)
- Keyboard-only navigation
- Reduced motion mode
- Audio alerts (distinct sounds per type)
- i18n: multi-language support

---

## 8. Data Flow

Complete request flow for "Long BTC 0.1":

1. **INPUT** → CLI/Web/Telegram/Voice
2. **API Gateway** → auth check, rate limit
3. **Agent Router** → detect intent (TRADE), route to Conductor
4. **Memory Retriever** → pull HOT/SEMANTIC/EPISODIC/EMOTIONAL/TEMPORAL/GRAPH memories
5. **Plugin Runtime** → Conductor dispatches to Analyst + Risk agents
6. **LLM Call** → system prompt + memory context + request → analysis + recommendation
7. **Risk Engine** → pre-trade checks (size, risk%, what-if simulation, correlation, watchdog)
8. **Permission Check** → CONFIRM mode for this size → send to user
9. **User Confirms**
10. **Smart Order Router** → check aggregated orderbook → best price on Binance
11. **Adapter** → place order + set server-side SL (dead man's switch)
12. **Post-Trade** → audit log (hash chain) + memory update + notification + event emit
13. **Response** → fill confirmation with risk summary

### Background Processes (Always Running)
- Watchdog, Memory Consolidation, Event Bus, Heartbeat Monitor
- Risk Live Monitor, Adapter Health Check, Skill Runtime, Notification Dispatcher

---

## 9. Project Structure

```
clawtrade/
├── cmd/clawtrade/main.go          # Go binary entry
├── internal/                       # Go core (private)
│   ├── engine/                     # Agent router, trading, event bus
│   ├── memory/                     # 5-layer memory + graph + retriever
│   ├── adapter/                    # Manager, SOR, aggregator, exchanges/
│   ├── risk/                       # Pre/live/post trade, circuit breaker
│   ├── security/                   # Vault, permissions, watchdog, audit
│   ├── api/                        # REST, GraphQL, WebSocket, MCP server
│   └── plugin/                     # Plugin runtime manager, IPC, sandbox
├── plugins/                        # TypeScript plugin runtime
│   ├── runtime/src/                # Agent orchestrator, LLM, skills, MCP
│   └── skills/                     # Built-in + community skills
├── cli/src/                        # CLI: REPL, TUI, voice, commands
├── web/src/                        # React dashboard: widgets, pages, layouts
├── sdk/                            # Client SDKs (TS, Python, Go)
├── mql5/                           # MetaTrader bridge EA
├── data/                           # Local data (gitignored)
├── config/                         # YAML configs
└── docs/                           # Documentation
```

---

## 10. Monetization

Open-source (MIT license), community-driven. No paid tiers, no marketplace fees. Value comes from community growth and ecosystem.

---

*Design approved on 2026-03-13. Ready for implementation planning.*
