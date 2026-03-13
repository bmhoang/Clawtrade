const mockPositions = [
  { symbol: 'BTC-USDT', side: 'Long', size: '0.05', entry: '68,500', current: '70,200', pnl: '+$85.00', pnlPct: '+2.48%' },
  { symbol: 'ETH-USDT', side: 'Long', size: '1.2', entry: '3,450', current: '3,380', pnl: '-$84.00', pnlPct: '-2.03%' },
]

export default function PositionsTable() {
  return (
    <div className="bg-slate-800 rounded-lg border border-slate-700 flex-1">
      <div className="px-4 py-3 border-b border-slate-700 text-sm font-medium">Open Positions</div>
      <div className="overflow-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-slate-400 text-xs border-b border-slate-700">
              <th className="text-left px-4 py-2">Symbol</th>
              <th className="text-left px-4 py-2">Side</th>
              <th className="text-right px-4 py-2">Size</th>
              <th className="text-right px-4 py-2">Entry</th>
              <th className="text-right px-4 py-2">Current</th>
              <th className="text-right px-4 py-2">PnL</th>
            </tr>
          </thead>
          <tbody>
            {mockPositions.map((pos) => (
              <tr key={pos.symbol} className="border-b border-slate-700/50 hover:bg-slate-700/30">
                <td className="px-4 py-2 font-medium text-white">{pos.symbol}</td>
                <td className="px-4 py-2">
                  <span className={pos.side === 'Long' ? 'text-green-400' : 'text-red-400'}>{pos.side}</span>
                </td>
                <td className="px-4 py-2 text-right">{pos.size}</td>
                <td className="px-4 py-2 text-right">${pos.entry}</td>
                <td className="px-4 py-2 text-right">${pos.current}</td>
                <td className={`px-4 py-2 text-right ${pos.pnl.startsWith('+') ? 'text-green-400' : 'text-red-400'}`}>
                  {pos.pnl} ({pos.pnlPct})
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
