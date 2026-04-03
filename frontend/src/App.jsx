import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { ClusterProvider } from './context/ClusterContext'
import Layout from './components/Layout'
import ClusterOverview from './pages/ClusterOverview'
import TransactionsExplorer from './pages/TransactionsExplorer'
import WALInspector from './pages/WALInspector'
import ShardMap from './pages/ShardMap'
import Metrics from './pages/Metrics'
import ReplicationHealth from './pages/ReplicationHealth'
import LoadMonitor from './pages/LoadMonitor'
import FaultInjection from './pages/FaultInjection'
import SubmitTransfer from './pages/SubmitTransfer'

export default function App() {
  return (
    <BrowserRouter>
      <ClusterProvider>
        <Layout>
          <Routes>
            <Route path="/" element={<Navigate to="/cluster" replace />} />
            <Route path="/cluster" element={<ClusterOverview />} />
            <Route path="/transactions" element={<TransactionsExplorer />} />
            <Route path="/wal" element={<WALInspector />} />
            <Route path="/shard-map" element={<ShardMap />} />
            <Route path="/metrics" element={<Metrics />} />
            <Route path="/replication" element={<ReplicationHealth />} />
            <Route path="/load-monitor" element={<LoadMonitor />} />
            <Route path="/fault-injection" element={<FaultInjection />} />
            <Route path="/transfer" element={<SubmitTransfer />} />
          </Routes>
        </Layout>
      </ClusterProvider>
    </BrowserRouter>
  )
}
