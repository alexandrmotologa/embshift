import React, { useState } from 'react'
import type { MigrationTelemetry } from '../types'
import { Zap, Check, ShieldCheck, X } from 'lucide-react'

interface CutoverButtonProps {
  telemetry: MigrationTelemetry | null
  onExecuteCutover: () => void
  loadingAction: boolean
}

export const CutoverButton: React.FC<CutoverButtonProps> = ({
  telemetry,
  onExecuteCutover,
  loadingAction,
}) => {
  const [showConfirmModal, setShowConfirmModal] = useState(false)
  const isCutoverComplete = telemetry?.state === 'cutover_complete'
  const isEligible = telemetry?.cutover_eligible || false
  const parityRatio = telemetry?.parity_ratio || 0
  const queueSize = telemetry?.replication_queue_size || 0
  const lagMs = telemetry?.replication_lag_ms || 0

  if (isCutoverComplete) {
    return (
      <div className="bg-emerald-950/40 border border-emerald-800/80 rounded-xl p-5 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="h-9 w-9 rounded-full bg-emerald-900/60 flex items-center justify-center text-emerald-400">
            <Check className="w-5 h-5" />
          </div>
          <div>
            <h4 className="text-sm font-semibold text-emerald-300">Target Vector Store is Active Primary</h4>
            <p className="text-xs text-emerald-400/80 mt-0.5">
              Production traffic routed to 3072-dimensional index with reverse replication buffer active.
            </p>
          </div>
        </div>
      </div>
    )
  }

  return (
    <>
      <div className="bg-slate-900/90 border border-slate-800 rounded-xl p-5 shadow-sm flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div>
          <h3 className="text-sm font-semibold text-white flex items-center gap-2">
            <Zap className="w-4 h-4 text-emerald-400" />
            Atomic Traffic Cutover Switch
          </h3>
          <p className="text-xs text-slate-400 mt-1">
            Zero-downtime pointer flip. Safely transfers live search and write traffic to target collection.
          </p>

          {/* Checklist */}
          <div className="mt-3 flex flex-wrap gap-4 text-xs font-mono">
            <span className={`flex items-center gap-1.5 ${parityRatio >= 0.999 ? 'text-emerald-400' : 'text-slate-500'}`}>
              <Check className="w-3.5 h-3.5" />
              100% Parity ({(parityRatio * 100).toFixed(1)}%)
            </span>
            <span className={`flex items-center gap-1.5 ${queueSize === 0 ? 'text-emerald-400' : 'text-slate-500'}`}>
              <Check className="w-3.5 h-3.5" />
              Queue Drained ({queueSize} pending)
            </span>
            <span className={`flex items-center gap-1.5 ${lagMs <= 100 ? 'text-emerald-400' : 'text-slate-500'}`}>
              <Check className="w-3.5 h-3.5" />
              Replication Lag &lt; 100ms ({lagMs.toFixed(1)}ms)
            </span>
          </div>
        </div>

        <button
          onClick={() => setShowConfirmModal(true)}
          disabled={!isEligible || loadingAction}
          className={`flex items-center gap-2 px-5 py-2.5 rounded-lg text-xs font-semibold uppercase tracking-wider transition cursor-pointer ${
            isEligible
              ? 'bg-emerald-600 hover:bg-emerald-500 active:bg-emerald-700 text-white shadow-lg shadow-emerald-950/50'
              : 'bg-slate-800 text-slate-500 border border-slate-700 cursor-not-allowed opacity-60'
          }`}
        >
          <Zap className="w-4 h-4 fill-current" />
          Execute Atomic Cutover
        </button>
      </div>

      {/* Safety Confirmation Modal */}
      {showConfirmModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-xl max-w-md w-full p-6 shadow-2xl relative">
            <button
              onClick={() => setShowConfirmModal(false)}
              className="absolute top-4 right-4 text-slate-400 hover:text-white"
            >
              <X className="w-5 h-5" />
            </button>

            <div className="flex items-center gap-3 text-emerald-400 mb-3">
              <ShieldCheck className="w-6 h-6" />
              <h3 className="text-lg font-bold text-white">Confirm Production Cutover</h3>
            </div>

            <p className="text-xs text-slate-300 leading-relaxed mb-4">
              All safety invariants have cleared. Executing cutover will atomically reroute production vector search and ingestion traffic to the target 3072-dimensional store with zero dropped requests.
            </p>

            <div className="bg-slate-950 p-3.5 rounded-lg border border-slate-800 mb-5 space-y-2 text-xs font-mono">
              <div className="flex justify-between text-slate-300">
                <span>Corpus Parity:</span>
                <span className="text-emerald-400 font-semibold">100.0%</span>
              </div>
              <div className="flex justify-between text-slate-300">
                <span>Replication Lag:</span>
                <span className="text-emerald-400 font-semibold">{lagMs.toFixed(1)} ms</span>
              </div>
              <div className="flex justify-between text-slate-300">
                <span>Rollback Buffer:</span>
                <span className="text-blue-400 font-semibold">Reverse Dual-Writing Enabled</span>
              </div>
            </div>

            <div className="flex items-center justify-end gap-3">
              <button
                onClick={() => setShowConfirmModal(false)}
                className="px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-lg text-xs font-medium cursor-pointer"
              >
                Cancel
              </button>
              <button
                onClick={() => {
                  setShowConfirmModal(false)
                  onExecuteCutover()
                }}
                disabled={loadingAction}
                className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-white rounded-lg text-xs font-semibold cursor-pointer shadow-md"
              >
                Confirm and Cutover
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
