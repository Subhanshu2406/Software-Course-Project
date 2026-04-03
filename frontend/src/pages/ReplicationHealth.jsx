import { useCluster } from '../context/ClusterContext'

export default function ReplicationHealth() {
  const { shards } = useCluster()
  const shardList = Object.entries(shards)
  const onlineCount = shardList.filter(([, s]) => s.online).length

  return (
    <>
      <p className="text-sm text-on-surface-variant">WAL replication status across shard groups.</p>

      {/* Global health summary */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <div className="card">
          <div className="label-sm">Total Shards</div>
          <div className="text-2xl font-mono font-semibold mt-1">{shardList.length}</div>
        </div>
        <div className="card">
          <div className="label-sm">Online</div>
          <div className="text-2xl font-mono font-semibold text-emerald-400 mt-1">{onlineCount}</div>
        </div>
        <div className="card">
          <div className="label-sm">Followers</div>
          <div className="text-2xl font-mono font-semibold mt-1">
            {shardList.reduce((s, [, d]) => s + (d.follower_count || 0), 0)}
          </div>
        </div>
        <div className="card border-l-4 border-primary">
          <div className="label-sm">Quorum Health</div>
          <div className="text-2xl font-mono font-semibold text-primary mt-1">
            {onlineCount > 0 ? '100%' : '0%'}
          </div>
        </div>
      </div>

      {/* Shard group cards */}
      {shardList.map(([id, data]) => (
        <div key={id} className="card">
          <div className="flex items-center justify-between mb-4">
            <div>
              <h3 className="font-mono text-sm font-medium">{data.shard_id || id}</h3>
              <span className="text-xs text-on-surface-variant font-mono">
                WAL Index: {data.wal_index ?? 0}
              </span>
            </div>
            <span className={data.online ? 'badge-green' : 'badge-red'}>
              {data.role || 'PRIMARY'} {data.online ? 'Active' : 'Down'}
            </span>
          </div>

          <table className="w-full text-left text-xs">
            <thead>
              <tr className="text-on-surface-variant uppercase tracking-wider">
                <th className="py-1.5 px-3">Node</th>
                <th className="py-1.5 px-3">Role</th>
                <th className="py-1.5 px-3">WAL Index</th>
                <th className="py-1.5 px-3">Followers</th>
                <th className="py-1.5 px-3">Status</th>
              </tr>
            </thead>
            <tbody>
              <tr className="hover:bg-surface-container/50">
                <td className="py-2 px-3 font-mono">{data.shard_id || id}</td>
                <td className="py-2 px-3"><span className="badge-primary">{data.role || 'PRIMARY'}</span></td>
                <td className="py-2 px-3 font-mono">{data.wal_index ?? 0}</td>
                <td className="py-2 px-3 font-mono">{data.follower_count ?? 0}</td>
                <td className="py-2 px-3">
                  <span className={data.online ? 'badge-green' : 'badge-red'}>
                    {data.online ? 'Synchronized' : 'Disconnected'}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>

          {/* WAL progress bar */}
          <div className="mt-3">
            <div className="flex justify-between text-xs text-on-surface-variant mb-1">
              <span>WAL Propagation</span>
              <span>{data.wal_index ?? 0} entries</span>
            </div>
            <div className="h-1.5 bg-surface-highest rounded-full">
              <div className="h-full bg-primary rounded-full" style={{ width: '100%' }} />
            </div>
          </div>
        </div>
      ))}

      {shardList.length === 0 && (
        <div className="card text-center text-on-surface-variant py-12">No shard data available</div>
      )}
    </>
  )
}
