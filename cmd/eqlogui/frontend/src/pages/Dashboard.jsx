import React, { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'

import { ConfigureHub, ConfigureSubscribe, GetConfigDefaults, GetDamageBreakdownByKey, GetEncounterByKey, GetPlayersSeries, GetRemotePlayersSeries, ListHubRooms, PublishingStatus, SelectLogFile, Start, StartPublishing, StartSubscribe, Stop, StopPublishing, StopSubscribe, SubscribeStatus, SetIncludePCTargets, SetLastHours } from '../../wailsjs/go/main/App'

import { useSnapshot } from '../hooks/useSnapshot'

import { formatCompact, formatFloat1, formatInt } from '../lib/format'

import Modal from '../components/Modal'

function formatDuration(seconds) {
  if (typeof seconds !== 'number' || !Number.isFinite(seconds) || seconds < 0) return ''
  const s = Math.floor(seconds)
  const m = Math.floor(s / 60)
  const rem = s % 60
  if (m <= 0) return `${rem}s`
  return `${m}m ${rem}s`
}

function isPCLikeActorName(name) {
  const s = String(name || '').trim()
  if (s.length < 3 || s.length > 20) return false
  if (s.includes(' ') || s.includes('\t') || s.includes('\n') || s.includes('\r')) return false
  const c0 = s.charCodeAt(0)
  if (c0 < 65 || c0 > 90) return false
  for (let i = 0; i < s.length; i++) {
    const c = s.charCodeAt(i)
    const ch = s[i]
    const isUpper = c >= 65 && c <= 90
    const isLower = c >= 97 && c <= 122
    if (isUpper || isLower || ch === "'" || ch === '-') continue
    return false
  }
  return true
}

function StatusClock() {
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(id)
  }, [])

  return <>{new Date(now).toLocaleTimeString()}</>
}

