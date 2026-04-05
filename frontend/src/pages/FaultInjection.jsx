import { useState, useCallback } from 'react'
import usePolling from '../hooks/usePolling'
import api from '../api/client'

export default function FaultInjection() {
  const [log, setLog] = useState([])
  const [acting, setActing] = useState(null)

  const statusFetcher = useCallback(() => api.faultStatus().catch(() => ({})), [])
  const { data: status, refresh } = usePolling(statusFetcher, 3000)

  const addLog = (msg, type = 'info') => {
    setLog((prev) => [{ msg, type, ts: new Date().toISOString() }, ...prev].slice(0, 50))
  }

  const handleKill = async (container) => {
    setActing(container)
    try {
      const res = await api.faultKill(container)
      addLog(`FAULT_INJECTED: Killed ${container}`, 'error')
      refresh()
    } catch (err) {
      addLog(`FAILED to kill ${container}: ${err.message}`, 'error')
    } finally {
      setActing(null)
    }
  }

  const handleRestart = async (container) => {
    setActing(container)
    try {
      const res = await api.faultRestart(container)
      addLog(`RECOVERY: Restarted ${container}`, 'success')
      refresh()
    } catch (err) {
      addLog(`FAILED to restart ${container}: ${err.message}`, 'error')
    } finally {
      setActing(null)
    }
  }

  const shardContainers = ['shard1', 'shard2', 'shard3']
  const coordinatorContainers = ['coordinator', 'coordinator2']
  const getContainerStatus = (name) => {
    if (!status) return 'unknown'
    const c = status[name]
    if (!c) return 'unknown'
    return c.running === 'true' || c.status === 'running' ? 'running' : 'stopped'
  }

  return (
    <>
      {/* Warning banner */}
      <div className="bg-tertiary/10 border border-tertiary/20 rounded-xl p-4 flex items-center gap-3">
        <span className="material-icons-outlined text-tertiary animate-pulse">warning</span>
        <div>
          <div className="text-sm font-medium text-tertiary">CRITICAL ACCESS: Fault Injection Mode</div>
          <div className="text-xs text-on-surface-variant">
            Actions on this page will affect live containers. Use with caution.
          </div>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Shard & coordinator controls */}
        <div className="lg:col-span-2 space-y-6">
          {/* Shard failures */}
          <div>
            <h2 className="label-sm mb-3">Shard Failures</h2>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              {shardContainers.map((name) => {
                const st = getContainerStatus(name)
                return (
                  <div key={name} className="card bg-gradient-to-b from-surface-low to-surface">
                    <div className="flex items-center justify-between mb-3">
                      <span className="font-mono text-sm font-medium">{name}</span>
                      <span
                        className={`status-dot ${
                          st === 'running' ? 'bg-emerald-500' : st === 'stopped' ? 'bg-red-500' : 'bg-amber-500'
                        }`}
                      />
                    </div>
                    <div className="text-xs text-on-surface-variant mb-3">
                      Status: <span className="font-mono">{st}</span>
                    </div>
                    <div className="flex gap-2">
                      <button
                        onClick={() => handleKill(name)}
                        disabled={acting === name}
                        className="flex-1 px-3 py-1.5 rounded-lg bg-error/15 text-error text-xs font-medium hover:bg-error/25 transition-colors disabled:opacity-50"
                      >
                        Kill
                      </button>
                      <button
                        onClick={() => handleRestart(name)}
                        disabled={acting === name}
                        className="flex-1 px-3 py-1.5 rounded-lg bg-emerald-500/15 text-emerald-400 text-xs font-medium hover:bg-emerald-500/25 transition-colors disabled:opacity-50"
                      >
                        Restart
                      </button>
                    </div>
                  </div>
                )
              })}
            </div>
          </div>

          {/* Coordinator failures */}
          <div>
            <h2 className="label-sm mb-3">Coordinator Failures</h2>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {coordinatorContainers.map((name) => {
                const st = getContainerStatus(name)
                return (
                  <div key={name} className="card flex items-center gap-4">
                    <span className="material-icons-outlined text-on-surface-variant">hub</span>
                    <div className="flex-1">
                      <div className="font-mono text-sm">{name}</div>
                      <div className="text-xs text-on-surface-variant">{st}</div>
                    </div>
                    <div className="flex gap-2">
                      <button
                        onClick={() => handleKill(name)}
                        disabled={acting === name}
                        className="px-3 py-1.5 rounded-lg bg-error/15 text-error text-xs font-medium hover:bg-error/25 disabled:opacity-50"
                      >
                        Kill
                      </button>
                      <button
                        onClick={() => handleRestart(name)}
                        disabled={acting === name}
                        className="px-3 py-1.5 rounded-lg bg-emerald-500/15 text-emerald-400 text-xs font-medium hover:bg-emerald-500/25 disabled:opacity-50"
                      >
                        Restart
                      </button>
                    </div>
                  </div>
                )
              })}
            </div>
          </div>
        </div>

        {/* Fault event log */}
        <div className="card glass-panel sticky top-6 self-start max-h-[70vh] flex flex-col">
          <div className="flex items-center gap-2 mb-4">
            <span className="status-dot bg-emerald-500 animate-pulse" />
            <h2 className="label-sm">Fault Event Log</h2>
          </div>
          <div className="flex-1 overflow-y-auto space-y-2">
            {log.length === 0 && (
              <p className="text-xs text-on-surface-variant">No events yet. Perform an action.</p>
            )}
            {log.map((entry, i) => (
              <div key={i} className="text-xs font-mono">
                <span className="text-on-surface-variant">{new Date(entry.ts).toLocaleTimeString()}</span>{' '}
                <span
                  className={
                    entry.type === 'error'
                      ? 'text-error'
                      : entry.type === 'success'
                      ? 'text-emerald-400'
                      : 'text-tertiary'
                  }
                >
                  {entry.msg}
                </span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </>
  )
}
