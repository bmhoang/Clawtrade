# Dashboard Redesign Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Redesign the dashboard to be real-time first with TradingView widget, WebSocket-driven data, right sidebar with AI chat + agent insights, and consistent Tailwind styling.

**Architecture:** A custom `useWebSocket` hook centralizes all real-time data subscriptions. Components subscribe to specific event types and update reactively. TradingView Advanced Chart widget embeds replace the custom SVG chart. Layout shifts to 2-column (main + right sidebar).

**Tech Stack:** React 18, TypeScript, Vite 6, Tailwind CSS 4, TradingView Widget (embed), WebSocket

---

### Task 1: useWebSocket custom hook

**Files:**
- Create: `web/src/hooks/useWebSocket.ts`

**Step 1: Write the hook**

```typescript
// web/src/hooks/useWebSocket.ts
import { useEffect, useRef, useCallback, useState } from 'react'

type EventCallback = (data: Record<string, unknown>) => void

interface UseWebSocketReturn {
  subscribe: (eventType: string, callback: EventCallback) => () => void
  connected: boolean
  lastEventTime: number | null
}

const DEV_BASE = window.location.port === '5173' ? 'http://127.0.0.1:8899' : ''

export function useWebSocket(): UseWebSocketReturn {
  const wsRef = useRef<WebSocket | null>(null)
  const listenersRef = useRef<Map<string, Set<EventCallback>>>(new Map())
  const subscribedRef = useRef<Set<string>>(new Set())
  const [connected, setConnected] = useState(false)
  const [lastEventTime, setLastEventTime] = useState<number | null>(null)
  const reconnectTimer = useRef<ReturnType<typeof setTimeout>>()
  const reconnectDelay = useRef(1000)

  const connect = useCallback(() => {
    const wsBase = DEV_BASE || `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}`
    const wsUrl = `${wsBase.replace(/^http/, 'ws')}/ws`
    const ws = new WebSocket(wsUrl)

    ws.onopen = () => {
      setConnected(true)
      reconnectDelay.current = 1000
      // Re-subscribe to all event types
      for (const eventType of subscribedRef.current) {
        ws.send(JSON.stringify({ type: 'subscribe', data: eventType }))
      }
    }

    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data)
        setLastEventTime(Date.now())
        const callbacks = listenersRef.current.get(msg.type)
        if (callbacks) {
          for (const cb of callbacks) {
            cb(msg.data)
          }
        }
      } catch {
        // ignore non-JSON
      }
    }

    ws.onclose = () => {
      setConnected(false)
      reconnectTimer.current = setTimeout(() => {
        reconnectDelay.current = Math.min(reconnectDelay.current * 2, 30000)
        connect()
      }, reconnectDelay.current)
    }

    ws.onerror = () => ws.close()

    wsRef.current = ws
  }, [])

  useEffect(() => {
    connect()
    return () => {
      clearTimeout(reconnectTimer.current)
      wsRef.current?.close()
    }
  }, [connect])

  const subscribe = useCallback((eventType: string, callback: EventCallback) => {
    if (!listenersRef.current.has(eventType)) {
      listenersRef.current.set(eventType, new Set())
    }
    listenersRef.current.get(eventType)!.add(callback)

    // Send subscribe message to server if not already subscribed
    if (!subscribedRef.current.has(eventType)) {
      subscribedRef.current.add(eventType)
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        wsRef.current.send(JSON.stringify({ type: 'subscribe', data: eventType }))
      }
    }

    // Return unsubscribe function
    return () => {
      const set = listenersRef.current.get(eventType)
      if (set) {
        set.delete(callback)
        if (set.size === 0) {
          listenersRef.current.delete(eventType)
        }
      }
    }
  }, [])

  return { subscribe, connected, lastEventTime }
}
```

**Step 2: Verify it compiles**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

**Step 3: Commit**

```bash
git add web/src/hooks/useWebSocket.ts
git commit -m "feat(web): add useWebSocket custom hook with auto-reconnect"
```

---

### Task 2: WebSocket context provider

**Files:**
- Create: `web/src/hooks/WebSocketProvider.tsx`

**Step 1: Create the provider**

```typescript
// web/src/hooks/WebSocketProvider.tsx
import { createContext, useContext, type ReactNode } from 'react'
import { useWebSocket } from './useWebSocket'

type EventCallback = (data: Record<string, unknown>) => void

interface WebSocketContextValue {
  subscribe: (eventType: string, callback: EventCallback) => () => void
  connected: boolean
  lastEventTime: number | null
}

const WebSocketContext = createContext<WebSocketContextValue | null>(null)

export function WebSocketProvider({ children }: { children: ReactNode }) {
  const ws = useWebSocket()
  return (
    <WebSocketContext.Provider value={ws}>
      {children}
    </WebSocketContext.Provider>
  )
}

export function useWS(): WebSocketContextValue {
  const ctx = useContext(WebSocketContext)
  if (!ctx) throw new Error('useWS must be used within WebSocketProvider')
  return ctx
}
```

**Step 2: Wrap App in provider**

In `web/src/main.tsx`, wrap `<App />` with `<WebSocketProvider>`:

