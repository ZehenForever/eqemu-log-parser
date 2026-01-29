import { useCallback, useEffect, useRef, useState } from 'react'

import { GetEncounterList } from '../../wailsjs/go/main/App'

export function useSnapshot(pollMs = 500) {
  const [snapshot, setSnapshot] = useState(null)
  const [connected, setConnected] = useState(false)
  const [lastError, setLastError] = useState(null)

  const mountedRef = useRef(false)
  const inFlightRef = useRef(false)
  const snapshotRef = useRef(null)
  const sigRef = useRef('')

  const limit = 100
  const slowMs = 2000

  const fetchOnce = useCallback(async () => {
    if (inFlightRef.current) return
    inFlightRef.current = true

    try {
      const s = await GetEncounterList(limit)
      if (!mountedRef.current) return

      const first = Array.isArray(s?.encounters) && s.encounters.length > 0 ? s.encounters[0] : null
      const sig = `${s?.filePath || ''}|${s?.tailing ? 1 : 0}|${s?.lastHours || 0}|${s?.encounterCount || 0}|${first?.end || ''}|${first?.target || ''}`
      snapshotRef.current = s
      if (sig !== sigRef.current) {
        sigRef.current = sig
        setSnapshot(s)
      }
      setConnected(true)
      setLastError(null)
    } catch (e) {
      if (!mountedRef.current) return
      const msg = e && typeof e === 'object' && 'message' in e ? String(e.message) : String(e)
      setConnected(false)
      setLastError(msg)
    } finally {
      inFlightRef.current = false
    }
  }, [])

  const refreshNow = useCallback(() => {
    void fetchOnce()
  }, [fetchOnce])

  useEffect(() => {
    mountedRef.current = true
	let timerId = null
	let canceled = false

	const loop = async () => {
		if (canceled) return
		await fetchOnce()
		if (canceled) return
		const s = snapshotRef.current
		const nextMs = s && s.tailing ? pollMs : slowMs
		timerId = setTimeout(loop, nextMs)
	}

	void loop()

	return () => {
		canceled = true
		mountedRef.current = false
		if (timerId) clearTimeout(timerId)
	}
  }, [fetchOnce, pollMs])

  return { snapshot, connected, lastError, refreshNow }
}
