import React from 'react'
import type { MigrationTelemetry } from '../types'
import { Database, CheckCircle2, Clock, Layers } from 'lucide-react'

interface MigrationProgressBarProps {
  telemetry: MigrationTelemetry | null
}

export const MigrationProgressBar: React.FC<MigrationProgressBarProps> = ({ telemetry }) => {
  const legacyCount = telemetry?.legacy_count || 0
  const backfilledCount = telemetry?.backfilled_count || 0
  const parityRatio = telemetry?.parity_ratio || 0.0
  const percentage = Math.round(parityRatio * 1000) / 10

  return (
    <div className="bg-slate-900/90 border border-slate-800 rounded-xl p-5 shadow-sm">
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-3 mb-4">
        <div>
          <div className="flex items-center gap-2">
            <Layers className="w-4 h-4 text-blue-400" />
            <h2 className="text-base font-semibold text-white tracking-tight flex items-center gap-2">
              Re-Embedding Corpus Progress
            </h2>
            <span className="text-xs px-2 py-0.5 rounded bg-blue-950 text-blue-300 border border-blue-800 font-mono">
              1536d → 3072d
            </span>
          </div>
          <p className="text-xs text-slate-400 mt-1">
            Chunked traversal across legacy collection with idempotency checkpoints
          </p>
        </div>

        <div className="flex items-center gap-4 text-xs font-mono">
          <div className="flex items-center gap-1.5 text-slate-300">
            <Database className="w-3.5 h-3.5 text-slate-400" />
            <span>{backfilledCount.toLocaleString()} / {legacyCount.toLocaleString()} docs</span>
          </div>
          <div className="flex items-center gap-1.5 text-emerald-400 font-semibold">
            <CheckCircle2 className="w-3.5 h-3.5" />
            <span>ρ = {(parityRatio * 100).toFixed(1)}%</span>
          </div>
        </div>
      </div>

      {/* Progress Bar Container */}
      <div className="relative w-full h-4 bg-slate-950 rounded-full overflow-hidden border border-slate-800/80 p-0.5">
        <div
          className={`h-full rounded-full transition-all duration-500 ease-out ${
            percentage >= 100
              ? 'bg-gradient-to-r from-emerald-600 to-teal-400'
              : 'bg-gradient-to-r from-blue-600 via-indigo-500 to-purple-500'
          }`}
          style={{ width: `${Math.min(100, Math.max(0, percentage))}%` }}
        />
      </div>

      {/* Sub metrics */}
      <div className="mt-3 flex items-center justify-between text-xs text-slate-400">
        <span className="font-mono">Completion: {percentage.toFixed(1)}%</span>
        <div className="flex items-center gap-3">
          {percentage < 100 && (
            <span className="flex items-center gap-1 text-slate-400 font-mono">
              <Clock className="w-3 h-3 text-slate-500" />
              {percentage > 0 ? 'Backfill active' : 'Awaiting start'}
            </span>
          )}
          {percentage >= 100 && (
            <span className="text-emerald-400 font-semibold font-mono flex items-center gap-1">
              <CheckCircle2 className="w-3.5 h-3.5" />
              100% Corpus Parity Achieved
            </span>
          )}
        </div>
      </div>
    </div>
  )
}