```typescript
import { WebSocketProvider } from './hooks/WebSocketProvider'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <WebSocketProvider>
      <App />
    </WebSocketProvider>
  </StrictMode>,
)
```

**Step 3: Verify it compiles**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

**Step 4: Commit**

```bash
git add web/src/hooks/WebSocketProvider.tsx web/src/main.tsx
git commit -m "feat(web): add WebSocketProvider context for shared WS connection"
```

---

### Task 3: TradingView chart component

**Files:**
- Create: `web/src/components/TradingViewChart.tsx`

**Step 1: Create TradingView widget wrapper**

```typescript
// web/src/components/TradingViewChart.tsx
import { useEffect, useRef, useState } from 'react'

const SYMBOLS: { label: string; tv: string }[] = [
  { label: 'BTC/USDT', tv: 'BINANCE:BTCUSDT' },
  { label: 'ETH/USDT', tv: 'BINANCE:ETHUSDT' },
  { label: 'SOL/USDT', tv: 'BINANCE:SOLUSDT' },
  { label: 'BNB/USDT', tv: 'BINANCE:BNBUSDT' },
  { label: 'EUR/USD', tv: 'FX:EURUSD' },
  { label: 'XAU/USD', tv: 'TVC:GOLD' },
  { label: 'AAPL', tv: 'NASDAQ:AAPL' },
  { label: 'NVDA', tv: 'NASDAQ:NVDA' },
]

export default function TradingViewChart() {
  const containerRef = useRef<HTMLDivElement>(null)
  const [symbol, setSymbol] = useState(SYMBOLS[0])

  useEffect(() => {
    if (!containerRef.current) return
    containerRef.current.innerHTML = ''

    const script = document.createElement('script')
    script.src = 'https://s3.tradingview.com/external-embedding/embed-widget-advanced-chart.js'
    script.type = 'text/javascript'
    script.async = true
    script.innerHTML = JSON.stringify({
      autosize: true,
      symbol: symbol.tv,
      interval: '60',
      timezone: 'Etc/UTC',
      theme: 'dark',
      style: '1',
      locale: 'en',
      backgroundColor: 'rgba(9, 9, 11, 1)',
      gridColor: 'rgba(255, 255, 255, 0.03)',
      hide_top_toolbar: false,
      hide_legend: false,
      save_image: false,
      calendar: false,
      hide_volume: false,
      support_host: 'https://www.tradingview.com',
    })

    const wrapper = document.createElement('div')
    wrapper.className = 'tradingview-widget-container__widget'
    wrapper.style.height = '100%'
    wrapper.style.width = '100%'

    containerRef.current.appendChild(wrapper)
    containerRef.current.appendChild(script)
  }, [symbol])

  return (
    <div className="card fade-in flex flex-col h-full" style={{ animationDelay: '100ms' }}>
      {/* Symbol selector */}
      <div className="flex items-center gap-2 px-4 py-2.5 border-b border-white/[0.06]">
        <span className="text-xs font-bold text-[var(--text-1)]">Chart</span>
        <div className="flex gap-1 ml-2">
          {SYMBOLS.map(s => (
            <button
              key={s.tv}
              onClick={() => setSymbol(s)}
              className={`px-2 py-0.5 rounded text-[10px] font-semibold border-none cursor-pointer transition-all ${
                symbol.tv === s.tv
                  ? 'bg-[rgba(99,102,241,0.1)] text-[#818cf8]'
                  : 'bg-transparent text-[var(--text-3)] hover:text-[var(--text-2)]'
              }`}
            >
              {s.label}
            </button>
          ))}
        </div>
      </div>
      {/* TradingView widget */}
      <div
        ref={containerRef}
        className="tradingview-widget-container flex-1 min-h-0"
        style={{ overflow: 'hidden' }}
      />
    </div>
  )
}
```

**Step 2: Verify it compiles**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

**Step 3: Commit**

```bash
git add web/src/components/TradingViewChart.tsx
git commit -m "feat(web): add TradingView Advanced Chart widget component"
```

---

### Task 4: AgentInsights component

**Files:**
- Create: `web/src/components/AgentInsights.tsx`

**Step 1: Create agent insights panel**

```typescript
// web/src/components/AgentInsights.tsx
import { useState, useEffect, useRef } from 'react'
import { useWS } from '../hooks/WebSocketProvider'

interface InsightEvent {
  id: number
  type: string
  source: string
  symbol: string
  summary: string
  data: Record<string, unknown>
  time: number
}

const AGENT_EVENTS = [
  'agent.analysis',
  'agent.counter',
  'agent.narrative',
  'agent.reflection',
  'agent.correlation',
]

const ICONS: Record<string, string> = {
  'agent.analysis': '\u{1F50D}',
  'agent.counter': '\u{2694}\uFE0F',
  'agent.narrative': '\u{1F4D6}',
  'agent.reflection': '\u{1FA9E}',
  'agent.correlation': '\u{1F517}',
}

const LABELS: Record<string, string> = {
  'agent.analysis': 'Analysis',
  'agent.counter': 'Counter',
  'agent.narrative': 'Narrative',
  'agent.reflection': 'Reflection',
  'agent.correlation': 'Correlation',
}

function timeAgo(ts: number): string {
  const diff = Math.floor((Date.now() - ts) / 1000)
  if (diff < 60) return `${diff}s ago`
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`
  return `${Math.floor(diff / 3600)}h ago`
}

