import { useEffect, useState } from 'react'
import { fetchHealth } from '../api/client'

export default function Header() {
  const [status, setStatus] = useState<string>('connecting...')
  const [version, setVersion] = useState<string>('')

  useEffect(() => {
    fetchHealth()
      .then((data) => {
        setStatus(data.status)
        setVersion(data.version)
      })
      .catch(() => setStatus('offline'))
  }, [])

  return (
    <header className="flex items-center justify-between px-6 py-3 bg-slate-800 border-b border-slate-700">
      <h1 className="text-lg font-bold text-white">Clawtrade</h1>
      <div className="flex items-center gap-4 text-sm">
        <span className={`px-2 py-1 rounded ${status === 'ok' ? 'bg-green-900 text-green-300' : 'bg-red-900 text-red-300'}`}>
          {status === 'ok' ? 'Connected' : status}
        </span>
        {version && <span className="text-slate-400">v{version}</span>}
      </div>
    </header>
  )
}
