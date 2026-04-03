import { useState } from 'react'
import api from '../api/client'

export default function SubmitTransfer() {
  const [form, setForm] = useState({ source: '', destination: '', amount: '' })
  const [result, setResult] = useState(null)
  const [error, setError] = useState(null)
  const [submitting, setSubmitting] = useState(false)

  const handleSubmit = async (e) => {
    e.preventDefault()
    setError(null)
    setResult(null)
    setSubmitting(true)
    try {
      const txnId = `txn-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
      const res = await api.submitTransfer({
        txn_id: txnId,
        source: form.source,
        destination: form.destination,
        amount: Number(form.amount),
      })
      setResult(res)
    } catch (err) {
      setError(err.message)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <>
      <p className="text-sm text-on-surface-variant">Submit a synchronous ledger transfer between accounts.</p>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Form */}
        <div className="card">
          <h2 className="label-sm mb-4">New Transfer</h2>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label className="text-xs text-on-surface-variant block mb-1">Source Account</label>
              <input
                className="w-full bg-surface-container rounded-lg px-3 py-2.5 text-sm font-mono text-on-surface placeholder:text-on-surface-variant/50 outline-none focus:ring-1 focus:ring-primary/30"
                placeholder="e.g. user0"
                value={form.source}
                onChange={(e) => setForm((f) => ({ ...f, source: e.target.value }))}
                required
              />
            </div>
            <div>
              <label className="text-xs text-on-surface-variant block mb-1">Destination Account</label>
              <input
                className="w-full bg-surface-container rounded-lg px-3 py-2.5 text-sm font-mono text-on-surface placeholder:text-on-surface-variant/50 outline-none focus:ring-1 focus:ring-primary/30"
                placeholder="e.g. user1"
                value={form.destination}
                onChange={(e) => setForm((f) => ({ ...f, destination: e.target.value }))}
                required
              />
            </div>
            <div>
              <label className="text-xs text-on-surface-variant block mb-1">Amount</label>
              <input
                type="number"
                min="1"
                step="1"
                className="w-full bg-surface-container rounded-lg px-3 py-2.5 text-sm font-mono text-on-surface placeholder:text-on-surface-variant/50 outline-none focus:ring-1 focus:ring-primary/30"
                placeholder="100"
                value={form.amount}
                onChange={(e) => setForm((f) => ({ ...f, amount: e.target.value }))}
                required
              />
            </div>
            <button type="submit" disabled={submitting} className="btn-primary w-full disabled:opacity-50">
              {submitting ? 'Submitting...' : 'Submit Transfer'}
            </button>
          </form>
        </div>

        {/* Result */}
        <div className="space-y-4">
          {error && (
            <div className="card border-l-4 border-error">
              <div className="label-sm text-error mb-2">Error</div>
              <pre className="text-xs font-mono text-error whitespace-pre-wrap">{error}</pre>
            </div>
          )}
          {result && (
            <div className="card border-l-4 border-emerald-500">
              <div className="label-sm text-emerald-400 mb-2">Transfer Result</div>
              <pre className="text-xs font-mono text-on-surface whitespace-pre-wrap">
                {JSON.stringify(result, null, 2)}
              </pre>
            </div>
          )}
          {!error && !result && (
            <div className="card text-center py-12 text-on-surface-variant text-sm">
              Submit a transfer to see the result here
            </div>
          )}
        </div>
      </div>
    </>
  )
}
