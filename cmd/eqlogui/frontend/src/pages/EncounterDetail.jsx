import React, { useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'

import { GetEncounterByKey } from '../../wailsjs/go/main/App'

import { formatCompact, formatFloat1, formatInt } from '../lib/format'

export default function EncounterDetail() {
  const { encounterId } = useParams()
  const decodedEncounterKey = useMemo(() => {
    try {
      return decodeURIComponent(encounterId || '')
    } catch {
      return encounterId || ''
    }
  }, [encounterId])

  const [encounter, setEncounter] = useState(null)
  const [error, setError] = useState('')
  const [backendConnected, setBackendConnected] = useState(null)

  const [pollMs] = useState(750)

  useEffect(() => {
    let alive = true
    let inFlight = false

    const poll = async () => {
      if (inFlight) return
      inFlight = true
      try {
        const e = await GetEncounterByKey(decodedEncounterKey)
        if (!alive) return
        setEncounter(e)
        setBackendConnected(true)
      } catch (e) {
        if (!alive) return
        setBackendConnected(false)
        setError('Wails backend not connected')
      } finally {
        inFlight = false
      }
    }

    poll()
    const id = setInterval(() => {
      if (backendConnected === false) return
      void poll()
    }, pollMs)
    return () => {
      alive = false
      clearInterval(id)
    }
  }, [backendConnected, decodedEncounterKey, pollMs])

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <div className="text-sm text-slate-400">Encounter</div>
          <div className="text-xl font-semibold">{encounter?.target || ''}</div>
        </div>
        <Link to="/" className="text-sm text-slate-200 hover:underline">
          Back
        </Link>
      </div>

      {!encounter ? (
        <div className="rounded-lg border border-slate-800 bg-slate-900/30 p-4 text-slate-400">
          {error ? error : 'Encounter not found in current snapshot.'}
        </div>
      ) : (
        <div className="rounded-lg border border-slate-800 bg-slate-900/30 p-4">
          <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
            <div>
              <div className="text-xs text-slate-400">Duration</div>
              <div className="font-mono tabular-nums">{encounter.encounterSec}s</div>
            </div>
            <div>
              <div className="text-xs text-slate-400">TotalDamage</div>
              <div className="font-mono tabular-nums" title={formatInt(encounter.totalDamage || 0)}>
                {formatInt(encounter.totalDamage || 0)}
              </div>
              <div className="text-xs text-slate-500" title={formatInt(encounter.totalDamage || 0)}>
                {formatCompact(encounter.totalDamage || 0)}
              </div>
            </div>
            <div>
              <div className="text-xs text-slate-400">DPS(enc)</div>
              <div className="font-mono tabular-nums">{formatFloat1(encounter.dpsEncounter || 0)}</div>
            </div>
            <div>
              <div className="text-xs text-slate-400">Actors</div>
              <div className="font-mono tabular-nums">{formatInt((encounter.actors || []).length)}</div>
            </div>
          </div>

          <div className="mt-4 overflow-x-auto">
            <table className="min-w-full text-sm">
              <thead className="text-slate-400">
                <tr className="border-b border-slate-800">
                  <th className="py-2 text-left font-medium">Actor</th>
                  <th className="py-2 text-right font-medium">%Total</th>
                  <th className="py-2 text-right font-medium">Total</th>
                  <th className="py-2 text-right font-medium">DPS(enc)</th>
                  <th className="py-2 text-right font-medium">SDPS</th>
                  <th className="py-2 text-right font-medium">ActiveSec</th>
                  <th className="py-2 text-right font-medium">Hits</th>
                  <th className="py-2 text-right font-medium">MaxHit</th>
                  <th className="py-2 text-right font-medium">AvgHit</th>
                  <th className="py-2 text-right font-medium">Crit%</th>
                </tr>
              </thead>
              <tbody>
                {(encounter.actors || []).map((a) => (
                  <tr key={a.actor} className="border-b border-slate-900">
                    <td className="py-2 pr-4">{a.actor}</td>
                    <td className="py-2 text-right font-mono tabular-nums">{formatFloat1(a.pctTotal || 0)}%</td>
                    <td className="py-2 text-right font-mono tabular-nums" title={formatCompact(a.total || 0)}>
                      {formatInt(a.total || 0)}
                    </td>
                    <td className="py-2 text-right font-mono tabular-nums">{formatFloat1(a.dpsEncounter || 0)}</td>
                    <td className="py-2 text-right font-mono tabular-nums">{formatFloat1(a.sdps || 0)}</td>
                    <td className="py-2 text-right font-mono tabular-nums">{formatInt(a.activeSec || 0)}</td>
                    <td className="py-2 text-right font-mono tabular-nums">{formatInt(a.hits || 0)}</td>
                    <td className="py-2 text-right font-mono tabular-nums" title={formatInt(a.maxHit || 0)}>
                      {formatCompact(a.maxHit || 0)}
                    </td>
                    <td className="py-2 text-right font-mono tabular-nums">{formatFloat1(a.avgHit || 0)}</td>
                    <td className="py-2 text-right font-mono tabular-nums">{formatFloat1(a.critPct || 0)}%</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  )
}
