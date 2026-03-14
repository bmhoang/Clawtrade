import { useState, useRef, useEffect } from 'react'
import ReactMarkdown from 'react-markdown'

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
  ? 'http://127.0.0.1:9090'
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
  create_alert: 'Creating alert',
  list_alerts: 'Listing alerts',
  delete_alert: 'Deleting alert',
}

const TOOL_ICONS: Record<string, string> = {
  get_price: '📊',
  get_candles: '🕯️',
  analyze_market: '🔍',
  get_balances: '💰',
  get_positions: '📋',
  risk_check: '🛡️',
  calculate_position_size: '📐',
  place_order: '⚡',
  cancel_order: '❌',
  get_open_orders: '📑',
  backtest: '📈',
  create_alert: '🔔',
  list_alerts: '📋',
  delete_alert: '🗑️',
}

const SUGGESTIONS = [
  { icon: '📊', label: 'Market Analysis', prompt: 'How is BTC doing right now?' },
  { icon: '💼', label: 'Portfolio', prompt: 'Show my current positions and P&L' },
  { icon: '📈', label: 'Backtest', prompt: 'Backtest RSI strategy on BTC/USDT 4h' },
  { icon: '🔔', label: 'Create Alert', prompt: 'Alert me when ETH drops below $3000' },
  { icon: '⚖️', label: 'Risk Check', prompt: 'Run a risk check on my portfolio' },
  { icon: '🔍', label: 'Deep Analysis', prompt: 'Analyze SOL/USDT with SMC strategy on 1h timeframe' },
]

const COMPACT_SUGGESTIONS = [
  { icon: '📊', label: 'BTC price?', prompt: 'How is BTC doing right now?' },
  { icon: '💼', label: 'Positions', prompt: 'Show my current positions and P&L' },
  { icon: '🔔', label: 'Set alert', prompt: 'Alert me when ETH drops below $3000' },
]

