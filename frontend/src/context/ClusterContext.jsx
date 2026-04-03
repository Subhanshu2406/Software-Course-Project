import { createContext, useContext, useState, useCallback, useEffect, useRef } from 'react'
import api from '../api/client'

const ClusterContext = createContext(null)

const SHARD_IDS = ['shard1', 'shard2', 'shard3']

export function ClusterProvider({ children }) {
  const [shards, setShards] = useState({})
  const [coordinator, setCoordinator] = useState(null)
  const [loadMonitor, setLoadMonitor] = useState(null)
  const [error, setError] = useState(null)
  const [isLoading, setIsLoading] = useState(true)
  const mountedRef = useRef(true)

  const refresh = useCallback(async () => {
    try {
      const results = {}

      // Fetch shard metrics in parallel
      const shardPromises = SHARD_IDS.map(async (id) => {
        try {
          const metrics = await api.shardMetrics(id)
          results[id] = { ...metrics, online: true }
        } catch (err) {
          console.warn(`Failed to fetch ${id} metrics:`, err.message)
          results[id] = { shard_id: id, online: false, error: err.message }
        }
      })

      // Fetch coordinator metrics
      let coordData = null
      try {
        coordData = await api.coordinatorMetrics()
      } catch (err) {
        console.warn('Failed to fetch coordinator metrics:', err.message)
      }

      // Fetch load monitor metrics
      let lmData = null
      try {
        lmData = await api.loadMonitorMetrics()
      } catch (err) {
        console.warn('Failed to fetch load monitor metrics:', err.message)
      }

      // Wait for all shard requests to complete
      await Promise.all(shardPromises)

      if (mountedRef.current) {
        setShards(results)
        setCoordinator(coordData)
        setLoadMonitor(lmData)
        setError(null)
        setIsLoading(false)
      }
    } catch (err) {
      console.error('Error in refresh:', err)
      if (mountedRef.current) {
        setError(err.message)
        setIsLoading(false)
      }
    }
  }, [])

  useEffect(() => {
    mountedRef.current = true
    refresh()
    // Poll every 3 seconds
    const timer = setInterval(refresh, 3000)

    return () => {
      mountedRef.current = false
      clearInterval(timer)
    }
  }, [refresh])

  return (
    <ClusterContext.Provider value={{ shards, coordinator, loadMonitor, error, isLoading, refresh }}>
      {children}
    </ClusterContext.Provider>
  )
}

export function useCluster() {
  const ctx = useContext(ClusterContext)
  if (!ctx) throw new Error('useCluster must be used within ClusterProvider')
  return ctx
}
