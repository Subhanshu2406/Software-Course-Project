import { useCluster } from '../context/ClusterContext'
import { useCallback } from 'react'
import usePolling from '../hooks/usePolling'
import api from '../api/client'

function StatCard({ label, value, sub, icon, color = 'text-primary' }) {
  return (
    <div className="card">
      <div className="label-sm mb-2">{label}</div>
      <div className={`text-2xl font-semibold font-mono ${color}`}>{value}</div>
      {sub && <div className="text-xs text-on-surface-variant mt-1">{sub}</div>}
    </div>
  )
}

function ShardRow({ id, data }) {
  if (!data) return null
  const isOnline = data.online
  return (
    <tr className="hover:bg-surface-container transition-colors">
      <td className="py-3 px-4 font-mono text-sm">{data.shard_id || id}</td>
      <td className="py-3 px-4">
        <span className={isOnline ? 'badge-green' : 'badge-red'}>
          {isOnline ? 'Healthy' : 'Offline'}
        </span>
      </td>
      <td className="py-3 px-4">
        <span className="badge-primary">{data.role || 'PRIMARY'}</span>
      </td>
      <td className="py-3 px-4">
        <div className="flex items-center gap-2">
          <div className="flex-1 h-1.5 bg-surface-highest rounded-full max-w-[120px]">
            <div
              className="h-full bg-primary rounded-full transition-all"
              style={{ width: `${Math.min(100, (data.queue_depth || 0) / 5)}%` }}
            />
          </div>
          <span className="font-mono text-xs">{data.queue_depth ?? 0}</span>
        </div>
      </td>
      <td className="py-3 px-4 font-mono text-sm">{(data.tps ?? data.total_qps ?? 0).toFixed(1)}</td>
      <td className="py-3 px-4 font-mono text-xs text-on-surface-variant">{data.wal_index ?? 0}</td>
      <td className="py-3 px-4 font-mono text-xs">{data.account_count ?? 0}</td>
    </tr>
  )
}

