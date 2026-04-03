import { useCallback } from 'react'
import usePolling from '../hooks/usePolling'
import api from '../api/client'
import { useCluster } from '../context/ClusterContext'

export default function LoadMonitor() {
  const { shards } = useCluster()
  const shardList = Object.entries(shards)

  const metricsFetcher = useCallback(() => api.loadMonitorMetrics(), [])
  const migrationsFetcher = useCallback(() => api.loadMonitorMigrations(), [])
  const { data: metrics } = usePolling(metricsFetcher, 3000)
  const { data: migrationsData } = usePolling(migrationsFetcher, 5000)
  const migrations = Array.isArray(migrationsData) ? migrationsData : migrationsData?.migrations || []

  // Find hottest and coolest
  const sorted = [...shardList].sort(
    ([, a], [, b]) => (b.queue_depth || 0) - (a.queue_depth || 0)
  )
  const hottest = sorted[0]
  const coolest = sorted[sorted.length - 1]

  return (
    <>
      <p className="text-sm text-on-surface-variant font-mono">/shard-orchestration/load-monitor</p>

      {/* Top stat cards */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div className="card">
          <div className="label-sm">Hottest Shard</div>
          <div className="text-xl font-mono font-semibold text-error mt-1">
            {hottest ? hottest[1].shard_id || hottest[0] : '—'}
          </div>
          <div className="text-xs text-on-surface-variant mt-1">
            Queue: {hottest ? hottest[1].queue_depth || 0 : 0}
          </div>
        </div>
        <div className="card">
          <div className="label-sm">Coolest Shard</div>
          <div className="text-xl font-mono font-semibold text-primary mt-1">
            {coolest ? coolest[1].shard_id || coolest[0] : '—'}
          </div>
          <div className="text-xs text-on-surface-variant mt-1">
            Queue: {coolest ? coolest[1].queue_depth || 0 : 0}
          </div>
        </div>
        <div className="card">
          <div className="label-sm">Migrations Triggered</div>
          <div className="flex items-center gap-2 mt-1">
            <span className="text-xl font-mono font-semibold text-tertiary">
              {migrations.length}
            </span>
            <span className="status-dot bg-emerald-500 animate-pulse" />
            <span className="text-xs text-on-surface-variant">LIVE</span>
          </div>
        </div>
      </div>

      {/* Queue depth per shard — bar chart representation */}
      <div className="card">
        <h2 className="label-sm mb-4">Queue Depth per Shard</h2>
        <div className="space-y-3">
          {shardList.map(([id, data]) => {
            const depth = data.queue_depth || 0
            const maxDepth = Math.max(...shardList.map(([, d]) => d.queue_depth || 1), 1)
            const colors = {
              shard1: 'bg-primary',
              shard2: 'bg-emerald-500',
              shard3: 'bg-amber-500',
            }
            return (
              <div key={id} className="flex items-center gap-3">
                <span className="font-mono text-xs w-16 text-on-surface-variant">{data.shard_id || id}</span>
                <div className="flex-1 h-6 bg-surface-highest rounded">
                  <div
                    className={`h-full ${colors[id] || 'bg-primary'} rounded transition-all flex items-center px-2`}
                    style={{ width: `${Math.max(2, (depth / maxDepth) * 100)}%` }}
                  >
                    <span className="text-[10px] font-mono text-surface font-medium">{depth}</span>
                  </div>
                </div>
              </div>
            )
          })}
        </div>
      </div>

      {/* Migration log */}
      <div className="card">
        <h2 className="label-sm mb-4">Migration History</h2>
        {migrations.length > 0 ? (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead>
                <tr className="text-on-surface-variant uppercase tracking-wider">
                  <th className="py-2 px-3">Timestamp</th>
                  <th className="py-2 px-3">Partition</th>
                  <th className="py-2 px-3">Source</th>
                  <th className="py-2 px-3">Target</th>
                  <th className="py-2 px-3">Duration</th>
                  <th className="py-2 px-3">Status</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-outline-variant/5">
                {migrations.map((m, i) => (
                  <tr key={i} className="hover:bg-surface-container/50">
                    <td className="py-2 px-3 font-mono">
                      {m.triggered_at ? new Date(m.triggered_at).toLocaleTimeString() : '—'}
                    </td>
                    <td className="py-2 px-3 font-mono">{m.partition_id ?? '—'}</td>
                    <td className="py-2 px-3 font-mono">{m.from_shard || '—'}</td>
                    <td className="py-2 px-3 font-mono">{m.to_shard || '—'}</td>
                    <td className="py-2 px-3 font-mono">{m.duration_ms ? `${m.duration_ms}ms` : '—'}</td>
                    <td className="py-2 px-3">
                      <span className={m.success ? 'badge-green' : 'badge-red'}>
                        {m.success ? 'Complete' : 'Failed'}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <p className="text-on-surface-variant text-xs">No migrations recorded</p>
        )}
      </div>
    </>
  )
}
