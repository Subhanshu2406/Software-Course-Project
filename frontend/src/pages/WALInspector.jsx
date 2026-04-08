import { useState, useCallback } from 'react'
import usePolling from '../hooks/usePolling'
import api from '../api/client'

const OP_COLORS = {
  CREDIT: 'badge-green',
  DEBIT: 'badge-red',
  PREPARE: 'badge-amber',
  COMMIT: 'badge-green',
  ABORT: 'badge-red',
  CHECKPOINT: 'badge-primary',
}

export default function WALInspector() {
  const [shard, setShard] = useState('shard1')
  const [limit, setLimit] = useState(50)
  const [search, setSearch] = useState('')

  const fetcher = useCallback(() => api.shardWAL(shard, limit), [shard, limit])
  const { data, loading, error } = usePolling(fetcher, 3000)

  const entries = data?.entries || []
  const totalEntries = data?.total_entries || 0
  const lastCheckpoint = data?.last_checkpoint_log_id || 0

  const filtered = entries.filter((e) => {
    if (!search) return true
    const q = search.toLowerCase()
    return (
      (e.txn_id || '').toLowerCase().includes(q) ||
      (e.op_type || '').toLowerCase().includes(q) ||
      (e.account_id || '').toLowerCase().includes(q)
    )
  })

  return (
    <>
      <p className="text-sm text-on-surface-variant font-mono">/shard-logs/wal-inspector</p>

      {/* Controls */}
      <div className="card flex flex-wrap items-center gap-3">
        <select
          className="bg-surface-container rounded-lg px-3 py-2 text-sm text-on-surface outline-none"
          value={shard}
          onChange={(e) => setShard(e.target.value)}
        >
          <option value="shard1">Shard 1</option>
          <option value="shard2">Shard 2</option>
          <option value="shard3">Shard 3</option>
        </select>
        <select
          className="bg-surface-container rounded-lg px-3 py-2 text-sm text-on-surface outline-none"
          value={limit}
          onChange={(e) => setLimit(Number(e.target.value))}
        >
          <option value={25}>25</option>
          <option value={50}>50</option>
          <option value={100}>100</option>
        </select>
        <input
          className="bg-surface-container rounded-lg px-3 py-2 text-sm text-on-surface placeholder:text-on-surface-variant/50 outline-none flex-1 min-w-[200px]"
          placeholder="Filter by Txn ID or Op Type..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <span className="flex items-center gap-1.5 text-xs text-on-surface-variant ml-auto">
          <span className="status-dot bg-emerald-500 animate-pulse" />
          Auto-Refresh
        </span>
      </div>

      {error && <div className="text-error text-sm">{error}</div>}

      {/* Table */}
      <div className="card overflow-x-auto">
        <table className="w-full text-left">
          <thead>
            <tr className="text-on-surface-variant text-xs uppercase tracking-wider">
              <th className="py-2 px-4">Log ID</th>
              <th className="py-2 px-4">Txn ID</th>
              <th className="py-2 px-4">Operation</th>
              <th className="py-2 px-4">Account</th>
              <th className="py-2 px-4 text-right">Amount</th>
              <th className="py-2 px-4">State</th>
              <th className="py-2 px-4">Timestamp</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-outline-variant/5">
            {filtered.map((e, i) => (
              <tr key={i} className="hover:bg-surface-container/50 transition-colors">
                <td className="py-2 px-4 font-mono text-xs">{e.log_id ?? i}</td>
                <td className="py-2 px-4 font-mono text-xs text-primary truncate max-w-[120px]">
                  {e.txn_id || '—'}
                </td>
                <td className="py-2 px-4">
                  <span className={OP_COLORS[e.op_type] || 'badge-primary'}>
                    {e.op_type || '—'}
                  </span>
                </td>
                <td className="py-2 px-4 font-mono text-xs">{e.account_id || e.account || '—'}</td>
                <td className="py-2 px-4 text-right font-mono text-xs">
                  {e.amount != null ? e.amount : '—'}
                </td>
                <td className="py-2 px-4">
                  <span
                    className={e.committed ? 'badge-green' : 'badge-amber'}
                  >
                    {e.committed ? 'Committed' : 'Pending'}
                  </span>
                </td>
                <td className="py-2 px-4 font-mono text-xs text-on-surface-variant">
                  {e.timestamp ? new Date(e.timestamp).toLocaleTimeString() : '—'}
                </td>
              </tr>
            ))}
            {filtered.length === 0 && (
              <tr>
                <td colSpan={7} className="py-8 text-center text-on-surface-variant">
                  {loading ? 'Loading...' : 'No WAL entries found'}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* Footer info */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <div className="card">
          <div className="label-sm">Last Checkpoint</div>
          <div className="font-mono text-sm mt-1">{lastCheckpoint}</div>
        </div>
        <div className="card">
          <div className="label-sm">Total Entries</div>
          <div className="font-mono text-sm mt-1">{totalEntries}</div>
        </div>
        <div className="card">
          <div className="label-sm">Viewing</div>
          <div className="font-mono text-sm mt-1">{filtered.length} entries</div>
        </div>
        <div className="card">
          <div className="label-sm">Shard</div>
          <div className="font-mono text-sm mt-1 text-primary">{shard}</div>
        </div>
      </div>
    </>
  )
}