let nextId = 0

export default function AgentInsights() {
  const { subscribe } = useWS()
  const [events, setEvents] = useState<InsightEvent[]>([])
  const [expanded, setExpanded] = useState<number | null>(null)
  const scrollRef = useRef<HTMLDivElement>(null)
  const [, setTick] = useState(0)

  // Subscribe to all agent event types
  useEffect(() => {
    const unsubs = AGENT_EVENTS.map(eventType =>
      subscribe(eventType, (data) => {
        const event: InsightEvent = {
          id: nextId++,
          type: eventType,
          source: (data.source as string) || 'unknown',
          symbol: (data.symbol as string) || '',
          summary: (data.summary as string) || '',
          data: (data.data as Record<string, unknown>) || {},
          time: Date.now(),
        }
        setEvents(prev => {
          const updated = [event, ...prev]
          return updated.slice(0, 50) // keep max 50
        })
      })
    )
    return () => unsubs.forEach(fn => fn())
  }, [subscribe])

  // Update relative times every 30s
  useEffect(() => {
    const timer = setInterval(() => setTick(t => t + 1), 30000)
    return () => clearInterval(timer)
  }, [])

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-4 py-2.5 border-b border-white/[0.06]">
        <span className="text-xs font-bold text-[var(--text-1)]">Agent Insights</span>
        <span className="text-[10px] text-[var(--text-3)]">{events.length} events</span>
      </div>

      <div ref={scrollRef} className="flex-1 overflow-y-auto">
        {events.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full text-center px-4">
            <div className="text-2xl mb-2 opacity-30">\u{1F916}</div>
            <p className="text-[11px] text-[var(--text-3)]">Waiting for agent insights...</p>
            <p className="text-[9px] text-[var(--text-3)] mt-1">Sub-agents will stream analysis here</p>
          </div>
        ) : (
          events.map(ev => (
            <div
              key={ev.id}
              className="px-4 py-3 border-b border-white/[0.03] cursor-pointer hover:bg-white/[0.02] transition-colors"
              onClick={() => setExpanded(expanded === ev.id ? null : ev.id)}
            >
              <div className="flex items-start gap-2">
                <span className="text-sm mt-0.5">{ICONS[ev.type] || '\u{1F4AC}'}</span>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-[10px] font-semibold text-[#818cf8]">
                      {LABELS[ev.type] || ev.type}
                    </span>
                    {ev.symbol && (
                      <span className="text-[9px] font-medium text-[var(--text-2)]">{ev.symbol}</span>
                    )}
                    <span className="text-[9px] text-[var(--text-3)] ml-auto shrink-0">
                      {timeAgo(ev.time)}
                    </span>
                  </div>
                  <p className="text-[11px] text-[var(--text-2)] mt-1 leading-relaxed">
                    {ev.summary}
                  </p>
                  {expanded === ev.id && (
                    <pre className="text-[9px] text-[var(--text-3)] mt-2 p-2 rounded bg-[var(--bg-0)] overflow-x-auto whitespace-pre-wrap">
                      {JSON.stringify(ev.data, null, 2)}
                    </pre>
                  )}
                </div>
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  )
}
```

**Step 2: Verify it compiles**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

**Step 3: Commit**

```bash
git add web/src/components/AgentInsights.tsx
git commit -m "feat(web): add AgentInsights live event stream component"
```

---

### Task 5: TradeToast notification component

**Files:**
- Create: `web/src/components/TradeToast.tsx`

**Step 1: Create toast component**

```typescript
// web/src/components/TradeToast.tsx
import { useState, useEffect } from 'react'
import { useWS } from '../hooks/WebSocketProvider'

interface Toast {
  id: number
  symbol: string
  side: string
  price: number
  size: number
  time: number
}

let toastId = 0

export default function TradeToast() {
  const { subscribe } = useWS()
  const [toasts, setToasts] = useState<Toast[]>([])

  useEffect(() => {
    return subscribe('trade.executed', (data) => {
      const toast: Toast = {
        id: toastId++,
        symbol: (data.symbol as string) || '',
        side: (data.side as string) || '',
        price: (data.price as number) || 0,
        size: (data.size as number) || 0,
        time: Date.now(),
      }
      setToasts(prev => [...prev, toast])

      // Auto-dismiss after 5 seconds
      setTimeout(() => {
        setToasts(prev => prev.filter(t => t.id !== toast.id))
      }, 5000)
    })
  }, [subscribe])

  if (!toasts.length) return null

  return (
    <div className="fixed top-4 right-4 z-50 flex flex-col gap-2">
      {toasts.map(t => {
        const isBuy = t.side.toLowerCase() === 'buy'
        return (
          <div
            key={t.id}
            className="fade-in rounded-lg border px-4 py-3 shadow-lg min-w-[280px]"
            style={{
              background: 'var(--bg-1)',
              borderColor: isBuy ? 'rgba(16,185,129,0.2)' : 'rgba(239,68,68,0.2)',
            }}
          >
            <div className="flex items-center gap-3">
              <div
                className="w-8 h-8 rounded-lg flex items-center justify-center text-xs font-bold"
                style={{
                  background: isBuy ? 'rgba(16,185,129,0.1)' : 'rgba(239,68,68,0.1)',
                  color: isBuy ? '#10b981' : '#ef4444',
                }}
              >
                {isBuy ? 'B' : 'S'}
              </div>
              <div>
                <div className="text-[12px] font-semibold text-[var(--text-1)]">
                  {t.side} {t.symbol}
                </div>
                <div className="mono text-[11px] text-[var(--text-2)]">
                  {t.size} @ ${t.price.toLocaleString()}
                </div>
              </div>
            </div>
          </div>
        )
      })}
    </div>
  )
}
```

**Step 2: Verify it compiles**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

**Step 3: Commit**

```bash
git add web/src/components/TradeToast.tsx
git commit -m "feat(web): add TradeToast notification for live trade events"
```

---

### Task 6: StatusBar component

**Files:**
- Create: `web/src/components/StatusBar.tsx`

**Step 1: Create status bar**

```typescript
// web/src/components/StatusBar.tsx
import { useState, useEffect } from 'react'
import { useWS } from '../hooks/WebSocketProvider'

export default function StatusBar() {
  const { connected, lastEventTime } = useWS()
  const [, setTick] = useState(0)

  // Update relative time every 5s
  useEffect(() => {
    const timer = setInterval(() => setTick(t => t + 1), 5000)
    return () => clearInterval(timer)
  }, [])

  const lastUpdate = lastEventTime
    ? `${Math.floor((Date.now() - lastEventTime) / 1000)}s ago`
    : 'No events yet'

  return (
    <div className="flex items-center justify-between px-4 py-1.5 border-t border-white/[0.06] bg-[var(--bg-1)]">
      <div className="flex items-center gap-4">
        <div className="flex items-center gap-1.5">
          <div
            className={`w-1.5 h-1.5 rounded-full ${connected ? 'bg-[#10b981] pulse-dot' : 'bg-[#ef4444]'}`}
          />
          <span className="text-[10px] text-[var(--text-3)]">
            {connected ? 'WebSocket Connected' : 'Disconnected'}
          </span>
        </div>
      </div>
      <span className="text-[10px] text-[var(--text-3)]">
        Last update: {lastUpdate}
      </span>
    </div>
  )
}
```

**Step 2: Verify it compiles**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

**Step 3: Commit**

```bash
git add web/src/components/StatusBar.tsx
git commit -m "feat(web): add StatusBar with WebSocket connection status"
```

---

### Task 7: Rewrite MarketOverview with WebSocket

**Files:**
- Modify: `web/src/components/MarketOverview.tsx`

**Step 1: Rewrite MarketOverview to use useWS**

Replace the entire file:

```typescript
// web/src/components/MarketOverview.tsx
import { useState, useEffect } from 'react'
import { useWS } from '../hooks/WebSocketProvider'
import { fetchPrice, type PriceData } from '../api/client'

type AC = 'all' | 'crypto' | 'forex' | 'stocks' | 'index'

interface MarketItem {
  sym: string
  price: number
  change: number
  exchange: string
  class: AC
  live: boolean
}

const SYMBOLS: { sym: string; exchange: string; class: AC }[] = [
  { sym: 'BTC/USDT', exchange: 'Binance', class: 'crypto' },
  { sym: 'ETH/USDT', exchange: 'Binance', class: 'crypto' },
  { sym: 'SOL/USDT', exchange: 'Binance', class: 'crypto' },
  { sym: 'BNB/USDT', exchange: 'Binance', class: 'crypto' },
  { sym: 'EUR/USD', exchange: 'MT5', class: 'forex' },
  { sym: 'GBP/JPY', exchange: 'MT5', class: 'forex' },
  { sym: 'XAU/USD', exchange: 'MT5', class: 'forex' },
  { sym: 'AAPL', exchange: 'IBKR', class: 'stocks' },
  { sym: 'NVDA', exchange: 'IBKR', class: 'stocks' },
  { sym: 'TSLA', exchange: 'IBKR', class: 'stocks' },
  { sym: 'NQ100', exchange: 'CME', class: 'index' },
  { sym: 'SPX', exchange: 'CME', class: 'index' },
  { sym: 'VIX', exchange: 'CME', class: 'index' },
]

const FALLBACK: MarketItem[] = [
  { sym: 'BTC/USDT', price: 70245, change: 2.34, exchange: 'Binance', class: 'crypto', live: false },
  { sym: 'ETH/USDT', price: 3382, change: -1.12, exchange: 'Binance', class: 'crypto', live: false },
  { sym: 'SOL/USDT', price: 172.3, change: 5.67, exchange: 'Bybit', class: 'crypto', live: false },
  { sym: 'BNB/USDT', price: 612.4, change: 0.89, exchange: 'Binance', class: 'crypto', live: false },
  { sym: 'EUR/USD', price: 1.0847, change: -0.12, exchange: 'MT5', class: 'forex', live: false },
  { sym: 'GBP/JPY', price: 196.42, change: 0.34, exchange: 'MT5', class: 'forex', live: false },
  { sym: 'XAU/USD', price: 2345, change: 0.89, exchange: 'MT5', class: 'forex', live: false },
  { sym: 'AAPL', price: 198.52, change: 1.05, exchange: 'IBKR', class: 'stocks', live: false },
  { sym: 'NVDA', price: 875.3, change: 3.42, exchange: 'IBKR', class: 'stocks', live: false },
  { sym: 'TSLA', price: 245.6, change: -2.15, exchange: 'IBKR', class: 'stocks', live: false },
  { sym: 'NQ100', price: 18420, change: 0.67, exchange: 'CME', class: 'index', live: false },
  { sym: 'SPX', price: 5234, change: 0.42, exchange: 'CME', class: 'index', live: false },
  { sym: 'VIX', price: 14.8, change: -5.12, exchange: 'CME', class: 'index', live: false },
]

const tabs: { id: AC; label: string }[] = [
  { id: 'all', label: 'All' },
  { id: 'crypto', label: 'Crypto' },
  { id: 'forex', label: 'Forex' },
  { id: 'stocks', label: 'Stocks' },
  { id: 'index', label: 'Index' },
]

function fmt(p: number, s: string) {
  if (p >= 10000) return `$${(p / 1000).toFixed(1)}k`
  if (p >= 100) return `$${p.toLocaleString()}`
  if (s.includes('/') && p < 10) return p.toFixed(4)
  return `$${p.toFixed(1)}`
}

export default function MarketOverview() {
  const { subscribe } = useWS()
  const [tab, setTab] = useState<AC>('all')
  const [markets, setMarkets] = useState<MarketItem[]>(FALLBACK)
  const [hasLive, setHasLive] = useState(false)

  // Initial fetch for crypto prices
  useEffect(() => {
    let cancelled = false
    const cryptoSymbols = SYMBOLS.filter(s => s.class === 'crypto')
    Promise.allSettled(
      cryptoSymbols.map(s => fetchPrice(s.sym, 'binance'))
    ).then(results => {
      if (cancelled) return
      const updated = [...FALLBACK]
      results.forEach((r, i) => {
        if (r.status === 'fulfilled') {
          const p = r.value as PriceData
          const idx = updated.findIndex(m => m.sym === cryptoSymbols[i].sym)
          if (idx >= 0) {
            const prevPrice = updated[idx].price
            const change = prevPrice > 0 ? ((p.last - prevPrice) / prevPrice * 100) : 0
            updated[idx] = { ...updated[idx], price: p.last, change: +change.toFixed(2), live: true }
            setHasLive(true)
          }
        }
      })
      setMarkets(updated)
    })
    return () => { cancelled = true }
  }, [])

  // Subscribe to real-time price updates
  useEffect(() => {
    return subscribe('price.update', (data) => {
      const symbol = data.symbol as string
      const last = data.last as number
      const changePct = data.change_pct as number
      if (!symbol || !last) return

      setMarkets(prev => prev.map(m =>
        m.sym === symbol
          ? { ...m, price: last, change: +changePct.toFixed(2), live: true }
          : m
      ))
      setHasLive(true)
    })
  }, [subscribe])

  const filtered = tab === 'all' ? markets : markets.filter(m => m.class === tab)

  return (
    <div className="card fade-in flex flex-col h-full min-h-0" style={{ animationDelay: '200ms' }}>
      <div className="flex items-center justify-between px-5 py-2.5 border-b border-white/[0.06]">
        <div className="flex items-center gap-2">
          <span className="text-xs font-bold text-[var(--text-1)]">Markets</span>
          {hasLive && (
            <span className="text-[8px] font-bold px-1.5 py-0.5 rounded text-[#10b981] bg-[rgba(16,185,129,0.1)]">
              LIVE
            </span>
          )}
        </div>
        <div className="flex gap-0.5">
          {tabs.map(t => (
            <button
              key={t.id}
              onClick={() => setTab(t.id)}
              className={`px-2 py-1 rounded text-[10px] font-semibold border-none cursor-pointer transition-all ${
                tab === t.id
                  ? 'bg-[rgba(99,102,241,0.1)] text-[#818cf8]'
                  : 'bg-transparent text-[var(--text-3)]'
              }`}
            >
              {t.label}
            </button>
          ))}
        </div>
      </div>

      <div className="flex-1 overflow-auto">
        {filtered.map(m => {
          const up = m.change >= 0
          return (
            <div
              key={m.sym}
              className="flex items-center justify-between px-5 py-2.5 border-b border-white/[0.025] cursor-pointer hover:bg-white/[0.02] transition-colors"
            >
              <div className="flex items-center gap-2">
                <span className="text-xs font-semibold text-[var(--text-1)]">{m.sym}</span>
                <span className="text-[9px] text-[var(--text-3)]">{m.exchange}</span>
                {m.live && <div className="w-1 h-1 rounded-full bg-[#10b981]" />}
              </div>
              <div className="flex items-center gap-4">
                <span className="mono text-xs text-[var(--text-1)] font-medium">{fmt(m.price, m.sym)}</span>
                <span className={`mono text-[11px] font-semibold min-w-[52px] text-right ${up ? 'text-[#10b981]' : 'text-[#ef4444]'}`}>
                  {up ? '+' : ''}{m.change}%
                </span>
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
```

**Step 2: Verify it compiles**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

**Step 3: Commit**

```bash
git add web/src/components/MarketOverview.tsx
git commit -m "feat(web): rewrite MarketOverview with live WebSocket price updates"
```

---

### Task 8: Rewrite PortfolioSummary with WebSocket

**Files:**
- Modify: `web/src/components/PortfolioSummary.tsx`

**Step 1: Rewrite with useWS**

Replace the entire file:

```typescript
// web/src/components/PortfolioSummary.tsx
import { useState, useEffect } from 'react'
import { useWS } from '../hooks/WebSocketProvider'
import { fetchBalances, fetchPositions, type BalanceData, type PositionData } from '../api/client'

interface StatItem {
  label: string
  value: string
  change: string
  pct: string
  up: boolean
}

const FALLBACK: StatItem[] = [
  { label: 'Portfolio Value', value: '$10,245.80', change: '+$245.80', pct: '+2.45%', up: true },
  { label: 'Unrealized P&L', value: '+$747.00', change: '6 positions', pct: '+7.29%', up: true },
  { label: "Today's P&L", value: '-$45.20', change: 'Since 00:00 UTC', pct: '-0.44%', up: false },
  { label: 'Win Rate', value: '68.5%', change: '94W / 43L', pct: '1.85 R:R', up: true },
]

function fmtUsd(n: number): string {
  const abs = Math.abs(n)
  if (abs >= 1000000) return `${n >= 0 ? '' : '-'}$${(abs / 1000000).toFixed(2)}M`
  if (abs >= 1000) return `${n >= 0 ? '' : '-'}$${abs.toLocaleString(undefined, { maximumFractionDigits: 2 })}`
  return `${n >= 0 ? '' : '-'}$${abs.toFixed(2)}`
}

function buildStats(balances: BalanceData[], positions: PositionData[], totalPnl?: number): StatItem[] {
  const totalValue = balances.reduce((s, b) => s + b.total, 0)
  const pnl = totalPnl ?? positions.reduce((s, p) => s + p.pnl, 0)
  const pnlPct = totalValue > 0 ? (pnl / totalValue * 100) : 0

  return [
    {
      label: 'Portfolio Value',
      value: fmtUsd(totalValue),
      change: `${balances.length} assets`,
      pct: totalValue > 0 ? '+0.00%' : '0.00%',
      up: true,
    },
    {
      label: 'Unrealized P&L',
      value: `${pnl >= 0 ? '+' : ''}${fmtUsd(pnl)}`,
      change: `${positions.length} positions`,
      pct: `${pnlPct >= 0 ? '+' : ''}${pnlPct.toFixed(2)}%`,
      up: pnl >= 0,
    },
    FALLBACK[2],
    FALLBACK[3],
  ]
}

export default function PortfolioSummary() {
  const { subscribe } = useWS()
  const [stats, setStats] = useState<StatItem[]>(FALLBACK)

  // Initial fetch
  useEffect(() => {
    let cancelled = false
    Promise.allSettled([fetchBalances(), fetchPositions()])
      .then(([balRes, posRes]) => {
        if (cancelled) return
        const balances: BalanceData[] = balRes.status === 'fulfilled' ? balRes.value : []
        const positions: PositionData[] = posRes.status === 'fulfilled' ? posRes.value : []
        if (!balances.length && !positions.length) return
        setStats(buildStats(balances, positions))
      })
    return () => { cancelled = true }
  }, [])

  // Subscribe to real-time portfolio updates
  useEffect(() => {
    return subscribe('portfolio.update', (data) => {
      const balances = (data.balances as BalanceData[]) || []
      const positions = (data.positions as PositionData[]) || []
      const totalPnl = data.total_pnl as number | undefined
      if (balances.length || positions.length) {
        setStats(buildStats(balances, positions, totalPnl))
      }
    })
  }, [subscribe])

  return (
    <div className="card fade-in grid grid-cols-4">
      {stats.map((s, i) => (
        <div
          key={s.label}
          className={`px-6 py-4 ${i < stats.length - 1 ? 'border-r border-white/[0.06]' : ''}`}
        >
          <div className="flex items-center justify-between mb-2.5">
            <span className="text-[11px] text-[var(--text-3)] font-medium">{s.label}</span>
            <span className={`mono text-[10px] font-semibold px-1.5 py-0.5 rounded ${
              s.up ? 'text-[#10b981] bg-[rgba(16,185,129,0.08)]' : 'text-[#ef4444] bg-[rgba(239,68,68,0.08)]'
            }`}>
              {s.pct}
            </span>
          </div>
          <div className={`mono text-[22px] font-extrabold tracking-tight leading-none ${
            s.label.includes('P&L') ? (s.up ? 'text-[#10b981]' : 'text-[#ef4444]') : 'text-[var(--text-1)]'
          }`}>
            {s.value}
          </div>
          <div className="text-[10px] text-[var(--text-3)] mt-1.5">{s.change}</div>
        </div>
      ))}
    </div>
  )
}
```

**Step 2: Verify it compiles**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

**Step 3: Commit**

```bash
git add web/src/components/PortfolioSummary.tsx
git commit -m "feat(web): rewrite PortfolioSummary with live WebSocket updates"
```

---

### Task 9: Rewrite RecentTrades with WebSocket

**Files:**
- Modify: `web/src/components/RecentTrades.tsx`

**Step 1: Rewrite with useWS**

Replace the entire file:

```typescript
// web/src/components/RecentTrades.tsx
import { useState, useEffect } from 'react'
import { useWS } from '../hooks/WebSocketProvider'

interface Trade {
  id: number
  sym: string
  exchange: string
  side: string
  size: number
  price: number
  time: string
  live: boolean
}

const FALLBACK: Trade[] = [
  { id: 0, sym: 'BTC/USDT', exchange: 'Binance', side: 'Buy', size: 0.05, price: 68500, time: '14:32', live: false },
  { id: 1, sym: 'EUR/USD', exchange: 'MT5', side: 'Sell', size: 0.5, price: 1.0892, time: '14:15', live: false },
  { id: 2, sym: 'SOL/USDT', exchange: 'Hyperliquid', side: 'Sell', size: 15, price: 178.5, time: '13:15', live: false },
  { id: 3, sym: 'AAPL', exchange: 'IBKR', side: 'Buy', size: 25, price: 195.40, time: '12:30', live: false },
  { id: 4, sym: 'BNB/USDT', exchange: 'Binance', side: 'Sell', size: 2, price: 625.4, time: '10:22', live: false },
]

let tradeId = 100

function fmtTime(d: Date): string {
  return d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false })
}

export default function RecentTrades() {
  const { subscribe } = useWS()
  const [trades, setTrades] = useState<Trade[]>(FALLBACK)

  useEffect(() => {
    return subscribe('trade.executed', (data) => {
      const trade: Trade = {
        id: tradeId++,
        sym: (data.symbol as string) || '',
        exchange: (data.exchange as string) || '',
        side: (data.side as string) || '',
        size: (data.size as number) || 0,
        price: (data.price as number) || 0,
        time: fmtTime(new Date()),
        live: true,
      }
      setTrades(prev => [trade, ...prev].slice(0, 20))
    })
  }, [subscribe])

  return (
    <div className="card fade-in flex flex-col" style={{ animationDelay: '300ms' }}>
      <div className="flex items-center justify-between px-4 py-2.5 border-b border-white/[0.06]">
        <span className="text-xs font-bold text-[var(--text-1)]">Recent Trades</span>
        <span className="text-[10px] text-[var(--text-3)]">Today</span>
      </div>
      <div className="flex-1 overflow-auto">
        {trades.map(t => {
          const isBuy = t.side.toLowerCase() === 'buy'
          return (
            <div
              key={t.id}
              className="flex items-center justify-between px-4 py-2 border-b border-white/[0.02] hover:bg-white/[0.015] transition-colors"
            >
              <div className="flex items-center gap-2">
                <span className={`text-[9px] font-semibold px-1.5 py-0.5 rounded ${
                  isBuy ? 'text-[#10b981] bg-[rgba(16,185,129,0.08)]' : 'text-[#ef4444] bg-[rgba(239,68,68,0.08)]'
                }`}>
                  {t.side}
                </span>
                <span className="text-[11px] font-semibold text-[var(--text-1)]">{t.sym}</span>
                <span className="text-[9px] text-[var(--text-3)]">{t.exchange}</span>
                {t.live && <div className="w-1 h-1 rounded-full bg-[#10b981]" />}
              </div>
              <div className="flex items-center gap-3">
                <span className="mono text-[11px] text-[var(--text-2)]">
                  {t.size} @ ${t.price.toLocaleString()}
                </span>
                <span className="mono text-[9px] text-[var(--text-3)]">{t.time}</span>
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
```

**Step 2: Verify it compiles**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

**Step 3: Commit**

```bash
git add web/src/components/RecentTrades.tsx
git commit -m "feat(web): rewrite RecentTrades with live WebSocket trade feed"
```

---

### Task 10: Rewrite App.tsx layout

**Files:**
- Modify: `web/src/App.tsx`

**Step 1: Rewrite App with new 2-column layout**

Replace the entire file:

```typescript
// web/src/App.tsx
import { useState } from 'react'
import Header from './components/Header'
import Sidebar from './components/Sidebar'
import ChatPanel from './components/ChatPanel'
import PositionsTable from './components/PositionsTable'
import PortfolioSummary from './components/PortfolioSummary'
import TradingViewChart from './components/TradingViewChart'
import MarketOverview from './components/MarketOverview'
import ExchangeStatus from './components/ExchangeStatus'
import AgentStatus from './components/AgentStatus'
import AgentInsights from './components/AgentInsights'
import RecentTrades from './components/RecentTrades'
import TradeToast from './components/TradeToast'
import StatusBar from './components/StatusBar'
import SettingsPanel from './components/SettingsPanel'
import PerformanceChart from './components/PerformanceChart'
import RiskAnalysis from './components/RiskAnalysis'
import AssetAllocation from './components/AssetAllocation'
import MonthlyReturns from './components/MonthlyReturns'
import TopMovers from './components/TopMovers'
import DrawdownChart from './components/DrawdownChart'

export default function App() {
  const [tab, setTab] = useState('dashboard')

  return (
    <div className="flex h-screen w-screen bg-[var(--bg-0)] text-[var(--text-2)] overflow-hidden">
      <TradeToast />
      <Sidebar activeTab={tab} onTabChange={setTab} />
      <div className="flex flex-col flex-1 min-w-0">
        <Header />
        <main className="flex-1 overflow-auto p-3">
          {tab === 'dashboard' && <DashboardView />}
          {tab === 'chat' && (
            <div className="h-[calc(100vh-68px)] max-w-[900px] mx-auto">
              <ChatPanel />
            </div>
          )}
          {tab === 'positions' && (
            <div className="flex flex-col gap-3">
              <PortfolioSummary />
              <PositionsTable />
            </div>
          )}
          {tab === 'analytics' && <AnalyticsView />}
          {tab === 'strategies' && <StrategiesPlaceholder />}
          {tab === 'settings' && <SettingsPanel />}
        </main>
        <StatusBar />
      </div>
    </div>
  )
}

function DashboardView() {
  return (
    <div className="flex gap-3 h-[calc(100vh-100px)]">
      {/* Main area */}
      <div className="flex-1 flex flex-col gap-3 min-w-0">
        {/* Portfolio stats */}
        <PortfolioSummary />

        {/* TradingView chart */}
        <div className="flex-1 min-h-0">
          <TradingViewChart />
        </div>

        {/* Bottom row: Market + Trades */}
        <div className="grid grid-cols-2 gap-3" style={{ height: '220px' }}>
          <MarketOverview />
          <RecentTrades />
        </div>
      </div>

      {/* Right sidebar */}
      <div className="w-[340px] flex flex-col gap-3 shrink-0">
        {/* Chat */}
        <div className="flex-1 min-h-0 card overflow-hidden">
          <ChatPanel />
        </div>

        {/* Agent Insights */}
        <div className="h-[280px] card overflow-hidden">
          <AgentInsights />
        </div>
      </div>
    </div>
  )
}

function AnalyticsView() {
  return (
    <div
      className="grid gap-3 w-full min-h-0"
      style={{
        gridTemplateColumns: '1fr 1fr',
        gridTemplateRows: 'minmax(280px, 1fr) minmax(220px, auto) minmax(180px, auto)',
        height: 'calc(100vh - 100px)',
      }}
    >
      <PerformanceChart />
      <RiskAnalysis />
      <div className="col-span-2 grid gap-3 min-h-0" style={{ gridTemplateColumns: '1fr 1.2fr 1fr' }}>
        <AssetAllocation />
        <MonthlyReturns />
        <TopMovers />
      </div>
      <div className="col-span-2 min-h-0">
        <DrawdownChart />
      </div>
    </div>
  )
}

function StrategiesPlaceholder() {
  return (
    <div className="flex items-center justify-center" style={{ height: 'calc(100vh - 120px)' }}>
      <div className="card text-center p-12 max-w-[440px]">
        <div className="w-14 h-14 rounded-[14px] mx-auto mb-5 bg-[rgba(99,102,241,0.08)] border border-[rgba(99,102,241,0.15)] flex items-center justify-center">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#818cf8" strokeWidth="2" strokeLinecap="round">
            <path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z"/>
          </svg>
        </div>
        <h2 className="text-lg font-bold text-[var(--text-1)] mb-2">Strategy Arena</h2>
        <p className="text-[13px] text-[var(--text-3)] leading-relaxed">
          Create, backtest, and A/B test trading strategies across Crypto, Forex, Stocks and Indices.
        </p>
      </div>
    </div>
  )
}
```

**Step 2: Verify it compiles**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

**Step 3: Build the frontend**

Run: `cd web && npx vite build`
Expected: Build succeeds

**Step 4: Commit**

```bash
git add web/src/App.tsx
git commit -m "feat(web): redesign dashboard layout with right sidebar and real-time components"
```

---

### Task 11: Build and verify

**Files:**
- None (verification only)

**Step 1: Full TypeScript check**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

**Step 2: Build the frontend**

Run: `cd web && npx vite build`
Expected: Build succeeds, outputs to `web/dist/`

**Step 3: Run Go tests to make sure nothing broke**

Run: `go test ./... -count=1`
Expected: All PASS

**Step 4: Commit build output**

```bash
git add web/dist/
git commit -m "chore(web): rebuild dashboard frontend"
```
