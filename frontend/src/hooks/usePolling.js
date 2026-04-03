import { useState, useEffect, useRef, useCallback } from 'react'

export default function usePolling(fetcher, intervalMs = 2000) {
  const [data, setData] = useState(null)
  const [error, setError] = useState(null)
  const [loading, setLoading] = useState(true)
  const timerRef = useRef(null)
  const mountedRef = useRef(true)

  const refresh = useCallback(async () => {
    try {
      const result = await fetcher()
      if (mountedRef.current) {
        setData(result)
        setError(null)
      }
    } catch (err) {
      if (mountedRef.current) setError(err.message)
    } finally {
      if (mountedRef.current) setLoading(false)
    }
  }, [fetcher])

  useEffect(() => {
    mountedRef.current = true
    setLoading(true)
    refresh()
    timerRef.current = setInterval(refresh, intervalMs)
    return () => {
      mountedRef.current = false
      clearInterval(timerRef.current)
    }
  }, [refresh, intervalMs])

  return { data, error, loading, refresh }
}
