const stats = [
  { label: 'Balance', value: '$10,000.00' },
  { label: 'Unrealized PnL', value: '+$1.00', color: 'text-green-400' },
  { label: 'Today PnL', value: '-$45.00', color: 'text-red-400' },
  { label: 'Open Positions', value: '2' },
]

export default function PortfolioSummary() {
  return (
    <div className="bg-slate-800 rounded-lg border border-slate-700">
      <div className="px-4 py-3 border-b border-slate-700 text-sm font-medium">Portfolio</div>
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 p-4">
        {stats.map((stat) => (
          <div key={stat.label}>
            <div className="text-xs text-slate-400 mb-1">{stat.label}</div>
            <div className={`text-lg font-semibold ${stat.color || 'text-white'}`}>{stat.value}</div>
          </div>
        ))}
      </div>
    </div>
  )
}