export default function Dashboard() {
  const { snapshot, connected, lastError, refreshNow } = useSnapshot(500)
  const [selectedFile, setSelectedFile] = useState('')
  const [startAtEnd, setStartAtEnd] = useState(true)
  const [includePCTargets, setIncludePCTargets] = useState(false)
  const [lastHours, setLastHours] = useState(0)

  const [settingsOpen, setSettingsOpen] = useState(true)

  const [hubUrl, setHubUrl] = useState('https://sync.dpslogs.com')
  const [hubRoomId, setHubRoomId] = useState('')
  const [hubToken, setHubToken] = useState('')
  const [hubPublisherId, setHubPublisherId] = useState('')
  const [publishStatus, setPublishStatus] = useState(null)
  const [publishError, setPublishError] = useState('')

  const [activeTab, setActiveTab] = useState('encounters')

  const [playersBucketSec, setPlayersBucketSec] = useState(5)
  const [playersMaxBuckets, setPlayersMaxBuckets] = useState(100)
  const [playersMode, setPlayersMode] = useState('all')
  const [playersSeries, setPlayersSeries] = useState(null)
  const [playersError, setPlayersError] = useState('')
  const [playersSelectedActors, setPlayersSelectedActors] = useState(() => new Set())
  const playersInFlightRef = useRef(false)

  const [subscribeHubUrl, setSubscribeHubUrl] = useState('https://sync.dpslogs.com')
  const [subscribeRoomId, setSubscribeRoomId] = useState('')
  const [subscribeToken, setSubscribeToken] = useState('')
  const [subscribeRooms, setSubscribeRooms] = useState([])
  const [subscribeRoomsError, setSubscribeRoomsError] = useState('')
  const [subscribeStatus, setSubscribeStatus] = useState(null)

  const [configInfo, setConfigInfo] = useState({ configPath: '', configError: '' })

  const [roomPlayersMode, setRoomPlayersMode] = useState('all')
  const [roomPlayersSeries, setRoomPlayersSeries] = useState(null)
  const [roomPlayersError, setRoomPlayersError] = useState('')
  const [roomPlayersSelectedActors, setRoomPlayersSelectedActors] = useState(() => new Set())
	const roomPlayersNewestRef = useRef('')
  const roomPlayersInFlightRef = useRef(false)

  const [expandedEncounterKeys, setExpandedEncounterKeys] = useState(() => new Set())
  const [encounterDetails, setEncounterDetails] = useState({})
  const [uiError, setUIError] = useState('')

  const [breakdownOpen, setBreakdownOpen] = useState(false)
  const [breakdownLoading, setBreakdownLoading] = useState(false)
  const [breakdownError, setBreakdownError] = useState(null)
  const [breakdownData, setBreakdownData] = useState(null)
  const [breakdownKey, setBreakdownKey] = useState(null)

  const breakdownCacheRef = useRef(new Map())
  const breakdownKeyRef = useRef(null)

  const detailsInFlightRef = useRef(false)

  const tailing = !!snapshot?.tailing
  const filePath = snapshot?.filePath || selectedFile

  const effectiveLastHours = typeof snapshot?.lastHours === 'number' ? snapshot.lastHours : lastHours

  const formatLastHours = (h) => {
    if (typeof h !== 'number' || !Number.isFinite(h) || h <= 0) return ''
    if (Math.abs(h - Math.round(h)) < 1e-9) return `${Math.round(h)}h`
    return `${h}h`
  }

  const refreshPublishStatus = async () => {
    try {
      const s = await PublishingStatus()
      setPublishStatus(s)
      return s
    } catch {
      return null
    }
  }

  useEffect(() => {
    let canceled = false
    let timerId = null

    const loop = async () => {
      if (canceled) return
      const s = await refreshPublishStatus()
      if (canceled) return
      const enabled = !!s?.enabled
      const nextMs = enabled ? 1000 : 3000
      timerId = setTimeout(loop, nextMs)
    }

    void loop()
    return () => {
      canceled = true
      if (timerId) clearTimeout(timerId)
    }
  }, [])

  useEffect(() => {
    let canceled = false

    const load = async () => {
      try {
        const cfg = await GetConfigDefaults()
        if (canceled) return

        const nextHubUrl = cfg?.hubUrl || ''
        const nextRoomId = cfg?.roomId || ''
        const nextToken = cfg?.token || ''

        if (nextHubUrl) {
          setHubUrl(nextHubUrl)
          setSubscribeHubUrl(nextHubUrl)
        }
        if (nextRoomId) {
          setHubRoomId(nextRoomId)
          setSubscribeRoomId(nextRoomId)
        }
        if (nextToken) {
          setHubToken(nextToken)
          setSubscribeToken(nextToken)
        }

        setConfigInfo({ configPath: cfg?.configPath || '', configError: cfg?.configError || '' })
      } catch {
        if (canceled) return
        setConfigInfo({ configPath: '', configError: '' })
      }
    }

    void load()
    return () => {
      canceled = true
    }
  }, [])

  const onStartPublishing = async () => {
    setPublishError('')
    try {
      await ConfigureHub(hubUrl, hubRoomId, hubToken, hubPublisherId)
      await StartPublishing()
      await refreshPublishStatus()
    } catch (e) {
      setPublishError(String(e))
    }
  }

  const onStopPublishing = async () => {
    setPublishError('')
    try {
      await StopPublishing()
      await refreshPublishStatus()
    } catch (e) {
      setPublishError(String(e))
    }
  }

  const openBreakdown = async (ev, encounterKey, actor, target) => {
    if (ev) {
      if (typeof ev.preventDefault === 'function') ev.preventDefault()
      if (typeof ev.stopPropagation === 'function') ev.stopPropagation()
    }
    if (!encounterKey || !actor) return

    const key = `${encounterKey}|${actor}`
    setBreakdownKey(key)
    breakdownKeyRef.current = key

    const cached = breakdownCacheRef.current.get(key)
    if (cached) {
      setBreakdownData(cached)
      setBreakdownError(null)
      setBreakdownLoading(false)
      setBreakdownOpen(true)
      return
    }

    setBreakdownData(null)
    setBreakdownError(null)
    setBreakdownLoading(true)
    setBreakdownOpen(true)

    try {
      const res = await GetDamageBreakdownByKey(encounterKey, actor)
      if (breakdownKeyRef.current !== key) return
      if (!res || !Array.isArray(res.rows) || res.rows.length === 0) {
        setBreakdownError('No breakdown available')
        setBreakdownLoading(false)
        return
      }

      const withTarget = res?.target ? res : { ...res, target }
      breakdownCacheRef.current.set(key, withTarget)
      setBreakdownData(withTarget)
      setBreakdownLoading(false)
    } catch {
      if (breakdownKeyRef.current !== key) return
      setBreakdownError('No breakdown available')
      setBreakdownLoading(false)
    }
  }

  const encountersRaw = snapshot?.encounters || []
  const [encounters, setEncounters] = useState([])
  const encountersSigRef = useRef('')

  useEffect(() => {
    const list = Array.isArray(encountersRaw) ? encountersRaw : []
    const newest = list[0]
    const sig = `${list.length}|${newest?.encounterKey || ''}|${newest?.totalDamage || 0}|${newest?.encounterSec || 0}`
    if (sig === encountersSigRef.current) return
    encountersSigRef.current = sig
    setEncounters(list)
  }, [encountersRaw])

  useLayoutEffect(() => {
    if (!Array.isArray(encounters) || encounters.length === 0) return

    const present = new Set(encounters.map((e) => e?.encounterKey).filter(Boolean))
    setExpandedEncounterKeys((prev) => {
      if (!prev || prev.size === 0) return prev
      const next = new Set()
      let changed = false
      for (const k of prev) {
        if (present.has(k)) next.add(k)
        else changed = true
      }
      return changed ? next : prev
    })
  }, [encounters])

  const playersActors = useMemo(() => {
    return Array.isArray(playersSeries?.actors) ? playersSeries.actors : []
  }, [playersSeries])

  const refreshSubscribeStatus = async () => {
    try {
      const s = await SubscribeStatus()
      setSubscribeStatus(s)
      return s
    } catch {
      return null
    }
  }

  const onRefreshRooms = async () => {
    setSubscribeRoomsError('')
    try {
      const res = await ListHubRooms(subscribeHubUrl)
      const rooms = Array.isArray(res?.rooms) ? res.rooms : []
      setSubscribeRooms(rooms)
      if (!subscribeRoomId && rooms.length > 0) {
        setSubscribeRoomId(String(rooms[0].roomId || ''))
      }
    } catch (e) {
      setSubscribeRoomsError(String(e))
    }
  }

  const onSubscribeConnect = async () => {
    setRoomPlayersError('')
    try {
      await ConfigureSubscribe(subscribeHubUrl, subscribeRoomId, subscribeToken)
      await StartSubscribe()
      await refreshSubscribeStatus()
    } catch (e) {
      setRoomPlayersError(String(e))
    }
  }

  const onSubscribeDisconnect = async () => {
    setRoomPlayersError('')
    try {
      await StopSubscribe()
      await refreshSubscribeStatus()
    } catch (e) {
      setRoomPlayersError(String(e))
    }
  }

  const onUseSameRoomForSubscribe = () => {
    setSubscribeHubUrl(hubUrl)
    setSubscribeRoomId(hubRoomId)
    setSubscribeToken(hubToken)
  }

  const maxBucketDamage = useMemo(() => {
    const buckets = Array.isArray(playersSeries?.buckets) ? playersSeries.buckets : []
    let max = 0
    for (const b of buckets) {
      const v = Number(b?.totalDamage || 0)
      if (Number.isFinite(v) && v > max) max = v
    }
    return max
  }, [playersSeries])

  const getStableColor = (name) => {
    const s = String(name || '')
    let h = 0
    for (let i = 0; i < s.length; i++) {
      h = (h * 31 + s.charCodeAt(i)) >>> 0
    }
    const hue = h % 360
    return `hsl(${hue} 70% 50%)`
  }

  useEffect(() => {
    if (activeTab !== 'players') return
    if (!snapshot) return

    let canceled = false
    let timerId = null

    const pollOnce = async () => {
      if (playersInFlightRef.current) return
      playersInFlightRef.current = true
      try {
        const res = await GetPlayersSeries(playersBucketSec, playersMaxBuckets, playersMode)
        if (canceled) return
        setPlayersSeries(res)
        setPlayersError('')

        const nextActors = Array.isArray(res?.actors) ? res.actors : []
        setPlayersSelectedActors((prev) => {
          if (playersMode === 'me') {
            return new Set(nextActors)
          }
          const next = new Set(prev)
          for (const a of nextActors) {
            if (!next.has(a)) next.add(a)
          }
          // Prune removed actors
          for (const a of Array.from(next)) {
            if (!nextActors.includes(a)) next.delete(a)
          }
          return next
        })
      } catch (e) {
        if (canceled) return
        const msg = e && typeof e === 'object' && 'message' in e ? String(e.message) : String(e)
        setPlayersError(msg || 'Failed to load players series')
      } finally {
        playersInFlightRef.current = false
      }
    }

    const loop = async () => {
      if (canceled) return
      await pollOnce()
      if (canceled) return
      const nextMs = tailing ? 900 : 3000
      timerId = setTimeout(loop, nextMs)
    }

    void loop()

    return () => {
      canceled = true
      if (timerId) clearTimeout(timerId)
    }
  }, [activeTab, playersBucketSec, playersMaxBuckets, playersMode, snapshot, tailing])

  useEffect(() => {
    if (activeTab !== 'roomPlayers') return

    let canceled = false
    let timerId = null

    const pollOnce = async () => {
      if (roomPlayersInFlightRef.current) return null
      roomPlayersInFlightRef.current = true
      try {
        const st = await refreshSubscribeStatus()
        if (canceled) return

        if (!st?.connected) {
          setRoomPlayersSeries(null)
          return st
        }

        const series = await GetRemotePlayersSeries()
        if (canceled) return
        if (series) {
			const newest = Array.isArray(series?.buckets) && series.buckets.length > 0 ? String(series.buckets[0]?.bucketStart || '') : ''
			roomPlayersNewestRef.current = newest
			setRoomPlayersSeries(series)
		}

        const nextActors = (Array.isArray(series?.actors) ? series.actors : []).filter(isPCLikeActorName)
        setRoomPlayersSelectedActors((prev) => {
          if (roomPlayersMode === 'me') {
            return new Set(nextActors)
          }
          const next = new Set(prev)
          for (const a of nextActors) {
            if (!next.has(a)) next.add(a)
          }
          for (const a of Array.from(next)) {
            if (!nextActors.includes(a)) next.delete(a)
          }
          return next
        })
        return st
      } catch {
        return null
      } finally {
        roomPlayersInFlightRef.current = false
      }
    }

    const loop = async () => {
      if (canceled) return
      const st = await pollOnce()
      if (canceled) return
      const nextMs = st?.connected ? 1000 : 3000
      timerId = setTimeout(loop, nextMs)
    }

    void loop()
    return () => {
      canceled = true
      if (timerId) clearTimeout(timerId)
    }
  }, [activeTab, roomPlayersMode])

  const toggleExpanded = (encounterKey) => {
    if (!encounterKey) return
    setExpandedEncounterKeys((prev) => {
      const next = new Set(prev)
      if (next.has(encounterKey)) next.delete(encounterKey)
      else next.add(encounterKey)
      return next
    })
  }

  const encountersTabContent = useMemo(() => {
    return (
      <div className="mt-3 overflow-x-auto">
        <table className="min-w-full text-sm">
          <thead className="text-slate-400">
            <tr className="border-b border-slate-800">
              <th className="py-2 text-left font-medium">Target</th>
              <th className="py-2 text-right font-medium">Duration</th>
              <th className="py-2 text-right font-medium">TotalDamage</th>
              <th className="py-2 text-right font-medium">DPS(enc)</th>
              <th className="py-2 text-right font-medium"></th>
            </tr>
          </thead>
          <tbody>
            {encounters.length === 0 ? (
              <tr>
                <td className="py-3 text-slate-500" colSpan={5}>
                  No encounters yet.
                </td>
              </tr>
            ) : (
              encounters.map((e) => {
                const encounterKey = e.encounterKey
                const href = `/encounter/${encodeURIComponent(encounterKey || '')}`
                const isExpanded = expandedEncounterKeys.has(encounterKey)
                const chevron = isExpanded ? '▾' : '▸'

                const detail = encounterDetails[encounterKey]
                let actors = detail?.actors
                if (Array.isArray(actors)) {
                  actors = [...actors].sort((a, b) => (b.total || 0) - (a.total || 0))
                }

                return (
                  <React.Fragment key={encounterKey}>
                    <tr
                      className="border-b border-slate-900 hover:bg-slate-950/40 cursor-pointer"
                      onClick={() => toggleExpanded(encounterKey)}
                    >
                      <td className="py-2 pr-4">
                        <span className="text-slate-500 pr-2">{chevron}</span>
                        <span className="text-slate-100">{e.target}</span>
                      </td>
                      <td className="py-2 text-right font-mono tabular-nums text-slate-200">{formatDuration(e.encounterSec)}</td>
                      <td className="py-2 text-right font-mono tabular-nums text-slate-200" title={formatInt(e.totalDamage || 0)}>
                        {formatCompact(e.totalDamage || 0)}
                      </td>
                      <td className="py-2 text-right font-mono tabular-nums text-slate-200">{formatFloat1(e.dpsEncounter || 0)}</td>
                      <td className="py-2 pl-3 text-right">
                        <Link
                          to={href}
                          className="text-xs text-slate-300 hover:text-white hover:underline"
                          onClick={(ev) => ev.stopPropagation()}
                        >
                          Open
                        </Link>
                      </td>
                    </tr>

                    {isExpanded ? (
                      <tr className="border-b border-slate-900 bg-slate-950/20">
                        <td colSpan={5} className="py-3 pl-6 pr-2">
                          {!Array.isArray(actors) || actors.length === 0 ? (
                            <div className="text-sm text-slate-500">Loading...</div>
                          ) : (
                            <div className="overflow-x-auto">
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
                                    <th className="py-2 text-right font-medium"></th>
                                  </tr>
                                </thead>
                                <tbody>
                                  {actors.map((a) => (
                                    <tr key={a.actor} className="border-b border-slate-900">
                                      <td className="py-2 text-slate-200">{a.actor}</td>
                                      <td className="py-2 text-right font-mono tabular-nums text-slate-200">{formatFloat1(a.pctTotal || 0)}%</td>
                                      <td className="py-2 text-right font-mono tabular-nums text-slate-200" title={formatInt(a.total || 0)}>
                                        {formatCompact(a.total || 0)}
                                      </td>
                                      <td className="py-2 text-right font-mono tabular-nums text-slate-200">{formatFloat1(a.dpsEncounter || 0)}</td>
                                      <td className="py-2 text-right font-mono tabular-nums text-slate-200">{formatFloat1(a.sdps || 0)}</td>
                                      <td className="py-2 text-right font-mono tabular-nums text-slate-200">{formatInt(a.activeSec || 0)}</td>
                                      <td className="py-2 text-right font-mono tabular-nums text-slate-200">{formatInt(a.hits || 0)}</td>
                                      <td className="py-2 text-right font-mono tabular-nums text-slate-200" title={formatInt(a.maxHit || 0)}>
                                        {formatCompact(a.maxHit || 0)}
                                      </td>
                                      <td className="py-2 text-right font-mono tabular-nums text-slate-200">{formatFloat1(a.avgHit || 0)}</td>
                                      <td className="py-2 text-right font-mono tabular-nums text-slate-200">{formatFloat1(a.critPct || 0)}%</td>
                                      <td className="py-2 pl-3 text-right">
                                        <button
                                          className="rounded-md border border-slate-800 bg-slate-950 px-2 py-1 text-xs text-slate-200 hover:bg-slate-900"
                                          type="button"
                                          onMouseDown={(ev) => ev.stopPropagation()}
                                          onPointerDown={(ev) => ev.stopPropagation()}
                                          onClick={(ev) => openBreakdown(ev, encounterKey, a.actor, e.target)}
                                        >
                                          Breakdown
                                        </button>
                                      </td>
                                    </tr>
                                  ))}
                                </tbody>
                              </table>
                            </div>
                          )}
                        </td>
                      </tr>
                    ) : null}
                  </React.Fragment>
                )
              })
            )}
          </tbody>
        </table>
      </div>
    )
  }, [encounters, encounterDetails, expandedEncounterKeys])

  useEffect(() => {
    let alive = true

    const expanded = Array.from(expandedEncounterKeys)
    if (expanded.length === 0) return () => {}

    let timerId = null

    const fetchOnce = async (force) => {
      if (detailsInFlightRef.current) return

      const keys = Array.from(expandedEncounterKeys)
      if (keys.length === 0) return

      const need = force
        ? keys
        : keys.filter((encounterKey) => {
            const d = encounterDetails[encounterKey]
            return !(d && Array.isArray(d.actors) && d.actors.length > 0)
          })
      if (need.length === 0) return

      detailsInFlightRef.current = true
      try {
        for (const encounterKey of need) {
          const enc = await GetEncounterByKey(encounterKey)
          if (!alive) return
          setEncounterDetails((prev) => ({ ...prev, [encounterKey]: enc }))
        }
      } catch {
      } finally {
        detailsInFlightRef.current = false
      }
    }

    // Initial load (missing only)
    void fetchOnce(false)

    // While tailing, refresh expanded details periodically so ActiveSec/actors stay live.
    if (tailing) {
      timerId = setInterval(() => {
        void fetchOnce(true)
      }, 1000)
    }

    return () => {
      alive = false
      if (timerId) clearInterval(timerId)
    }
  }, [expandedEncounterKeys, encounterDetails, tailing])

  const onPickFile = async () => {
    setUIError('')
    try {
      const p = await SelectLogFile()
      if (p) setSelectedFile(p)
      refreshNow()
    } catch (e) {
      setUIError(String(e))
    }
  }

  const onStart = async () => {
    setUIError('')
    const p = selectedFile || snapshot?.filePath
    if (!p) {
      setUIError('Select a log file first')
      return
    }
    try {
      await Start(p, startAtEnd)
      refreshNow()
    } catch (e) {
      setUIError(String(e))
    }
  }

  const onStop = async () => {
    setUIError('')
    try {
      await Stop()
      refreshNow()
    } catch (e) {
      setUIError(String(e))
    }
  }

  const onToggleIncludePC = async (v) => {
    setIncludePCTargets(v)
    try {
      await SetIncludePCTargets(v)
      refreshNow()
    } catch (e) {
      setUIError(String(e))
    }
  }

  const onChangeLastHours = async (v) => {
    const raw = Number(v)
    const next = Number.isFinite(raw) && raw > 0 ? raw : 0
    setLastHours(next)
    setUIError('')
    try {
      await SetLastHours(next)
      refreshNow()
    } catch (e) {
      setUIError(String(e))
    }
  }

  return (
    <div className="space-y-4">
      <section className="rounded-lg border border-slate-800 bg-slate-900/30 p-4">
        <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
          <div>
            <div className="text-sm text-slate-400">Log file</div>
            <div className="font-mono text-sm break-all">{filePath || '(none selected)'}</div>
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={onPickFile}
              className="rounded-md bg-slate-100 px-3 py-2 text-sm font-medium text-slate-900 hover:bg-white"
              disabled={tailing}
            >
              Choose File
            </button>
            {!tailing ? (
              <button
                onClick={onStart}
                className="rounded-md bg-emerald-500 px-3 py-2 text-sm font-medium text-emerald-950 hover:bg-emerald-400"
              >
                Start
              </button>
            ) : (
              <button
                onClick={onStop}
                className="rounded-md bg-rose-500 px-3 py-2 text-sm font-medium text-rose-950 hover:bg-rose-400"
              >
                Stop
              </button>
            )}
          </div>
        </div>

        <div className="mt-3 flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
          <div className="text-sm">
            <span className="text-slate-400">Status:</span>{' '}
            <span className={tailing ? 'text-emerald-400' : 'text-slate-200'}>
              {tailing ? 'Tailing' : 'Stopped'}
            </span>
            {tailing && effectiveLastHours > 0 ? (
              <span className="text-slate-400">{' '}({`last ${formatLastHours(effectiveLastHours)}`})</span>
            ) : null}
            <span className={connected ? 'text-slate-400' : 'text-rose-300'}>
              {' '}
              · {connected ? 'Connected' : 'Disconnected'}
            </span>
            <span className="text-slate-500"> · <StatusClock /></span>
          </div>

          <div className="flex items-center gap-4">
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={startAtEnd}
                onChange={(e) => setStartAtEnd(e.target.checked)}
                disabled={tailing}
              />
              Start at end
            </label>
          </div>
        </div>

        {uiError ? <div className="mt-3 text-sm text-rose-300">{uiError}</div> : null}
        {!uiError && lastError ? <div className="mt-3 text-sm text-rose-300">{lastError}</div> : null}
      </section>

      <section className="rounded-lg border border-slate-800 bg-slate-900/30">
        <button
          type="button"
          onClick={() => setSettingsOpen((v) => !v)}
          aria-expanded={settingsOpen}
          className="flex w-full items-center justify-between gap-3 p-4 text-left"
        >
          <div className="min-w-0">
            <div className="flex items-center gap-2 font-semibold">
              <span
                className={
                  settingsOpen
                    ? 'transition-transform duration-200 ease-in-out'
                    : 'transition-transform duration-200 ease-in-out -rotate-90'
                }
                aria-hidden="true"
              >
                ▼
              </span>
              <span>Settings</span>
            </div>
            <div className="text-xs text-slate-400">
              {settingsOpen ? '(UI polling; no push events)' : 'Settings (collapsed)'}
            </div>
          </div>
          <div className="text-xs text-slate-500" aria-hidden="true">
            {settingsOpen ? '' : 'Expand'}
          </div>
        </button>

        <div
          className={
            settingsOpen
              ? 'max-h-[2000px] overflow-hidden px-4 pb-4 transition-[max-height] duration-300 ease-in-out'
              : 'max-h-0 overflow-hidden px-4 pb-0 transition-[max-height] duration-300 ease-in-out'
          }
          aria-hidden={!settingsOpen}
        >
          <div
            className={
              settingsOpen
                ? 'opacity-100 transition-opacity duration-200 ease-in-out'
                : 'opacity-0 transition-opacity duration-200 ease-in-out'
            }
          >
            <div className="mt-2 text-xs text-slate-500">
              {configInfo?.configPath
                ? configInfo?.configError
                  ? `Config error: ${configInfo.configError} (defaults used)`
                  : `Config loaded: ${configInfo.configPath}`
                : 'No config file found (defaults used)'}
            </div>

            <div className="mt-3 grid grid-cols-1 gap-3 md:grid-cols-2">
              <div className="rounded-md border border-slate-800 bg-slate-950/30 p-3">
                <div className="text-sm font-medium">Idle timeout</div>
                <div className="text-xs text-slate-400">8s (display only)</div>
              </div>

              <div className="rounded-md border border-slate-800 bg-slate-950/30 p-3">
                <div className="flex items-center justify-between">
                  <div>
                    <div className="text-sm font-medium">Include PC targets</div>
                    <div className="text-xs text-slate-400">Default: off</div>
                  </div>
                  <input
                    type="checkbox"
                    checked={includePCTargets}
                    onChange={(e) => onToggleIncludePC(e.target.checked)}
                  />
                </div>
              </div>

		  <div className="rounded-md border border-slate-800 bg-slate-950/30 p-3">
			<div className="text-sm font-medium">Last X hours</div>
			<div className="mt-2 flex items-center justify-between gap-3">
				<div className="text-xs text-slate-400">
					<div>0 = all history</div>
					<div>1 = last hour</div>
				</div>
				<input
					type="number"
					min={0}
					step={0.5}
					value={effectiveLastHours}
					onChange={(e) => onChangeLastHours(e.target.value)}
					disabled={tailing}
					className="w-24 rounded-md border border-slate-800 bg-slate-950 px-2 py-1 text-sm font-mono tabular-nums text-slate-100"
				/>
			</div>
		  </div>

		  <div className="rounded-md border border-slate-800 bg-slate-950/30 p-3 md:col-span-3">
			<div className="flex items-center justify-between">
				<div className="text-sm font-medium">Share</div>
				<button
					type="button"
					onClick={onUseSameRoomForSubscribe}
					className="rounded-md border border-slate-800 bg-slate-950 px-2 py-1 text-xs text-slate-200 hover:bg-slate-900"
				>
					Use same room for Subscribe
				</button>
			</div>
			<div className="mt-2 grid gap-3 md:grid-cols-2">
				<label className="text-sm text-slate-200">
					Hub URL
					<input
						type="text"
						value={hubUrl}
						onChange={(e) => setHubUrl(e.target.value)}
						className="mt-1 w-full rounded-md border border-slate-800 bg-slate-950 px-2 py-1 text-sm font-mono text-slate-100"
					/>
				</label>

				<label className="text-sm text-slate-200">
					Room ID
					<input
						type="text"
						value={hubRoomId}
						onChange={(e) => setHubRoomId(e.target.value)}
						className="mt-1 w-full rounded-md border border-slate-800 bg-slate-950 px-2 py-1 text-sm font-mono text-slate-100"
					/>
				</label>

				<label className="text-sm text-slate-200">
					Room Token
					<input
						type="password"
						value={hubToken}
						onChange={(e) => setHubToken(e.target.value)}
						className="mt-1 w-full rounded-md border border-slate-800 bg-slate-950 px-2 py-1 text-sm font-mono text-slate-100"
					/>
				</label>

				<label className="text-sm text-slate-200">
					Publisher ID (optional)
					<input
						type="text"
						value={hubPublisherId}
						onChange={(e) => setHubPublisherId(e.target.value)}
						className="mt-1 w-full rounded-md border border-slate-800 bg-slate-950 px-2 py-1 text-sm font-mono text-slate-100"
					/>
				</label>
			</div>

			<div className="mt-3 flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
				<div className="text-sm">
					<span className="text-slate-400">Status:</span>{' '}
					<span className={publishStatus?.enabled ? 'text-emerald-400' : 'text-slate-200'}>
						{publishStatus?.enabled ? 'Publishing' : 'Stopped'}
					</span>
					<span className="text-slate-500"> · Sent: {formatInt(publishStatus?.sentEvents || 0)}</span>
					{publishStatus?.lastError ? <span className="text-rose-300"> · {publishStatus.lastError}</span> : null}
				</div>

				<div className="flex items-center gap-2">
					{publishStatus?.enabled ? (
						<button
							onClick={onStopPublishing}
							className="rounded-md bg-rose-500 px-3 py-2 text-sm font-medium text-rose-950 hover:bg-rose-400"
						>
							Stop
						</button>
					) : (
						<button
							onClick={onStartPublishing}
							className="rounded-md bg-emerald-500 px-3 py-2 text-sm font-medium text-emerald-950 hover:bg-emerald-400"
						>
							Start
						</button>
					)}
				</div>
			</div>
			{publishError ? <div className="mt-2 text-sm text-rose-300">{publishError}</div> : null}
		  </div>

		  <div className="rounded-md border border-slate-800 bg-slate-950/30 p-3 md:col-span-3">
			<div className="text-sm font-medium">Subscribe</div>
			<div className="mt-2 grid gap-3 md:grid-cols-2">
				<label className="text-sm text-slate-200">
					Hub URL
					<input
						type="text"
						value={subscribeHubUrl}
						onChange={(e) => setSubscribeHubUrl(e.target.value)}
						className="mt-1 w-full rounded-md border border-slate-800 bg-slate-950 px-2 py-1 text-sm font-mono text-slate-100"
					/>
				</label>

				<div className="flex items-end gap-2">
					<button
						type="button"
						onClick={onRefreshRooms}
						className="rounded-md border border-slate-800 bg-slate-950 px-3 py-2 text-sm text-slate-200 hover:bg-slate-900"
					>
						Refresh rooms
					</button>
					<div className="text-xs text-slate-500">{subscribeRoomsError || ''}</div>
				</div>

				<label className="text-sm text-slate-200">
					Room
					<select
						value={subscribeRoomId}
						onChange={(e) => setSubscribeRoomId(e.target.value)}
						className="mt-1 w-full rounded-md border border-slate-800 bg-slate-950 px-2 py-1 text-sm text-slate-100"
					>
						<option value="">(select)</option>
						{subscribeRooms.map((r) => {
							const rid = String(r?.roomId || '')
							const label = `${rid}  (pub ${r?.publisherCount ?? 0}, sub ${r?.subscriberCount ?? 0}) ${r?.lastSeen || ''}`
							return (
								<option key={rid} value={rid}>
									{label}
								</option>
							)
						})}
					</select>
				</label>

				<label className="text-sm text-slate-200">
					Room Token
					<input
						type="password"
						value={subscribeToken}
						onChange={(e) => setSubscribeToken(e.target.value)}
						className="mt-1 w-full rounded-md border border-slate-800 bg-slate-950 px-2 py-1 text-sm font-mono text-slate-100"
					/>
				</label>
			</div>

			<div className="mt-3 flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
				<div className="text-sm">
					<span className="text-slate-400">Status:</span>{' '}
					<span className={subscribeStatus?.connected ? 'text-emerald-400' : 'text-slate-200'}>
						{subscribeStatus?.connected ? 'Connected' : 'Disconnected'}
					</span>
					{subscribeStatus?.roomId ? <span className="text-slate-500"> · {subscribeStatus.roomId}</span> : null}
					{subscribeStatus?.lastError ? <span className="text-rose-300"> · {subscribeStatus.lastError}</span> : null}
				</div>
				<div className="flex items-center gap-2">
					{subscribeStatus?.connected ? (
						<button
							type="button"
							onClick={onSubscribeDisconnect}
							className="rounded-md bg-rose-500 px-3 py-2 text-sm font-medium text-rose-950 hover:bg-rose-400"
						>
							Disconnect
						</button>
					) : (
						<button
							type="button"
							onClick={onSubscribeConnect}
							className="rounded-md bg-emerald-500 px-3 py-2 text-sm font-medium text-emerald-950 hover:bg-emerald-400"
						>
							Connect
						</button>
					)}
				</div>
			</div>
			{roomPlayersError ? <div className="mt-2 text-sm text-rose-300">{roomPlayersError}</div> : null}
		  </div>
			</div>
		  </div>
		</div>
      </section>

      <section className="rounded-lg border border-slate-800 bg-slate-900/30 p-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setActiveTab('encounters')}
              className={
                activeTab === 'encounters'
                  ? 'rounded-md bg-slate-100 px-3 py-1.5 text-sm font-medium text-slate-900'
                  : 'rounded-md border border-slate-800 bg-slate-950 px-3 py-1.5 text-sm text-slate-200 hover:bg-slate-900'
              }
            >
              Encounters
            </button>
            <button
              type="button"
              onClick={() => setActiveTab('players')}
              className={
                activeTab === 'players'
                  ? 'rounded-md bg-slate-100 px-3 py-1.5 text-sm font-medium text-slate-900'
                  : 'rounded-md border border-slate-800 bg-slate-950 px-3 py-1.5 text-sm text-slate-200 hover:bg-slate-900'
              }
            >
              Players
            </button>
			<button
				type="button"
				onClick={() => setActiveTab('roomPlayers')}
				className={
					activeTab === 'roomPlayers'
						? 'rounded-md bg-slate-100 px-3 py-1.5 text-sm font-medium text-slate-900'
						: 'rounded-md border border-slate-800 bg-slate-950 px-3 py-1.5 text-sm text-slate-200 hover:bg-slate-900'
				}
			>
				Room Players
			</button>
          </div>

          {activeTab === 'encounters' ? (
            <div className="flex items-center gap-3">
              <button
                className="rounded-md border border-slate-800 bg-slate-950 px-2 py-1 text-xs text-slate-200 hover:bg-slate-900"
                onClick={() => setExpandedEncounterKeys(new Set())}
              >
                Collapse all
              </button>
              <div className="text-xs text-slate-400">Count: {snapshot?.encounterCount ?? 0}</div>
            </div>
          ) : null}
        </div>

        {activeTab === 'encounters' ? (
          encountersTabContent
        ) : activeTab === 'players' ? (
          <div className="mt-3 space-y-3">
            <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
              <div className="flex flex-wrap items-center gap-3">
                <label className="text-sm text-slate-200">
                  Bucket
                  <select
                    value={playersBucketSec}
                    onChange={(e) => setPlayersBucketSec(Number(e.target.value) || 5)}
                    className="ml-2 rounded-md border border-slate-800 bg-slate-950 px-2 py-1 text-sm text-slate-100"
                  >
                    <option value={2}>2s</option>
                    <option value={5}>5s</option>
                    <option value={10}>10s</option>
                  </select>
                </label>

                <label className="text-sm text-slate-200">
                  Window
                  <select
                    value={playersMaxBuckets}
                    onChange={(e) => setPlayersMaxBuckets(Number(e.target.value) || 100)}
                    className="ml-2 rounded-md border border-slate-800 bg-slate-950 px-2 py-1 text-sm text-slate-100"
                  >
                    <option value={50}>50</option>
                    <option value={100}>100</option>
                    <option value={200}>200</option>
                  </select>
                </label>

                <div className="flex items-center gap-2 text-sm">
                  <span className="text-slate-200">Mode</span>
                  <button
                    type="button"
                    onClick={() => setPlayersMode('all')}
                    className={
                      playersMode === 'all'
                        ? 'rounded-md bg-slate-100 px-2 py-1 text-xs font-medium text-slate-900'
                        : 'rounded-md border border-slate-800 bg-slate-950 px-2 py-1 text-xs text-slate-200 hover:bg-slate-900'
                    }
                  >
                    All
                  </button>
                  <button
                    type="button"
                    onClick={() => setPlayersMode('me')}
                    className={
                      playersMode === 'me'
                        ? 'rounded-md bg-slate-100 px-2 py-1 text-xs font-medium text-slate-900'
                        : 'rounded-md border border-slate-800 bg-slate-950 px-2 py-1 text-xs text-slate-200 hover:bg-slate-900'
                    }
                  >
                    Me
                  </button>
                </div>
              </div>

              <div className="text-xs text-slate-400">
                {tailing ? 'Live (fast poll while tailing)' : 'Idle (slow poll while stopped)'}
              </div>
            </div>

            {playersError ? <div className="text-sm text-rose-300">{playersError}</div> : null}

            {playersMode === 'all' && playersActors.length > 0 ? (
              <div className="rounded-md border border-slate-800 bg-slate-950/30 p-3">
                <div className="text-sm font-medium text-slate-200">Actors</div>
                <div className="mt-2 flex flex-wrap gap-3">
                  {playersActors.map((a) => {
                    const checked = playersSelectedActors.has(a)
                    return (
                      <label key={a} className="flex items-center gap-2 text-sm text-slate-200">
                        <input
                          type="checkbox"
                          checked={checked}
                          onChange={(ev) => {
                            const v = ev.target.checked
                            setPlayersSelectedActors((prev) => {
                              const next = new Set(prev)
                              if (v) next.add(a)
                              else next.delete(a)
                              return next
                            })
                          }}
                        />
                        <span className="inline-flex items-center gap-2">
                          <span className="h-2 w-2 rounded" style={{ backgroundColor: getStableColor(a) }} />
                          {a}
                        </span>
                      </label>
                    )
                  })}
                </div>
              </div>
            ) : null}

            <div className="rounded-md border border-slate-800 bg-slate-950/30 p-3">
              <div className="text-xs text-slate-400">Newest first</div>
              <div className="mt-2 space-y-2">
                {Array.isArray(playersSeries?.buckets) && playersSeries.buckets.length > 0 ? (
                  playersSeries.buckets.map((b) => {
                    const ts = b.bucketStart ? new Date(b.bucketStart) : null
                    const label = ts && !Number.isNaN(ts.getTime()) ? ts.toLocaleTimeString() : ''
                    const total = Number(b.totalDamage || 0)
                    const sec = Number(b.bucketSec || playersBucketSec || 1)
                    const dps = sec > 0 ? total / sec : 0

                    let rowWidthPct = maxBucketDamage > 0 ? (total / maxBucketDamage) * 100 : 0
                    if (!Number.isFinite(rowWidthPct)) rowWidthPct = 0
                    if (rowWidthPct < 0) rowWidthPct = 0
                    if (rowWidthPct > 100) rowWidthPct = 100
                    if (total > 0 && rowWidthPct > 0 && rowWidthPct < 2) rowWidthPct = 2

                    const damageByActor = b.damageByActor || {}
                    const actors = playersMode === 'me' ? playersActors : playersActors.filter((a) => playersSelectedActors.has(a))
                    const segments = []
                    for (const a of actors) {
                      const dmg = Number(damageByActor[a] || 0)
                      if (!dmg) continue
                      segments.push({ actor: a, dmg })
                    }

                    return (
                      <div key={b.bucketStart} className="flex items-center gap-3">
                        <div className="w-24 text-xs font-mono tabular-nums text-slate-400">{label}</div>
                        <div className="flex-1">
                          <div className="h-5 w-full overflow-hidden rounded bg-slate-900">
                            <div className="h-full flex" style={{ width: `${rowWidthPct}%` }}>
                              {total > 0
                                ? segments.map((s) => {
                                    const pct = (s.dmg / total) * 100
                                    const tip = `${s.actor}: ${formatInt(s.dmg)} dmg, ${formatFloat1(s.dmg / sec)} DPS, ${formatFloat1(pct)}%`
                                    return (
                                      <div
                                        key={s.actor}
                                        title={tip}
                                        className="h-full"
                                        style={{ width: `${pct}%`, backgroundColor: getStableColor(s.actor) }}
                                      />
                                    )
                                  })
                                : null}
                            </div>
                          </div>
                        </div>
                        <div className="w-24 text-right text-xs font-mono tabular-nums text-slate-200" title={`${formatInt(total)} dmg / ${sec}s`}>
                          {formatFloat1(dps)}
                        </div>
                      </div>
                    )
                  })
                ) : (
                  <div className="text-sm text-slate-500">No bucket data yet.</div>
                )}
              </div>
            </div>
          </div>
        ) : (
		  <div className="mt-3 space-y-3">
			{!subscribeStatus?.connected ? (
				<div className="rounded-md border border-slate-800 bg-slate-950/30 p-4">
					<div className="text-sm text-slate-200">Not connected.</div>
					<div className="mt-1 text-xs text-slate-400">Choose a room and connect in Settings → Subscribe.</div>
				</div>
			) : null}

			{roomPlayersError ? <div className="text-sm text-rose-300">{roomPlayersError}</div> : null}

			{roomPlayersMode === 'all' && Array.isArray(roomPlayersSeries?.actors) && roomPlayersSeries.actors.length > 0 ? (
				<div className="rounded-md border border-slate-800 bg-slate-950/30 p-3">
					<div className="text-sm font-medium text-slate-200">Actors</div>
					<div className="mt-2 flex flex-wrap gap-3">
						{roomPlayersSeries.actors.filter(isPCLikeActorName).map((a) => {
							const checked = roomPlayersSelectedActors.has(a)
							return (
								<label key={a} className="flex items-center gap-2 text-sm text-slate-200">
									<input
										type="checkbox"
										checked={checked}
										onChange={(ev) => {
											const v = ev.target.checked
											setRoomPlayersSelectedActors((prev) => {
												const next = new Set(prev)
												if (v) next.add(a)
												else next.delete(a)
												return next
											})
										}}
									/>
									<span className="inline-flex items-center gap-2">
										<span className="h-2 w-2 rounded" style={{ backgroundColor: getStableColor(a) }} />
										{a}
									</span>
								</label>
							)
						})}
					</div>
				</div>
			) : null}

			<div className="rounded-md border border-slate-800 bg-slate-950/30 p-3">
				<div className="text-xs text-slate-400">Newest first</div>
				<div className="mt-2 space-y-2">
					{Array.isArray(roomPlayersSeries?.buckets) && roomPlayersSeries.buckets.length > 0 ? (
						roomPlayersSeries.buckets.map((b) => {
							const ts = b.bucketStart ? new Date(b.bucketStart) : null
							const label = ts && !Number.isNaN(ts.getTime()) ? ts.toLocaleTimeString() : ''
							const total = Number(b.totalDamage || 0)
							const sec = Number(b.bucketSec || roomPlayersSeries?.bucketSec || 1)
							const dps = sec > 0 ? total / sec : 0

							// compute max for remote series on the fly (local series uses memo)
							let max = 0
							for (const bb of roomPlayersSeries.buckets) {
								const v = Number(bb?.totalDamage || 0)
								if (Number.isFinite(v) && v > max) max = v
							}

							let rowWidthPct = max > 0 ? (total / max) * 100 : 0
							if (!Number.isFinite(rowWidthPct)) rowWidthPct = 0
							if (rowWidthPct < 0) rowWidthPct = 0
							if (rowWidthPct > 100) rowWidthPct = 100
							if (total > 0 && rowWidthPct > 0 && rowWidthPct < 2) rowWidthPct = 2

							const damageByActor = b.damageByActor || {}
							const roomActors = roomPlayersSeries.actors.filter(isPCLikeActorName)
							const actors = roomPlayersMode === 'me'
								? roomActors
								: roomActors.filter((a) => roomPlayersSelectedActors.has(a))
							const segments = []
							for (const a of actors) {
								const dmg = Number(damageByActor[a] || 0)
								if (!dmg) continue
								segments.push({ actor: a, dmg })
							}

							return (
								<div key={b.bucketStart} className="flex items-center gap-3">
									<div className="w-24 text-xs font-mono tabular-nums text-slate-400">{label}</div>
									<div className="flex-1">
										<div className="h-5 w-full overflow-hidden rounded bg-slate-900">
											<div className="h-full flex" style={{ width: `${rowWidthPct}%` }}>
												{total > 0
													? segments.map((s) => {
														const pct = (s.dmg / total) * 100
														const tip = `${s.actor}: ${formatInt(s.dmg)} dmg, ${formatFloat1(s.dmg / sec)} DPS, ${formatFloat1(pct)}%`
														return (
															<div
																key={s.actor}
																title={tip}
																className="h-full"
																style={{ width: `${pct}%`, backgroundColor: getStableColor(s.actor) }}
															/>
														)
													})
												: null}
											</div>
										</div>
									</div>
									<div className="w-24 text-right text-xs font-mono tabular-nums text-slate-200" title={`${formatInt(total)} dmg / ${sec}s`}>
										{formatFloat1(dps)}
									</div>
								</div>
							)
						})
					) : (
						<div className="text-sm text-slate-500">No bucket data yet.</div>
					)}
				</div>
			</div>
		  </div>
		)}
      </section>

      <Modal
        open={breakdownOpen}
        title="Damage Breakdown"
        onClose={() => {
          setBreakdownOpen(false)
          setBreakdownLoading(false)
          setBreakdownError(null)
        }}
      >
        <div className="space-y-3">
          <div className="text-sm text-slate-300">
            <div className="font-medium text-slate-100">{breakdownData?.actor || breakdownKey?.split('|')[1] || ''}</div>
            <div className="text-slate-400">{breakdownData?.target || ''}</div>
          </div>

          {breakdownLoading ? (
            <div className="text-sm text-slate-400">Loading...</div>
          ) : breakdownError ? (
            <div className="text-sm text-rose-300">{breakdownError}</div>
          ) : breakdownData && Array.isArray(breakdownData.rows) ? (
            <div className="overflow-x-auto">
              <table className="w-full min-w-[900px] text-sm">
                <thead className="text-slate-400">
                  <tr className="border-b border-slate-800">
                    <th className="px-3 py-2 text-left font-medium whitespace-nowrap">Name</th>
                    <th className="px-3 py-2 text-right font-medium whitespace-nowrap">% Player</th>
                    <th className="px-3 py-2 text-right font-medium whitespace-nowrap">Damage</th>
                    <th className="px-3 py-2 text-right font-medium whitespace-nowrap">DPS</th>
                    <th className="px-3 py-2 text-right font-medium whitespace-nowrap">SDPS</th>
                    <th className="px-3 py-2 text-right font-medium whitespace-nowrap">Sec</th>
                    <th className="px-3 py-2 text-right font-medium whitespace-nowrap">Hits</th>
                    <th className="px-3 py-2 text-right font-medium whitespace-nowrap">Max</th>
                    <th className="px-3 py-2 text-right font-medium whitespace-nowrap">Min</th>
                    <th className="px-3 py-2 text-right font-medium whitespace-nowrap">Avg</th>
                    <th className="px-3 py-2 text-right font-medium whitespace-nowrap">Crit%</th>
                  </tr>
                </thead>
                <tbody>
                  {breakdownData.rows.map((r) => (
                    <tr key={r.name} className="border-b border-slate-900">
                      <td className="px-3 py-2 text-slate-200 whitespace-nowrap">{r.name}</td>
                      <td className="px-3 py-2 text-right font-mono tabular-nums whitespace-nowrap text-slate-200">{formatFloat1(r.pctPlayer || 0)}%</td>
                      <td className="px-3 py-2 text-right font-mono tabular-nums whitespace-nowrap text-slate-200" title={formatInt(r.damage || 0)}>
                        {formatCompact(r.damage || 0)}
                      </td>
                      <td className="px-3 py-2 text-right font-mono tabular-nums whitespace-nowrap text-slate-200">{formatFloat1(r.dpsEncounter || 0)}</td>
                      <td className="px-3 py-2 text-right font-mono tabular-nums whitespace-nowrap text-slate-200">{formatFloat1(r.sdps || 0)}</td>
                      <td className="px-3 py-2 text-right font-mono tabular-nums whitespace-nowrap text-slate-200">{formatInt(r.sec || 0)}</td>
                      <td className="px-3 py-2 text-right font-mono tabular-nums whitespace-nowrap text-slate-200">{formatInt(r.hits || 0)}</td>
                      <td className="px-3 py-2 text-right font-mono tabular-nums whitespace-nowrap text-slate-200" title={formatInt(r.maxHit || 0)}>
                        {formatCompact(r.maxHit || 0)}
                      </td>
                      <td className="px-3 py-2 text-right font-mono tabular-nums whitespace-nowrap text-slate-200" title={formatInt(r.minHit || 0)}>
                        {formatCompact(r.minHit || 0)}
                      </td>
                      <td className="px-3 py-2 text-right font-mono tabular-nums whitespace-nowrap text-slate-200">{formatFloat1(r.avgHit || 0)}</td>
                      <td className="px-3 py-2 text-right font-mono tabular-nums whitespace-nowrap text-slate-200">{formatFloat1(r.critPct || 0)}%</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <div className="text-sm text-slate-400">No breakdown available</div>
          )}
        </div>
      </Modal>
    </div>
  )
}
