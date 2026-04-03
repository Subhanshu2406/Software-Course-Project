import { NavLink, useLocation } from 'react-router-dom'
import { useCluster } from '../context/ClusterContext'

const NAV = [
  { to: '/cluster', icon: 'dashboard', label: 'Clusters' },
  { to: '/metrics', icon: 'monitoring', label: 'Metrics' },
  { to: '/transactions', icon: 'receipt_long', label: 'Transactions' },
  { to: '/wal', icon: 'article', label: 'WAL Inspector' },
  { to: '/shard-map', icon: 'grid_view', label: 'Shard Map' },
  { to: '/replication', icon: 'sync', label: 'Replication' },
  { to: '/load-monitor', icon: 'speed', label: 'Load Monitor' },
  { to: '/transfer', icon: 'send', label: 'Transfer' },
  { to: '/fault-injection', icon: 'bug_report', label: 'Fault Injection' },
]

function SidebarLink({ to, icon, label }) {
  return (
    <NavLink
      to={to}
      className={({ isActive }) =>
        `flex items-center gap-3 px-4 py-2.5 rounded-lg text-sm transition-colors ${
          isActive
            ? 'bg-surface-container text-primary border-r-2 border-primary'
            : 'text-on-surface-variant hover:bg-surface-container/60'
        }`
      }
    >
      <span className="material-icons-outlined text-lg">{icon}</span>
      {label}
    </NavLink>
  )
}

export default function Layout({ children }) {
  const location = useLocation()
  const { shards } = useCluster()
  const onlineCount = Object.values(shards).filter((s) => s.online).length

  const pageTitle = NAV.find((n) => location.pathname.startsWith(n.to))?.label ?? 'LedgerOS'

  return (
    <div className="flex h-screen overflow-hidden">
      {/* Sidebar */}
      <aside className="w-60 flex-shrink-0 bg-surface-low flex flex-col h-full">
        <div className="p-5 flex items-center gap-3">
          <div className="w-8 h-8 rounded-lg bg-gradient-to-br from-primary to-primary-container flex items-center justify-center text-surface font-bold text-sm">
            L
          </div>
          <span className="font-semibold text-on-surface tracking-tight">LedgerOS</span>
        </div>
        <nav className="flex-1 px-3 space-y-1 overflow-y-auto">
          {NAV.map((item) => (
            <SidebarLink key={item.to} {...item} />
          ))}
        </nav>
        <div className="px-4 pb-4 space-y-2">
          <div className="flex items-center gap-2 text-xs text-on-surface-variant">
            <span className={`status-dot ${onlineCount > 0 ? 'bg-emerald-500' : 'bg-red-500'}`} />
            {onlineCount}/3 shards online
          </div>
        </div>
      </aside>

      {/* Main */}
      <div className="flex-1 flex flex-col min-w-0">
        {/* Header */}
        <header className="h-14 flex-shrink-0 flex items-center justify-between px-6 bg-surface/80 backdrop-blur-xl border-b border-outline-variant/10">
          <h1 className="text-lg font-medium">{pageTitle}</h1>
          <div className="flex items-center gap-4 text-sm text-on-surface-variant">
            <span className="flex items-center gap-1.5">
              <span className="status-dot bg-emerald-500 animate-pulse" />
              Live
            </span>
          </div>
        </header>

        {/* Content */}
        <main className="flex-1 overflow-y-auto p-6 space-y-6">{children}</main>
      </div>
    </div>
  )
}