function ToolCallCard({ tools }: { tools: ToolCall[] }) {
  const [expanded, setExpanded] = useState(false)
  if (!tools.length) return null

  return (
    <div className="mb-3">
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex items-center gap-2 text-[11px] text-indigo-400 hover:text-indigo-300 transition-colors"
      >
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
          <path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/>
        </svg>
        <span className="font-medium">Used {tools.length} tool{tools.length > 1 ? 's' : ''}</span>
        <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5"
          className={`transition-transform ${expanded ? 'rotate-180' : ''}`}>
          <path d="M6 9l6 6 6-6"/>
        </svg>
      </button>
      {expanded && (
        <div className="mt-2 space-y-1.5">
          {tools.map((t, i) => (
            <div key={i} className="flex items-center gap-2 px-3 py-2 rounded-lg bg-white/[0.03] border border-white/[0.05]">
              <span className="text-xs">{TOOL_ICONS[t.name] || '🔧'}</span>
              <span className="text-[11px] font-medium text-slate-300">
                {TOOL_LABELS[t.name] || t.name}
              </span>
              {t.input?.symbol ? (
                <span className="text-[10px] text-slate-500 ml-auto font-mono">{String(t.input.symbol)}</span>
              ) : null}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function ClawIcon({ size = 18, color = 'white' }: { size?: number; color?: string }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none">
      {/* Claw pincers gripping coin */}
      <path d="M7.5 3C5.5 5.5 5.5 8.5 7 11" stroke={color} strokeWidth="2" strokeLinecap="round"/>
      <path d="M16.5 3C18.5 5.5 18.5 8.5 17 11" stroke={color} strokeWidth="2" strokeLinecap="round"/>
      {/* Coin */}
      <circle cx="12" cy="8" r="3.2" stroke={color} strokeWidth="1.5"/>
      <path d="M12 6v4" stroke={color} strokeWidth="1" strokeLinecap="round"/>
      <path d="M10.5 7.2c0-.5.7-.9 1.5-.9s1.5.4 1.5.9-.7.8-1.5.8-1.5.4-1.5.9.7.9 1.5.9 1.5-.4 1.5-.9" stroke={color} strokeWidth=".8" strokeLinecap="round" fill="none"/>
      {/* Shrimp body */}
      <path d="M7 11c0 3.5 2.2 6.5 5 9 2.8-2.5 5-5.5 5-9" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/>
      {/* Body segments */}
      <path d="M9 14.5h6" stroke={color} strokeWidth="1" strokeLinecap="round" opacity="0.5"/>
      <path d="M10 17h4" stroke={color} strokeWidth="1" strokeLinecap="round" opacity="0.5"/>
    </svg>
  )
}

function WelcomeScreen({ onSuggestionClick }: { onSuggestionClick: (prompt: string) => void }) {
  return (
    <div className="flex flex-col items-center justify-center h-full px-6">
      {/* Logo */}
      <div className="w-16 h-16 rounded-2xl bg-gradient-to-br from-indigo-500 to-cyan-400 p-[2px] mb-6">
        <div className="w-full h-full rounded-[14px] bg-[var(--bg-0)] flex items-center justify-center">
          <ClawIcon size={28} color="#818cf8" />
        </div>
      </div>

      <h2 className="text-xl font-semibold text-white mb-2">Clawtrade AI Agent</h2>
      <p className="text-sm text-slate-400 text-center max-w-md mb-8 leading-relaxed">
        Your AI trading assistant with real-time market data, portfolio analysis, backtesting, and trade execution across 7 exchanges.
      </p>

      {/* Suggestion grid */}
      <div className="grid grid-cols-2 gap-2.5 w-full max-w-lg">
        {SUGGESTIONS.map((s, i) => (
          <button
            key={i}
            onClick={() => onSuggestionClick(s.prompt)}
            className="flex items-start gap-3 p-3.5 rounded-xl bg-white/[0.03] border border-white/[0.06] hover:bg-white/[0.06] hover:border-white/[0.1] transition-all text-left group"
          >
            <span className="text-lg mt-0.5 group-hover:scale-110 transition-transform">{s.icon}</span>
            <div>
              <div className="text-[12px] font-semibold text-white mb-0.5">{s.label}</div>
              <div className="text-[11px] text-slate-500 leading-snug">{s.prompt}</div>
            </div>
          </button>
        ))}
      </div>
    </div>
  )
}

function CompactWelcome({ onSuggestionClick }: { onSuggestionClick: (prompt: string) => void }) {
  return (
    <div className="flex flex-col items-center justify-center h-full px-4">
      <div className="w-10 h-10 rounded-xl bg-gradient-to-br from-indigo-500 to-cyan-400 p-[1.5px] mb-3">
        <div className="w-full h-full rounded-[9px] bg-[var(--bg-1)] flex items-center justify-center">
          <ClawIcon size={16} color="#818cf8" />
        </div>
      </div>
      <p className="text-[12px] text-slate-400 text-center mb-4 leading-relaxed">
        Ask me about markets, positions, or set up alerts.
      </p>
      <div className="flex flex-wrap gap-1.5 justify-center">
        {COMPACT_SUGGESTIONS.map((s, i) => (
          <button
            key={i}
            onClick={() => onSuggestionClick(s.prompt)}
            className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg bg-white/[0.04] border border-white/[0.06] hover:bg-white/[0.08] hover:border-white/[0.1] transition-all text-[11px] text-slate-300"
          >
            <span>{s.icon}</span>
            {s.label}
          </button>
        ))}
      </div>
    </div>
  )
}

function AssistantAvatar() {
  return (
    <div className="w-7 h-7 rounded-lg bg-gradient-to-br from-indigo-500 to-cyan-400 p-[1.5px] shrink-0 mt-0.5">
      <div className="w-full h-full rounded-[6px] bg-[var(--bg-1)] flex items-center justify-center">
        <ClawIcon size={12} color="#818cf8" />
      </div>
    </div>
  )
}

function UserAvatar() {
  return (
    <div className="w-7 h-7 rounded-lg bg-gradient-to-br from-violet-600 to-fuchsia-500 shrink-0 mt-0.5 flex items-center justify-center">
      <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth="2" strokeLinecap="round">
        <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/>
        <circle cx="12" cy="7" r="4"/>
      </svg>
    </div>
  )
}

function TypingIndicator({ status }: { status: string }) {
  return (
    <div className="flex gap-3 px-4 py-4 slide-in">
      <AssistantAvatar />
      <div className="flex items-center gap-3 px-4 py-3 rounded-2xl bg-white/[0.03] border border-white/[0.05]">
        <div className="flex gap-1">
          {[0, 1, 2].map(i => (
            <div
              key={i}
              className="w-2 h-2 rounded-full bg-indigo-400/60"
              style={{
                animation: 'pulse-dot 1.4s ease-in-out infinite',
                animationDelay: `${i * 200}ms`,
              }}
            />
          ))}
        </div>
        <span className="text-[11px] text-slate-500">{status}</span>
      </div>
    </div>
  )
}

export default function ChatPanel({ compact = false }: { compact?: boolean }) {
  const [messages, setMessages] = useState<Message[]>([])
  const [input, setInput] = useState('')
  const [typing, setTyping] = useState(false)
  const [typingStatus, setTypingStatus] = useState('Thinking...')
  const [modelLabel, setModelLabel] = useState('AI')
  const [tokenCount, setTokenCount] = useState(0)
  const endRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, typing])

  // Auto-resize textarea
  useEffect(() => {
    if (inputRef.current) {
      inputRef.current.style.height = 'auto'
      inputRef.current.style.height = Math.min(inputRef.current.scrollHeight, 160) + 'px'
    }
  }, [input])

  const send = async (overrideInput?: string) => {
    const text = overrideInput ?? input
    if (!text.trim() || typing) return
    const userMsg: Message = { role: 'user', content: text, time: now() }
    setMessages(p => [...p, userMsg])
    setInput('')
    setTyping(true)
    setTypingStatus('Thinking...')

    const toolTimer = setTimeout(() => setTypingStatus('Using tools...'), 2000)

    try {
      const chatHistory = [...messages, userMsg]
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
        content: `**Connection Error**\n\n${errorMsg}\n\nMake sure the server is running (\`clawtrade serve\`) and an LLM model is configured (\`clawtrade models setup\`).`,
        time: now(),
      }])
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      send()
    }
  }

  const isEmpty = messages.length === 0

  return (
    <div className="flex flex-col h-full overflow-hidden">
      <style>{`
        @keyframes pulse-dot {
          0%, 80%, 100% { opacity: 0.3; transform: scale(0.8); }
          40% { opacity: 1; transform: scale(1); }
        }
        .chat-markdown p { margin-bottom: 0.75em; }
        .chat-markdown p:last-child { margin-bottom: 0; }
        .chat-markdown strong { color: #e2e8f0; font-weight: 600; }
        .chat-markdown code {
          font-family: 'JetBrains Mono', monospace;
          font-size: 0.85em;
          background: rgba(255,255,255,0.06);
          padding: 0.15em 0.4em;
          border-radius: 4px;
          color: #c4b5fd;
        }
        .chat-markdown pre {
          background: rgba(0,0,0,0.3);
          border: 1px solid rgba(255,255,255,0.06);
          border-radius: 8px;
          padding: 12px 16px;
          overflow-x: auto;
          margin: 0.75em 0;
        }
        .chat-markdown pre code {
          background: none;
          padding: 0;
          color: #d4d4d8;
          font-size: 0.82em;
        }
        .chat-markdown ul, .chat-markdown ol {
          margin: 0.5em 0;
          padding-left: 1.5em;
        }
        .chat-markdown li { margin-bottom: 0.3em; }
        .chat-markdown h1, .chat-markdown h2, .chat-markdown h3 {
          color: #f1f5f9;
          font-weight: 600;
          margin: 1em 0 0.5em;
        }
        .chat-markdown h1 { font-size: 1.2em; }
        .chat-markdown h2 { font-size: 1.1em; }
        .chat-markdown h3 { font-size: 1em; }
        .chat-markdown table {
          border-collapse: collapse;
          width: 100%;
          margin: 0.75em 0;
          font-size: 0.9em;
        }
        .chat-markdown th, .chat-markdown td {
          border: 1px solid rgba(255,255,255,0.08);
          padding: 6px 10px;
          text-align: left;
        }
        .chat-markdown th {
          background: rgba(255,255,255,0.04);
          color: #e2e8f0;
          font-weight: 600;
        }
        .chat-markdown blockquote {
          border-left: 3px solid #6366f1;
          padding-left: 12px;
          margin: 0.75em 0;
          color: #a1a1aa;
        }
      `}</style>

      {/* Header */}
      <div className={`flex items-center justify-between ${compact ? 'px-3 py-2' : 'px-5 py-3'} border-b border-white/[0.06] bg-white/[0.01]`}>
        <div className="flex items-center gap-2">
          {!compact && <AssistantAvatar />}
          <div>
            <div className={`${compact ? 'text-[11px]' : 'text-[13px]'} font-semibold text-white`}>Clawtrade AI</div>
            <div className="flex items-center gap-1.5">
              <div className="w-1.5 h-1.5 rounded-full bg-emerald-400 shadow-[0_0_6px_rgba(52,211,153,0.5)]" />
              <span className="text-[10px] text-slate-500">{modelLabel} · 14 tools</span>
            </div>
          </div>
        </div>
        {tokenCount > 0 && (
          <div className="text-[9px] text-slate-500 bg-white/[0.03] px-2 py-0.5 rounded-md border border-white/[0.05]">
            {tokenCount.toLocaleString()}
          </div>
        )}
      </div>

      {/* Messages area */}
      <div className="flex-1 overflow-y-auto">
        {isEmpty ? (
          compact
            ? <CompactWelcome onSuggestionClick={(prompt) => send(prompt)} />
            : <WelcomeScreen onSuggestionClick={(prompt) => send(prompt)} />
        ) : (
          <div className={`${compact ? '' : 'max-w-3xl mx-auto'} py-2`}>
            {messages.map((m, i) => (
              compact ? (
                /* Compact message style — bubble layout, no avatars */
                <div key={i} className={`flex ${m.role === 'user' ? 'justify-end' : 'justify-start'} px-3 py-1.5 slide-in`}>
                  <div className={`max-w-[90%] ${m.role === 'user' ? '' : ''}`}>
                    {m.role === 'assistant' && m.toolsUsed && m.toolsUsed.length > 0 && (
                      <div className="flex flex-wrap gap-1 mb-1">
                        {m.toolsUsed.map((t, j) => (
                          <span key={j} className="text-[9px] font-medium px-1.5 py-0.5 rounded bg-indigo-500/10 text-indigo-400 border border-indigo-500/10">
                            {TOOL_LABELS[t.name] || t.name}
                          </span>
                        ))}
                      </div>
                    )}
                    <div className={`px-3 py-2 text-[12px] leading-relaxed ${
                      m.role === 'user'
                        ? 'bg-indigo-500/20 text-white rounded-2xl rounded-br-md'
                        : 'bg-white/[0.04] border border-white/[0.06] text-slate-200 rounded-2xl rounded-bl-md'
                    }`}>
                      {m.role === 'assistant' ? (
                        <div className="chat-markdown">
                          <ReactMarkdown>{m.content}</ReactMarkdown>
                        </div>
                      ) : (
                        <span style={{ whiteSpace: 'pre-wrap' }}>{m.content}</span>
                      )}
                    </div>
                    <div className={`text-[9px] text-slate-600 mt-0.5 px-1 ${m.role === 'user' ? 'text-right' : ''}`}>
                      {m.time}
                    </div>
                  </div>
                </div>
              ) : (
                /* Full message style — avatars + names */
                <div
                  key={i}
                  className={`flex gap-3 px-4 py-4 slide-in ${
                    m.role === 'user' ? 'bg-transparent' : 'bg-white/[0.015]'
                  }`}
                >
                  {m.role === 'assistant' ? <AssistantAvatar /> : <UserAvatar />}
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 mb-1.5">
                      <span className="text-[11px] font-semibold text-slate-300">
                        {m.role === 'assistant' ? 'Clawtrade AI' : 'You'}
                      </span>
                      <span className="text-[10px] text-slate-600">{m.time}</span>
                    </div>
                    {m.role === 'assistant' && m.toolsUsed && m.toolsUsed.length > 0 && (
                      <ToolCallCard tools={m.toolsUsed} />
                    )}
                    {m.role === 'assistant' ? (
                      <div className="chat-markdown text-[13px] leading-relaxed text-slate-200">
                        <ReactMarkdown>{m.content}</ReactMarkdown>
                      </div>
                    ) : (
                      <div className="text-[13px] leading-relaxed text-slate-200" style={{ whiteSpace: 'pre-wrap' }}>
                        {m.content}
                      </div>
                    )}
                  </div>
                </div>
              )
            ))}
            {typing && (
              compact ? (
                <div className="flex justify-start px-3 py-1.5 slide-in">
                  <div className="flex items-center gap-2 px-3 py-2 rounded-2xl rounded-bl-md bg-white/[0.04] border border-white/[0.06]">
                    <div className="flex gap-1">
                      {[0, 1, 2].map(i => (
                        <div key={i} className="w-1.5 h-1.5 rounded-full bg-indigo-400/60"
                          style={{ animation: 'pulse-dot 1.4s ease-in-out infinite', animationDelay: `${i * 200}ms` }} />
                      ))}
                    </div>
                    <span className="text-[10px] text-slate-500">{typingStatus}</span>
                  </div>
                </div>
              ) : (
                <TypingIndicator status={typingStatus} />
              )
            )}
            <div ref={endRef} />
          </div>
        )}
      </div>

      {/* Input area */}
      <div className={`${compact ? 'p-2' : 'p-4'} border-t border-white/[0.06]`}>
        <div className={compact ? '' : 'max-w-3xl mx-auto'}>
          <div className={`relative flex items-end bg-white/[0.04] ${compact ? 'rounded-xl' : 'rounded-2xl'} border border-white/[0.08] focus-within:border-indigo-500/40 focus-within:shadow-[0_0_0_1px_rgba(99,102,241,0.15)] transition-all`}>
            {compact ? (
              <input
                type="text"
                value={input}
                onChange={e => setInput(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && send()}
                placeholder="Ask anything..."
                className="flex-1 py-2 px-3 bg-transparent text-[12px] text-white placeholder-slate-600 outline-none"
              />
            ) : (
              <textarea
                ref={inputRef}
                value={input}
                onChange={e => setInput(e.target.value)}
                onKeyDown={handleKeyDown}
                placeholder="Ask about markets, analyze positions, run backtests..."
                rows={1}
                className="flex-1 py-3.5 px-4 bg-transparent text-[13px] text-white placeholder-slate-600 outline-none resize-none leading-relaxed"
                style={{ maxHeight: '160px' }}
              />
            )}
            <div className={`flex items-center pr-2 ${compact ? 'pb-1.5' : 'gap-1.5 pr-3 pb-3'}`}>
              {!compact && input.trim() && (
                <span className="text-[9px] text-slate-600 mr-1">Shift+Enter for newline</span>
              )}
              <button
                onClick={() => send()}
                disabled={!input.trim() || typing}
                className={`${compact ? 'w-6 h-6' : 'w-8 h-8'} rounded-lg bg-indigo-500 hover:bg-indigo-400 disabled:bg-white/[0.05] disabled:opacity-40 flex items-center justify-center transition-all`}
              >
                <svg width={compact ? 11 : 14} height={compact ? 11 : 14} viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth="2.5" strokeLinecap="round">
                  <path d="M5 12h14M12 5l7 7-7 7"/>
                </svg>
              </button>
            </div>
          </div>
          {!compact && (
            <p className="text-[9px] text-slate-600 text-center mt-2">
              Clawtrade AI can make mistakes. Always verify before executing trades.
            </p>
          )}
        </div>
      </div>
    </div>
  )
}
