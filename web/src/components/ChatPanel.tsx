import { useState, useRef, useEffect } from 'react'

interface ToolCall {
  id: string
  name: string
  input: Record<string, unknown>
}

interface Message {
  role: 'user' | 'assistant'
  content: string
  time: string
  toolsUsed?: ToolCall[]
}

interface ChatApiResponse {
  content: string
  model: string
  tools_used?: ToolCall[]
  usage?: { input_tokens: number; output_tokens: number }
  error?: string
}

function now() {
  return new Date().toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' })
}

const API_BASE = window.location.port === '5173'
  ? 'http://127.0.0.1:8899'
  : window.location.origin

const TOOL_LABELS: Record<string, string> = {
  get_price: 'Fetching price',
  get_candles: 'Loading candles',
  analyze_market: 'Analyzing market',
  get_balances: 'Checking balances',
  get_positions: 'Loading positions',
  risk_check: 'Running risk check',
  calculate_position_size: 'Calculating size',
  place_order: 'Placing order',
  cancel_order: 'Cancelling order',
  get_open_orders: 'Loading orders',
  backtest: 'Running backtest',
}

function ToolBadges({ tools }: { tools: ToolCall[] }) {
  if (!tools.length) return null
  return (
    <div className="flex flex-wrap gap-1 mt-2 mb-1">
      {tools.map((t, i) => (
        <span key={i} className="text-[9px] font-semibold px-2 py-0.5 rounded-md bg-indigo-500/10 text-indigo-400 border border-indigo-500/10">
          {TOOL_LABELS[t.name] || t.name}
          {t.input?.symbol ? ` · ${t.input.symbol}` : ''}
        </span>
      ))}
    </div>
  )
}

