import { useState } from 'react'
import Header from './components/Header'
import Sidebar from './components/Sidebar'
import ChatPanel from './components/ChatPanel'
import PositionsTable from './components/PositionsTable'
import PortfolioSummary from './components/PortfolioSummary'

export default function App() {
  const [activeTab, setActiveTab] = useState('dashboard')

  return (
    <div className="flex h-screen bg-slate-900 text-slate-200">
      <Sidebar activeTab={activeTab} onTabChange={setActiveTab} />
      <div className="flex flex-col flex-1 overflow-hidden">
        <Header />
        <main className="flex-1 overflow-auto p-4">
          {activeTab === 'dashboard' && (
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 h-full">
              <div className="lg:col-span-2 flex flex-col gap-4">
                <PortfolioSummary />
                <PositionsTable />
              </div>
              <div className="lg:col-span-1">
                <ChatPanel />
              </div>
            </div>
          )}
          {activeTab === 'chat' && (
            <div className="h-full">
              <ChatPanel />
            </div>
          )}
        </main>
      </div>
    </div>
  )
}
