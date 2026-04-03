import { useCluster } from '../context/ClusterContext'

export default function Metrics() {
  const { shards, coordinator } = useCluster()
  const shardList = Object.entries(shards)
  const totalTps = shardList.reduce((s, [, d]) => s + (d.tps || d.total_qps || 0), 0)

  return (
    <>
      <div>
        <p className="text-sm text-on-surface-variant">
          Deep-level system observability and real-time protocol telemetry.
        </p>
      </div>

      {/* Top stat cards */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div className="card col-span-2">
          <div className="label-sm mb-2">Current Network Throughput</div>
          <div className="text-3xl font-semibold font-mono text-primary">
            {totalTps.toLocaleString()} <span className="text-lg text-on-surface-variant">TPS</span>
          </div>
          <div className="mt-3 h-2 bg-surface-highest rounded-full">
            <div
              className="h-full bg-gradient-to-r from-primary to-primary-container rounded-full transition-all"
              style={{ width: `${Math.min(100, (totalTps / 1000) * 100)}%` }}
            />
          </div>
        </div>
        <div className="card border-l-4 border-error/50">
          <div className="label-sm mb-2">Abort Rate</div>
          <div className="text-3xl font-semibold font-mono">
            {(() => {
              const committed = shardList.reduce((s, [, d]) => s + (d.committed_count || 0), 0)
              const aborted = shardList.reduce((s, [, d]) => s + (d.aborted_count || 0), 0)
              const total = committed + aborted
              return total > 0 ? ((aborted / total) * 100).toFixed(2) : '0.00'
            })()}{' '}
            <span className="text-lg text-on-surface-variant">%</span>
          </div>
          <div className="text-xs text-emerald-400 mt-1">Within Operational Bounds</div>
        </div>
      </div>

      {/* Grafana embed */}
      <div className="card">
        <div className="flex items-center justify-between mb-4">
          <h2 className="label-sm">Grafana Dashboard</h2>
          <div className="flex gap-2">
            <a
              href="/grafana/d/ledger-overview"
              target="_blank"
              rel="noopener noreferrer"
              className="btn-ghost text-xs"
            >
              Open in Grafana
            </a>
            <a
              href="http://localhost:9090"
              target="_blank"
              rel="noopener noreferrer"
              className="btn-ghost text-xs"
            >
              Open Prometheus
            </a>
          </div>
        </div>
        <iframe
          src="/grafana/d/ledger-overview?orgId=1&kiosk&theme=dark"
          className="w-full rounded-lg border border-outline-variant/10"
          style={{ height: '500px' }}
          title="Grafana Dashboard"
        />
      </div>

      {/* Per-shard metrics grid */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        {shardList.map(([id, data]) => (
          <div key={id} className="card">
            <div className="flex items-center justify-between mb-3">
              <span className="font-mono text-sm font-medium">{data.shard_id || id}</span>
              <span className={data.online ? 'badge-green' : 'badge-red'}>
                {data.online ? 'Online' : 'Offline'}
              </span>
            </div>
            <div className="space-y-2 text-xs">
              {[
                ['TPS', (data.tps ?? data.total_qps ?? 0).toFixed(1)],
                ['Queue', data.queue_depth],
                ['WAL Index', data.wal_index],
                ['Accounts', data.account_count],
                ['Committed', data.committed_count],
                ['Aborted', data.aborted_count],
                ['Uptime', data.uptime_seconds ? `${Math.floor(data.uptime_seconds)}s` : '0s'],
              ].map(([label, value]) => (
                <div key={label} className="flex justify-between">
                  <span className="text-on-surface-variant">{label}</span>
                  <span className="font-mono">{value ?? 0}</span>
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>
    </>
  )
}
