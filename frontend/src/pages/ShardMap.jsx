import { useCluster } from '../context/ClusterContext'

export default function ShardMap() {
  const { shards } = useCluster()
  const shardList = Object.entries(shards)

  const partitions = Array.from({ length: 30 }, (_, i) => {
    const shardId = i < 10 ? 'shard1' : i < 20 ? 'shard2' : 'shard3'
    return { id: i, shardId }
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
              const color =
                p.shardId === 'shard1'
                  ? 'bg-primary/70 hover:bg-primary'
                  : p.shardId === 'shard2'
                  ? 'bg-emerald-500/70 hover:bg-emerald-500'
                  : 'bg-amber-500/70 hover:bg-amber-500'
              return (
                <div
                  key={p.id}
                  className={`${color} rounded-lg aspect-square flex flex-col items-center justify-center cursor-pointer transition-all hover:scale-105`}
                >
                  <span className="text-[10px] font-mono text-on-surface/80">#{String(p.id).padStart(2, '0')}</span>
                </div>
              )
            })}
          </div>
          <div className="flex gap-4 mt-4 text-xs text-on-surface-variant">
            <span className="flex items-center gap-1.5">
              <span className="w-3 h-3 rounded bg-primary/70" /> Shard 1 (0-9)
            </span>
            <span className="flex items-center gap-1.5">
              <span className="w-3 h-3 rounded bg-emerald-500/70" /> Shard 2 (10-19)
            </span>
            <span className="flex items-center gap-1.5">
              <span className="w-3 h-3 rounded bg-amber-500/70" /> Shard 3 (20-29)
            </span>
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
