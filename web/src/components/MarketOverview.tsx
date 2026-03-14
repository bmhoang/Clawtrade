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
