import React from 'react'
import { HashRouter, Routes, Route, Link } from 'react-router-dom'
import Dashboard from './pages/Dashboard.jsx'
import EncounterDetail from './pages/EncounterDetail.jsx'

export default function App() {
  return (
    <HashRouter>
      <div className="min-h-screen">
        <header className="border-b border-slate-800 bg-slate-950">
          <div className="mx-auto max-w-6xl px-4 py-3 flex items-center justify-between">
            <Link to="/" className="text-lg font-semibold tracking-tight">
              EQEmu Log Parser
            </Link>
            <div className="text-xs text-slate-400">Wails + React</div>
          </div>
        </header>

        <main className="mx-auto max-w-6xl px-4 py-4">
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/encounter/:encounterKey" element={<EncounterDetail />} />
          </Routes>
        </main>
      </div>
    </HashRouter>
  )
}
