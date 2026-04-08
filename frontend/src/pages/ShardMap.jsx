import { useCluster } from '../context/ClusterContext'
import { useCallback } from 'react'
import usePolling from '../hooks/usePolling'
import api from '../api/client'

const SHARD_COLORS = {
  shard1: { bg: 'bg-primary/70 hover:bg-primary', dot: 'bg-primary/70', label: 'Shard 1' },
  shard2: { bg: 'bg-emerald-500/70 hover:bg-emerald-500', dot: 'bg-emerald-500/70', label: 'Shard 2' },
  shard3: { bg: 'bg-amber-500/70 hover:bg-amber-500', dot: 'bg-amber-500/70', label: 'Shard 3' },
}

export default function ShardMap() {
  const { shards } = useCluster()
  const shardList = Object.entries(shards)

  const mapFetcher = useCallback(() => api.loadMonitorShardMap().catch(() => null), [])
  const { data: mapData } = usePolling(mapFetcher, 5000)

  const partitions = Array.from({ length: 30 }, (_, i) => {
    const shardId = mapData?.partitions?.[String(i)] || (i < 10 ? 'shard1' : i < 20 ? 'shard2' : 'shard3')
    return { id: i, shardId }
  })

  // Count partitions per shard
  const shardPartCounts = {}
  partitions.forEach((p) => {
    shardPartCounts[p.shardId] = (shardPartCounts[p.shardId] || 0) + 1
  })

  return (
    <>
      <p className="text-sm text-on-surface-variant">Partition ownership and shard topology.</p>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Partition Grid */}
        <div className="card lg:col-span-2">
          <div className="flex items-center justify-between mb-4">
            <h2 className="label-sm">Partition Grid</h2>
            <span className="text-xs text-on-surface-variant">30 Active Segments</span>
          </div>
          <div className="grid grid-cols-10 gap-1.5">
            {partitions.map((p) => {
              const colorSet = SHARD_COLORS[p.shardId] || SHARD_COLORS.shard1
              return (
                <div
                  key={p.id}
                  className={`${colorSet.bg} rounded-lg aspect-square flex flex-col items-center justify-center cursor-pointer transition-all hover:scale-105`}
                  title={`Partition ${p.id} → ${p.shardId}`}
                >
                  <span className="text-[10px] font-mono text-on-surface/80">#{String(p.id).padStart(2, '0')}</span>
                </div>
              )
            })}
          </div>
          <div className="flex gap-4 mt-4 text-xs text-on-surface-variant">
            {Object.entries(SHARD_COLORS).map(([id, c]) => (
              <span key={id} className="flex items-center gap-1.5">
                <span className={`w-3 h-3 rounded ${c.dot}`} /> {c.label} ({shardPartCounts[id] || 0})
              </span>
            ))}
          </div>
        </div>

        {/* Shard Detail Cards */}
        <div className="space-y-4">
          {shardList.map(([id, data]) => (
            <div key={id} className="card border-l-4 border-primary">
              <div className="flex items-center justify-between mb-2">
                <span className="font-mono text-sm font-medium">{data.shard_id || id}</span>
                <span className={data.online ? 'badge-green' : 'badge-red'}>
                  {data.online ? 'Online' : 'Offline'}
                </span>
              </div>
              <div className="grid grid-cols-2 gap-2 text-xs">
                <div>
                  <span className="text-on-surface-variant">Accounts</span>
                  <div className="font-mono">{data.account_count ?? 0}</div>
                </div>
                <div>
                  <span className="text-on-surface-variant">Balance</span>
                  <div className="font-mono">{(data.total_balance ?? 0).toLocaleString()}</div>
                </div>
                <div>
                  <span className="text-on-surface-variant">WAL Index</span>
                  <div className="font-mono">{data.wal_index ?? 0}</div>
                </div>
                <div>
                  <span className="text-on-surface-variant">Role</span>
                  <div className="font-mono text-primary">{data.role || 'PRIMARY'}</div>
                </div>
              </div>
            </div>
          ))}
          {shardList.length === 0 && (
            <div className="card text-center text-on-surface-variant text-sm">No shard data</div>
          )}
        </div>
      </div>
    </>
  )
}
