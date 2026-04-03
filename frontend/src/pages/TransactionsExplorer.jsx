import { useState, useCallback } from 'react'
import usePolling from '../hooks/usePolling'
import api from '../api/client'

export default function TransactionsExplorer() {
  const [shard, setShard] = useState('coordinator')
  const [limit, setLimit] = useState(25)
  const [statusFilter, setStatusFilter] = useState('')
  const [typeFilter, setTypeFilter] = useState('')
  const [search, setSearch] = useState('')

  const fetcher = useCallback(() => {
    if (shard === 'coordinator') return api.coordinatorTransactions(limit)
    return api.shardTransactions(shard, limit)
  }, [shard, limit])

  const { data: txnData, loading, error } = usePolling(fetcher, 3000)
  const txns = Array.isArray(txnData) ? txnData : txnData?.transactions || []

  const filtered = txns.filter((tx) => {
    if (statusFilter && tx.state !== statusFilter) return false
    if (typeFilter && tx.type !== typeFilter) return false
    if (search) {
      const q = search.toLowerCase()
      const haystack = `${tx.txn_id} ${tx.source} ${tx.destination} ${tx.shard_id || ''}`.toLowerCase()
      if (!haystack.includes(q)) return false
    }
    return true
  })

  return (
    <>
      <div>
        <p className="text-sm text-on-surface-variant">
          Real-time log of all cross-shard and single-shard atomic commitments.
        </p>
      </div>

      {/* Filters */}
      <div className="card grid grid-cols-1 md:grid-cols-5 gap-3">
        <input
          className="bg-surface-container rounded-lg px-3 py-2 text-sm text-on-surface placeholder:text-on-surface-variant/50 outline-none"
          placeholder="Search by TXN ID, Source..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <select
          className="bg-surface-container rounded-lg px-3 py-2 text-sm text-on-surface outline-none"
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value)}
        >
          <option value="">All Status</option>
          <option value="committed">Committed</option>
          <option value="aborted">Aborted</option>
          <option value="prepared">Prepared</option>
        </select>
        <select
          className="bg-surface-container rounded-lg px-3 py-2 text-sm text-on-surface outline-none"
          value={typeFilter}
          onChange={(e) => setTypeFilter(e.target.value)}
        >
          <option value="">All Types</option>
          <option value="single">Single-Shard</option>
          <option value="cross">Cross-Shard</option>
        </select>
        <select
          className="bg-surface-container rounded-lg px-3 py-2 text-sm text-on-surface outline-none"
          value={shard}
          onChange={(e) => setShard(e.target.value)}
        >
          <option value="coordinator">Coordinator</option>
          <option value="shard1">Shard 1</option>
          <option value="shard2">Shard 2</option>
          <option value="shard3">Shard 3</option>
        </select>
        <select
          className="bg-surface-container rounded-lg px-3 py-2 text-sm text-on-surface outline-none"
          value={limit}
          onChange={(e) => setLimit(Number(e.target.value))}
        >
          <option value={25}>25 rows</option>
          <option value={50}>50 rows</option>
          <option value={100}>100 rows</option>
        </select>
      </div>

      {error && <div className="text-error text-sm">{error}</div>}

      {/* Table */}
      <div className="card overflow-x-auto">
        <table className="w-full text-left">
          <thead>
            <tr className="text-on-surface-variant text-xs uppercase tracking-wider">
              <th className="py-2 px-4">Transaction ID</th>
              <th className="py-2 px-4">Timestamp</th>
              <th className="py-2 px-4">Source / Dest</th>
              <th className="py-2 px-4 text-right">Amount</th>
              <th className="py-2 px-4">Type</th>
              <th className="py-2 px-4">Status</th>
              <th className="py-2 px-4 text-right">Latency</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-outline-variant/5">
            {filtered.map((tx, i) => (
              <tr key={i} className="hover:bg-surface-container transition-colors text-sm">
                <td className="py-2.5 px-4 font-mono text-xs text-primary truncate max-w-[140px]">
                  {tx.txn_id || '—'}
                </td>
                <td className="py-2.5 px-4 text-xs text-on-surface-variant font-mono">
                  {tx.timestamp ? new Date(tx.timestamp).toLocaleTimeString() : '—'}
                </td>
                <td className="py-2.5 px-4 text-xs font-mono">
                  {tx.source || '—'} → {tx.destination || '—'}
                </td>
                <td className="py-2.5 px-4 text-right font-mono">{tx.amount ?? 0}</td>
                <td className="py-2.5 px-4">
                  <span className={tx.type === 'cross' ? 'badge-blue' : 'badge-purple'}>
                    {tx.type || 'single'}
                  </span>
                </td>
                <td className="py-2.5 px-4">
                  <span
                    className={
                      tx.state === 'committed'
                        ? 'badge-green'
                        : tx.state === 'aborted'
                        ? 'badge-red'
                        : 'badge-amber'
                    }
                  >
                    {tx.state || 'unknown'}
                  </span>
                </td>
                <td className="py-2.5 px-4 text-right font-mono text-xs">
                  {tx.latency_ms != null ? `${tx.latency_ms}ms` : '—'}
                </td>
              </tr>
            ))}
            {filtered.length === 0 && (
              <tr>
                <td colSpan={7} className="py-8 text-center text-on-surface-variant">
                  {loading ? 'Loading...' : 'No transactions found'}
                </td>
              </tr>
            )}
          </tbody>
        </table>
        <div className="mt-3 text-xs text-on-surface-variant">
          Showing {filtered.length} of {(txns || []).length} transactions
        </div>
      </div>
    </>
  )
}