export default function ChatPanel() {
  const [messages, setMessages] = useState<Message[]>([
    { role: 'assistant', content: 'Hey! I\'m your AI trading agent. I can fetch real prices, analyze markets, check your portfolio, and execute trades. Try "How is BTC doing?" or "Show my positions".', time: now() },
  ])
  const [input, setInput] = useState('')
  const [typing, setTyping] = useState(false)
  const [typingStatus, setTypingStatus] = useState('Thinking...')
  const [modelLabel, setModelLabel] = useState('AI')
  const [tokenCount, setTokenCount] = useState(0)
  const endRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, typing])

  const send = async () => {
    if (!input.trim() || typing) return
    const userMsg: Message = { role: 'user', content: input, time: now() }
    setMessages(p => [...p, userMsg])
    setInput('')
    setTyping(true)
    setTypingStatus('Thinking...')

    // Show "Using tools..." after a delay (agent is likely calling tools)
    const toolTimer = setTimeout(() => setTypingStatus('Using tools...'), 2000)

    try {
      const chatHistory = [...messages.slice(1), userMsg]
        .map(m => ({ role: m.role, content: m.content }))

      const res = await fetch(`${API_BASE}/api/v1/chat`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ messages: chatHistory }),
      })

      const data: ChatApiResponse = await res.json()
      clearTimeout(toolTimer)

      if (!res.ok || data.error) {
        throw new Error(data.error || `HTTP ${res.status}`)
      }

      if (data.model) {
        const short = data.model.length > 20
          ? data.model.split(/[/-]/).slice(0, 2).join('-')
          : data.model
        setModelLabel(short)
      }
      if (data.usage) {
        setTokenCount(prev => prev + data.usage!.input_tokens + data.usage!.output_tokens)
      }

      setTyping(false)
      setMessages(p => [...p, {
        role: 'assistant',
        content: data.content,
        time: now(),
        toolsUsed: data.tools_used,
      }])
    } catch (err: any) {
      clearTimeout(toolTimer)
      setTyping(false)
      const errorMsg = err.message || 'Failed to connect'
      setMessages(p => [...p, {
        role: 'assistant',
        content: `Error: ${errorMsg}\n\nMake sure the server is running (\`clawtrade serve\`) and an LLM model is configured (\`clawtrade models setup\`).`,
        time: now(),
      }])
    }
  }

  return (
    <div className="flex flex-col h-full glass overflow-hidden">
      {/* Header */}
      <div className="flex items-center gap-3 px-5 py-3 border-b border-white/[0.04]">
        <div className="w-8 h-8 rounded-xl bg-gradient-to-br from-[#4f8fff] to-[#00e5ff] p-[1px]">
          <div className="w-full h-full rounded-[11px] bg-[#0b1120] flex items-center justify-center">
            <span className="text-[11px]">🤖</span>
          </div>
        </div>
        <div className="flex-1">
          <div className="text-[13px] font-semibold text-white">Clawtrade AI Agent</div>
          <div className="flex items-center gap-1.5">
            <div className="w-1.5 h-1.5 rounded-full bg-[#00dc82] pulse-glow" style={{ color: '#00dc82' }} />
            <span className="text-[10px] text-slate-500">{modelLabel} · Tool Use Enabled</span>
          </div>
        </div>
        <div className="text-[9px] text-slate-600 bg-white/[0.03] px-2 py-1 rounded-md">
          {tokenCount > 0 ? `${tokenCount.toLocaleString()} tokens` : '0 tokens'}
        </div>
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto px-4 py-4 space-y-4">
        {messages.map((m, i) => (
          <div key={i} className={`flex ${m.role === 'user' ? 'justify-end' : 'justify-start'} slide-in`}>
            <div className="max-w-[88%]">
              {m.role === 'assistant' && m.toolsUsed && m.toolsUsed.length > 0 && (
                <ToolBadges tools={m.toolsUsed} />
              )}
              <div className={`px-4 py-3 text-[13px] leading-relaxed ${
                m.role === 'user'
                  ? 'bg-gradient-to-r from-[#4f8fff] to-[#4f8fff]/80 text-white rounded-2xl rounded-br-lg'
                  : 'bg-white/[0.04] border border-white/[0.06] text-slate-200 rounded-2xl rounded-bl-lg'
              }`} style={{ whiteSpace: 'pre-wrap' }}>
                {m.content}
              </div>
              <div className={`text-[9px] text-slate-600 mt-1.5 px-2 ${m.role === 'user' ? 'text-right' : ''}`}>
                {m.time}
              </div>
            </div>
          </div>
        ))}
        {typing && (
          <div className="flex justify-start slide-in">
            <div className="bg-white/[0.04] border border-white/[0.06] rounded-2xl rounded-bl-lg px-4 py-3">
              <div className="flex items-center gap-2">
                <div className="flex gap-1.5">
                  {[0, 1, 2].map(i => (
                    <div key={i} className="w-1.5 h-1.5 rounded-full bg-[#4f8fff]/60 animate-bounce" style={{ animationDelay: `${i * 150}ms` }} />
                  ))}
                </div>
                <span className="text-[10px] text-slate-500">{typingStatus}</span>
              </div>
            </div>
          </div>
        )}
        <div ref={endRef} />
      </div>

      {/* Input */}
      <div className="p-3 border-t border-white/[0.04]">
        <div className="flex gap-2 items-center bg-white/[0.03] rounded-xl border border-white/[0.05] px-3 py-1 focus-within:border-[#4f8fff]/30 transition-colors">
          <input
            type="text"
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && send()}
            placeholder="Ask about markets, analyze BTC, check portfolio..."
            className="flex-1 py-2 bg-transparent text-[13px] text-white placeholder-slate-600 outline-none"
          />
          <button
            onClick={send}
            disabled={!input.trim() || typing}
            className="w-8 h-8 rounded-lg bg-[#4f8fff] hover:bg-[#4f8fff]/80 disabled:opacity-20 flex items-center justify-center transition-all shrink-0"
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth="2.5" strokeLinecap="round">
              <path d="M5 12h14M12 5l7 7-7 7"/>
            </svg>
          </button>
        </div>
      </div>
    </div>
  )
}