export default function ClusterOverview() {
  const { shards, coordinator, isLoading, error } = useCluster()
  const txnFetcher = useCallback(() => api.coordinatorTransactions(10), [])
  const { data: txnData } = usePolling(txnFetcher, 3000)
  const recentTxns = Array.isArray(txnData) ? txnData : txnData?.transactions || []

  const mapFetcher = useCallback(() => api.loadMonitorShardMap().catch(() => null), [])
  const { data: mapData } = usePolling(mapFetcher, 5000)

  const shardList = Object.entries(shards)
  const totalTps = shardList.reduce((sum, [, s]) => sum + (s.tps || s.total_qps || 0), 0)
  const totalBalance = shardList.reduce((sum, [, s]) => sum + (s.total_balance || 0), 0)
  const onlineCount = shardList.filter(([, s]) => s.online).length

  // Show loading state
  if (isLoading && shardList.length === 0) {
    return (
      <div className="text-center py-12">
        <div className="spinner mx-auto mb-4"></div>
        <p className="text-on-surface-variant">Connecting to cluster...</p>
      </div>
    )
  }

  // Show error banner if there are connectivity issues
  if (error) {
    return (
      <div className="space-y-4">
        <div className="card border-l-4 border-error bg-error/10">
          <h2 className="text-error font-semibold mb-2">⚠️ Connection Issues</h2>
          <p className="text-sm mb-3">{error}</p>
          <details className="text-xs mt-2">
            <summary className="cursor-pointer text-primary hover:underline">Troubleshooting Steps</summary>
            <div className="mt-2 space-y-1 text-on-surface-variant">
              <p>1. Check if Docker containers are running:</p>
              <code className="block bg-surface p-2 rounded my-1">docker compose ps</code>
              <p>2. Ensure containers show "Healthy":</p>
              <code className="block bg-surface p-2 rounded my-1">docker compose logs coordinator | tail -20</code>
              <p>3. Restart the frontend container:</p>
              <code className="block bg-surface p-2 rounded my-1">docker compose restart frontend</code>
              <p>4. Check browser console (F12 → Console tab) for network error details</p>
            </div>
          </details>
        </div>
        {shardList.length > 0 && <p className="text-xs text-on-surface-variant">Showing stale data: {JSON.stringify(shards)}</p>}
      </div>
    )
  }

  return (
    <>
      {/* Stat cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard
          label="System Balance"
          value={`₹${totalBalance.toLocaleString()}`}
          sub="Invariant holds"
          color="text-emerald-400"
        />
        <StatCard label="Total TPS" value={totalTps} sub={`${onlineCount} shards active`} />
        <StatCard
          label="Committed"
          value={shardList.reduce((s, [, d]) => s + (d.committed_count || 0), 0)}
          sub="transactions"
          color="text-emerald-400"
        />
        <StatCard
          label="Aborted"
          value={shardList.reduce((s, [, d]) => s + (d.aborted_count || 0), 0)}
          sub="transactions"
          color="text-error"
        />
      </div>

      {/* Shard Health Table */}
      <div className="card overflow-x-auto">
        <h2 className="label-sm mb-4">Shard Health Registry</h2>
        <table className="w-full text-left">
          <thead>
            <tr className="text-on-surface-variant text-xs uppercase tracking-wider">
              <th className="py-2 px-4">Shard</th>
              <th className="py-2 px-4">Status</th>
              <th className="py-2 px-4">Role</th>
              <th className="py-2 px-4">Queue Depth</th>
              <th className="py-2 px-4">TPS</th>
              <th className="py-2 px-4">WAL Index</th>
              <th className="py-2 px-4">Accounts</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-outline-variant/5">
            {shardList.map(([id, data]) => (
              <ShardRow key={id} id={id} data={data} />
            ))}
            {shardList.length === 0 && (
              <tr>
                <td colSpan={7} className="py-8 text-center text-on-surface-variant">
                  No shard data yet...
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* Bottom: Partition Map + Recent Transactions */}
      <div className="grid grid-cols-1 lg:grid-cols-5 gap-6">
        {/* Partition map */}
        <div className="card lg:col-span-3">
          <h2 className="label-sm mb-4">Partition Map</h2>
          <div className="grid grid-cols-10 gap-1">
            {Array.from({ length: 30 }, (_, i) => {
              const shardId =
                mapData?.partitions?.[String(i)] ||
                (i < 10 ? 'shard1' : i < 20 ? 'shard2' : 'shard3')
              const color =
                shardId === 'shard1'
                  ? 'bg-primary/60'
                  : shardId === 'shard2'
                  ? 'bg-emerald-500/60'
                  : 'bg-amber-500/60'
              return (
                <div
                  key={i}
                  className={`${color} rounded aspect-square flex items-center justify-center text-[10px] font-mono text-on-surface`}
                  title={`P${i} → ${shardId}`}
                >
                  {i}
                </div>
              )
            })}
          </div>
          <div className="flex gap-4 mt-3 text-xs text-on-surface-variant">
            <span className="flex items-center gap-1"><span className="w-3 h-3 rounded bg-primary/60" /> Shard 1</span>
            <span className="flex items-center gap-1"><span className="w-3 h-3 rounded bg-emerald-500/60" /> Shard 2</span>
            <span className="flex items-center gap-1"><span className="w-3 h-3 rounded bg-amber-500/60" /> Shard 3</span>
          </div>
        </div>

        {/* Recent txns */}
        <div className="card lg:col-span-2">
          <h2 className="label-sm mb-4">Recent Transactions</h2>
          <div className="space-y-2 max-h-64 overflow-y-auto">
            {recentTxns.map((tx, i) => (
              <div key={i} className="flex items-center justify-between text-xs py-1.5">
                <span className="font-mono text-on-surface-variant truncate max-w-[100px]">
                  {tx.txn_id?.slice(0, 12) || '—'}
                </span>
                <span className={tx.type === 'cross' ? 'badge-blue' : 'badge-purple'}>
                  {tx.type || 'single'}
                </span>
                <span className="font-mono">{tx.amount ?? 0}</span>
                <span
                  className={
                    tx.state === 'committed'
                      ? 'badge-green'
                      : tx.state === 'aborted'
                      ? 'badge-red'
                      : 'badge-amber'
                  }
                >
                  {tx.state || 'pending'}
                </span>
              </div>
            ))}
            {recentTxns.length === 0 && (
              <p className="text-on-surface-variant text-xs">No transactions yet</p>
            )}
          </div>
        </div>
      </div>
    </>
  )
}
