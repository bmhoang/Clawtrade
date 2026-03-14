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
