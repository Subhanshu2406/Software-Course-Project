const BASE = import.meta.env.VITE_API_BASE || ''

async function request(url, opts = {}) {
  const headers = { ...opts.headers }
  const token = localStorage.getItem('ledger_token')
  if (token) headers['Authorization'] = `Bearer ${token}`
  if (opts.body && !headers['Content-Type']) headers['Content-Type'] = 'application/json'

  const fullUrl = `${BASE}${url}`
  try {
    const res = await fetch(fullUrl, { ...opts, headers })
    if (!res.ok) {
      const text = await res.text()
      const msg = `${res.status}: ${text || res.statusText}`
      console.error(`[API ${opts.method || 'GET'} ${url}] Failed:`, msg)
      throw new Error(msg)
    }
    const ct = res.headers.get('content-type') || ''
    return ct.includes('application/json') ? res.json() : res.text()
  } catch (err) {
    // Network error or other fetch failure
    if (err instanceof TypeError) {
      console.error(`[API ${opts.method || 'GET'} ${url}] Network error (backend may be unavailable):`, err.message)
      throw new Error(`Network error: ${err.message}. Backend service may be unavailable.`)
    }
    throw err
  }
}

const api = {
  // Coordinator
  coordinatorHealth: () => request('/coordinator/health'),
  coordinatorMetrics: () => request('/coordinator/metrics'),
  coordinatorTransactions: (limit = 50) => request(`/coordinator/transactions?limit=${limit}`),
  submitTransfer: (body) => request('/coordinator/transfer', { method: 'POST', body: JSON.stringify(body) }),
  coordinatorPrometheus: () => request('/coordinator/metrics/prometheus'),

  // Shards
  shardHealth: (id) => request(`/${id}/health`),
  shardMetrics: (id) => request(`/${id}/metrics`),
  shardTransactions: (id, limit = 25) => request(`/${id}/transactions?limit=${limit}`),
  shardWAL: (id, limit = 50) => request(`/${id}/wal?limit=${limit}`),
  shardPrometheus: (id) => request(`/${id}/metrics/prometheus`),

  // Load Monitor
  loadMonitorHealth: () => request('/load-monitor/health'),
  loadMonitorMetrics: () => request('/load-monitor/metrics'),
  loadMonitorMigrations: () => request('/load-monitor/migrations'),
  loadMonitorShardMap: () => request('/load-monitor/shard-map'),

  // Fault Proxy
  faultStatus: () => request('/fault-proxy/status'),
  faultKill: (container) => request(`/fault-proxy/kill?container=${encodeURIComponent(container)}`, { method: 'POST' }),
  faultRestart: (container) => request(`/fault-proxy/restart?container=${encodeURIComponent(container)}`, { method: 'POST' }),
}

export default api
