import { useState, useEffect, useCallback, useRef } from 'react'

export interface Command {
  id: string;
  label: string;
  description?: string;
  category: 'navigation' | 'trading' | 'settings' | 'ai' | 'search';
  icon?: string;
  shortcut?: string;
  action: () => void;
}

interface CommandPaletteProps {
  commands: Command[];
  onClose: () => void;
  isOpen: boolean;
}

export default function CommandPalette({ commands, onClose, isOpen }: CommandPaletteProps) {
  const [query, setQuery] = useState('');
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

  // Filter commands based on query
  const filtered = commands.filter(cmd => {
    const q = query.toLowerCase();
    return cmd.label.toLowerCase().includes(q)
      || (cmd.description?.toLowerCase().includes(q))
      || cmd.category.toLowerCase().includes(q);
  });

  // Group by category
  const grouped = filtered.reduce<Record<string, Command[]>>((acc, cmd) => {
    if (!acc[cmd.category]) acc[cmd.category] = [];
    acc[cmd.category].push(cmd);
    return acc;
  }, {});

  // Keyboard navigation
  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setSelectedIndex(prev => Math.min(prev + 1, filtered.length - 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setSelectedIndex(prev => Math.max(prev - 1, 0));
    } else if (e.key === 'Enter' && filtered[selectedIndex]) {
      filtered[selectedIndex].action();
      onClose();
    } else if (e.key === 'Escape') {
      onClose();
    }
  }, [filtered, selectedIndex, onClose]);

  // Reset on open
  useEffect(() => {
    if (isOpen) {
      setQuery('');
      setSelectedIndex(0);
      setTimeout(() => inputRef.current?.focus(), 50);
    }
  }, [isOpen]);

  // Reset selection when query changes
  useEffect(() => {
    setSelectedIndex(0);
  }, [query]);

  if (!isOpen) return null;

  let flatIndex = 0;

  return (
    // Backdrop
    <div className="fixed inset-0 z-50 flex items-start justify-center pt-[20vh]" onClick={onClose}>
      {/* Overlay */}
      <div className="fixed inset-0 bg-black/60" />
      {/* Palette */}
      <div
        className="relative w-full max-w-lg bg-slate-800 rounded-xl border border-slate-600 shadow-2xl overflow-hidden"
        onClick={e => e.stopPropagation()}
        onKeyDown={handleKeyDown}
      >
        {/* Search input */}
        <div className="flex items-center px-4 py-3 border-b border-slate-700">
          <span className="text-slate-400 mr-3 text-lg">{'\u2318'}</span>
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={e => setQuery(e.target.value)}
            placeholder="Type a command or search..."
            className="flex-1 bg-transparent text-white text-sm outline-none placeholder-slate-500"
            autoFocus
          />
          <kbd className="text-xs text-slate-500 bg-slate-700 px-2 py-0.5 rounded">ESC</kbd>
        </div>
        {/* Results */}
        <div className="max-h-80 overflow-auto py-2">
          {Object.entries(grouped).map(([category, cmds]) => (
            <div key={category}>
              <div className="px-4 py-1.5 text-xs text-slate-500 uppercase tracking-wider">{category}</div>
              {cmds.map(cmd => {
                const idx = flatIndex++;
                return (
                  <button
                    key={cmd.id}
                    onClick={() => { cmd.action(); onClose(); }}
                    className={`w-full flex items-center justify-between px-4 py-2 text-sm ${
                      idx === selectedIndex ? 'bg-blue-600/30 text-white' : 'text-slate-300 hover:bg-slate-700/50'
                    }`}
                  >
                    <div className="flex items-center gap-3">
                      {cmd.icon && <span>{cmd.icon}</span>}
                      <div className="text-left">
                        <div>{cmd.label}</div>
                        {cmd.description && <div className="text-xs text-slate-500">{cmd.description}</div>}
                      </div>
                    </div>
                    {cmd.shortcut && <kbd className="text-xs text-slate-500 bg-slate-700 px-2 py-0.5 rounded">{cmd.shortcut}</kbd>}
                  </button>
                );
              })}
            </div>
          ))}
          {filtered.length === 0 && (
            <div className="px-4 py-8 text-center text-slate-500 text-sm">No commands found</div>
          )}
        </div>
        {/* Footer */}
        <div className="flex items-center gap-4 px-4 py-2 border-t border-slate-700 text-xs text-slate-500">
          <span>{'\u2191\u2193'} Navigate</span>
          <span>{'\u21B5'} Select</span>
          <span>ESC Close</span>
        </div>
      </div>
    </div>
  );
}

// Hook to manage command palette state with Ctrl+K
export function useCommandPalette() {
  const [isOpen, setIsOpen] = useState(false);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault();
        setIsOpen(prev => !prev);
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, []);

  return { isOpen, open: () => setIsOpen(true), close: () => setIsOpen(false) };
}

// Default trading commands
export function getDefaultCommands(handlers: {
  onNavigate: (tab: string) => void;
  onTrade: (action: string) => void;
}): Command[] {
  return [
    { id: 'nav-dashboard', label: 'Go to Dashboard', category: 'navigation', icon: '\uD83D\uDCCA', shortcut: 'G D', action: () => handlers.onNavigate('dashboard') },
    { id: 'nav-chat', label: 'Go to AI Chat', category: 'navigation', icon: '\uD83D\uDCAC', shortcut: 'G C', action: () => handlers.onNavigate('chat') },
    { id: 'trade-buy', label: 'Buy / Long', description: 'Place a buy order', category: 'trading', icon: '\uD83D\uDFE2', action: () => handlers.onTrade('buy') },
    { id: 'trade-sell', label: 'Sell / Short', description: 'Place a sell order', category: 'trading', icon: '\uD83D\uDD34', action: () => handlers.onTrade('sell') },
    { id: 'trade-close', label: 'Close All Positions', description: 'Close all open positions', category: 'trading', icon: '\u23F9', action: () => handlers.onTrade('close-all') },
    { id: 'ai-analyze', label: 'AI: Analyze Market', description: 'Get AI analysis of current market', category: 'ai', icon: '\uD83E\uDD16', action: () => handlers.onTrade('analyze') },
    { id: 'ai-suggest', label: 'AI: Suggest Trade', description: 'Get AI trade suggestion', category: 'ai', icon: '\uD83D\uDCA1', action: () => handlers.onTrade('suggest') },
    { id: 'settings-theme', label: 'Toggle Theme', category: 'settings', icon: '\uD83C\uDFA8', action: () => {} },
    { id: 'settings-notifications', label: 'Notification Settings', category: 'settings', icon: '\uD83D\uDD14', action: () => {} },
  ];
}
