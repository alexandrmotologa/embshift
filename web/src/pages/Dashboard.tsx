import React, { useState, useEffect, useCallback } from 'react'
import type { MigrationTelemetry, AuditManifest, DLQItem } from '../types'
import { Header } from '../components/Header'
import { MigrationProgressBar } from '../components/MigrationProgressBar'
import { DualWriteLagGauge } from '../components/DualWriteLagGauge'
import { SpendTracker } from '../components/SpendTracker'
import { ShadowQueryView } from '../components/ShadowQueryView'
import { CanarySlider } from '../components/CanarySlider'
import { DLQInspector } from '../components/DLQInspector'
import { CutoverButton } from '../components/CutoverButton'
import { QuerySimulator } from '../components/QuerySimulator'
import { AuditModal } from '../components/AuditModal'

export const Dashboard: React.FC = () => {
  const [telemetry, setTelemetry] = useState<MigrationTelemetry | null>(null)
  const [auditManifest, setAuditManifest] = useState<AuditManifest | null>(null)
  const [dlqItems, setDlqItems] = useState<DLQItem[]>([])
  const [showAuditModal, setShowAuditModal] = useState(false)
  const [loadingAction, setLoadingAction] = useState(false)
  const [feedbackMessage, setFeedbackMessage] = useState<string | null>(null)

  const showToast = (msg: string) => {
    setFeedbackMessage(msg)
    setTimeout(() => setFeedbackMessage(null), 3500)
  }

  // Fetch telemetry
  const fetchStatus = useCallback(async () => {
    try {
      const res = await fetch('/api/status')
      if (res.ok) {
        const data: MigrationTelemetry = await res.json()
        setTelemetry(data)
      }
    } catch {
      // Ignore network errors during initial load
    }
  }, [])

  // Fetch DLQ items
  const fetchDLQ = useCallback(async () => {
    try {
      const res = await fetch('/api/dlq')
      if (res.ok) {
        const data = await res.json()
        setDlqItems(data.items || [])
      }
    } catch {
      // Ignore
    }
  }, [])

  useEffect(() => {
    fetchStatus()
    fetchDLQ()

    const eventSource = new EventSource('/api/events')

    eventSource.addEventListener('telemetry', (e) => {
      try {
        const data: MigrationTelemetry = JSON.parse(e.data)
        setTelemetry(data)
      } catch (err) {
        console.error('Failed to parse SSE telemetry:', err)
      }
    })

    eventSource.addEventListener('cutover', (e) => {
      try {
        const manifest: AuditManifest = JSON.parse(e.data)
        setAuditManifest(manifest)
        setShowAuditModal(true)
        fetchStatus()
      } catch (err) {
        console.error('Failed to parse cutover event:', err)
      }
    })

    eventSource.addEventListener('canary', () => {
      fetchStatus()
    })

    eventSource.addEventListener('rollback', () => {
      fetchStatus()
    })

    return () => {
      eventSource.close()
    }
  }, [fetchStatus, fetchDLQ])

  const handleStartBackfill = async () => {
    setLoadingAction(true)
    try {
      const res = await fetch('/api/backfill/start', { method: 'POST' })
      if (res.ok) {
        showToast('Background backfill initiated')
        fetchStatus()
      } else {
        const err = await res.text()
        showToast(`Error: ${err}`)
      }
    } finally {
      setLoadingAction(false)
    }
  }

  const handlePauseBackfill = async () => {
    setLoadingAction(true)
    try {
      const res = await fetch('/api/backfill/pause', { method: 'POST' })
      if (res.ok) {
        showToast('Backfill paused')
        fetchStatus()
      }
    } finally {
      setLoadingAction(false)
    }
  }

  const handleResumeBackfill = async () => {
    setLoadingAction(true)
    try {
      const res = await fetch('/api/backfill/resume', { method: 'POST' })
      if (res.ok) {
        showToast('Backfill resumed')
        fetchStatus()
      }
    } finally {
      setLoadingAction(false)
    }
  }

  const handleSetCanaryWeight = async (weight: number) => {
    setLoadingAction(true)
    try {
      const res = await fetch('/api/canary/set', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ weight }),
      })
      if (res.ok) {
        showToast(`Canary traffic set to ${Math.round(weight * 100)}%`)
        fetchStatus()
      } else {
        const err = await res.text()
        showToast(`Canary error: ${err}`)
      }
    } finally {
      setLoadingAction(false)
    }
  }

  const handleAbortCanary = async () => {
    setLoadingAction(true)
    try {
      const res = await fetch('/api/canary/abort', { method: 'POST' })
      if (res.ok) {
        showToast('Canary traffic aborted. Read traffic reverted to 100% legacy.')
        fetchStatus()
      }
    } finally {
      setLoadingAction(false)
    }
  }

  const handleExecuteCutover = async () => {
    setLoadingAction(true)
    try {
      const res = await fetch('/api/cutover', { method: 'POST' })
      if (res.ok) {
        const manifest: AuditManifest = await res.json()
        setAuditManifest(manifest)
        setShowAuditModal(true)
        showToast('Atomic cutover executed with zero downtime')
        fetchStatus()
      } else {
        const err = await res.text()
        showToast(`Cutover failed: ${err}`)
      }
    } finally {
      setLoadingAction(false)
    }
  }

  const handleRollback = async () => {
    setLoadingAction(true)
    try {
      const res = await fetch('/api/rollback', { method: 'POST' })
      if (res.ok) {
        showToast('Production traffic reverted to legacy store')
        fetchStatus()
      } else {
        const err = await res.text()
        showToast(`Rollback failed: ${err}`)
      }
    } finally {
      setLoadingAction(false)
    }
  }

  const handleInjectSynthetic = async () => {
    setLoadingAction(true)
    try {
      const res = await fetch('/api/simulate/insert', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ count: 10 }),
      })
      if (res.ok) {
        showToast('Injected 10 live document writes')
        fetchStatus()
      }
    } finally {
      setLoadingAction(false)
    }
  }

  const handleSimulateQuery = async (text: string) => {
    setLoadingAction(true)
    try {
      const res = await fetch('/api/simulate/query', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ text, top_k: 5 }),
      })
      if (res.ok) {
        showToast(`Query "${text}" mirrored to shadow evaluator`)
        setTimeout(fetchStatus, 300)
      }
    } finally {
      setLoadingAction(false)
    }
  }

  const handleRetryDLQ = async (id: number) => {
    setLoadingAction(true)
    try {
      const res = await fetch('/api/dlq/retry', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id }),
      })
      if (res.ok) {
        showToast('Record re-enqueued for migration')
        fetchDLQ()
        fetchStatus()
      }
    } finally {
      setLoadingAction(false)
    }
  }

  const handlePurgeDLQ = async (id: number) => {
    setLoadingAction(true)
    try {
      const res = await fetch('/api/dlq/purge', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id }),
      })
      if (res.ok) {
        showToast('Record purged from DLQ')
        fetchDLQ()
        fetchStatus()
      }
    } finally {
      setLoadingAction(false)
    }
  }

  const handleOpenAudit = async () => {
    try {
      const res = await fetch('/api/audit')
      if (res.ok) {
        const manifest: AuditManifest = await res.json()
        setAuditManifest(manifest)
        setShowAuditModal(true)
      } else {
        showToast('No cutover audit manifest available yet')
      }
    } catch {
      showToast('Failed to retrieve audit manifest')
    }
  }

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100 flex flex-col font-sans">
      <Header
        telemetry={telemetry}
        onStartBackfill={handleStartBackfill}
        onPauseBackfill={handlePauseBackfill}
        onResumeBackfill={handleResumeBackfill}
        onRollback={handleRollback}
        onInjectSynthetic={handleInjectSynthetic}
        onOpenAudit={handleOpenAudit}
        loadingAction={loadingAction}
      />

      {feedbackMessage && (
        <div className="fixed bottom-6 right-6 z-50 px-4 py-2.5 rounded-lg bg-blue-600 text-white text-xs font-medium shadow-xl border border-blue-400/40 animate-fade-in">
          {feedbackMessage}
        </div>
      )}

      <main className="flex-1 max-w-7xl w-full mx-auto p-6 space-y-6">
        {/* Row 1: Backfill Progress, Dual-Write Lag, and Spend Meter */}
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          <div className="lg:col-span-2">
            <MigrationProgressBar telemetry={telemetry} />
          </div>
          <div>
            <DualWriteLagGauge telemetry={telemetry} />
          </div>
        </div>

        {/* Row 2: Spend Tracker & Cutover Switch */}
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          <div>
            <SpendTracker telemetry={telemetry} />
          </div>
          <div className="lg:col-span-2">
            <CutoverButton
              telemetry={telemetry}
              onExecuteCutover={handleExecuteCutover}
              loadingAction={loadingAction}
            />
          </div>
        </div>

        {/* Row 3: Canary Phased Routing */}
        <div>
          <CanarySlider
            canaryWeight={telemetry?.canary_weight || 0}
            onSetCanaryWeight={handleSetCanaryWeight}
            onAbortCanary={handleAbortCanary}
            disabled={loadingAction || telemetry?.state === 'cutover_complete'}
          />
        </div>

        {/* Row 4: Shadow Query Evaluation with Semantic Drift Filter */}
        <div>
          <ShadowQueryView telemetry={telemetry} />
        </div>

        {/* Row 5: Dead-Letter Queue (DLQ) Inspector */}
        <div>
          <DLQInspector
            dlqCount={telemetry?.dlq_count || 0}
            dlqItems={dlqItems}
            onRetryItem={handleRetryDLQ}
            onPurgeItem={handlePurgeDLQ}
            onRefresh={fetchDLQ}
            loading={loadingAction}
          />
        </div>

        {/* Row 6: Live Query Simulator */}
        <div>
          <QuerySimulator
            onSimulateQuery={handleSimulateQuery}
            loading={loadingAction}
          />
        </div>
      </main>

      <footer className="border-t border-slate-900 py-6 px-6 text-center text-xs text-slate-500 font-mono">
        embshift v1.0.0 &bull; Zero-Downtime Vector Store Migration Engine &bull; Prometheus Metrics: /metrics &bull; Go Runtime &lt; 20MB RAM
      </footer>

      {showAuditModal && (
        <AuditModal
          manifest={auditManifest}
          onClose={() => setShowAuditModal(false)}
        />
      )}
    </div>
  )
}
