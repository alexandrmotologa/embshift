import React from 'react'
import type { DLQItem } from '../types'
import { AlertOctagon, RotateCw, Trash2, CheckCircle2 } from 'lucide-react'

interface DLQInspectorProps {
  dlqCount: number
  dlqItems: DLQItem[]
  onRetryItem: (id: number) => void
  onPurgeItem: (id: number) => void
  onRefresh: () => void
  loading: boolean
}

export const DLQInspector: React.FC<DLQInspectorProps> = ({
  dlqCount,
  dlqItems,
  onRetryItem,
  onPurgeItem,
  onRefresh,
  loading,
}) => {
  return (
    <div className="bg-slate-900/90 border border-slate-800 rounded-xl p-5 shadow-sm">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-3">
        <div className="flex items-center gap-2">
          <AlertOctagon className={`w-4 h-4 ${dlqCount > 0 ? 'text-rose-400' : 'text-emerald-400'}`} />
          <h3 className="text-sm font-semibold text-white">Dead-Letter Queue (DLQ) Inspector</h3>
          <span
            className={`px-2 py-0.5 rounded text-[11px] font-mono font-semibold ${
              dlqCount > 0
                ? 'bg-rose-950 text-rose-300 border border-rose-800'
                : 'bg-emerald-950 text-emerald-300 border border-emerald-800'
            }`}
          >
            {dlqCount} {dlqCount === 1 ? 'poison record' : 'poison records'}
          </span>
        </div>

        <button
          onClick={onRefresh}
          disabled={loading}
          className="flex items-center gap-1.5 px-2.5 py-1 rounded bg-slate-950 hover:bg-slate-800 border border-slate-800 text-slate-300 text-xs font-mono transition cursor-pointer"
        >
          <RotateCw className={`w-3 h-3 ${loading ? 'animate-spin' : ''}`} />
          Refresh
        </button>
      </div>

      <p className="text-xs text-slate-400 mb-4">
        Failed or malformed document vectors sequestered after exceeding retry thresholds. Inspect errors and retry safely without stalling migrations.
      </p>

      {dlqItems.length === 0 ? (
        <div className="py-6 text-center text-slate-400 text-xs bg-slate-950/60 rounded-lg border border-slate-800/80 flex items-center justify-center gap-2">
          <CheckCircle2 className="w-4 h-4 text-emerald-400" />
          <span>Dead-Letter Queue is empty. No poison-pill records detected.</span>
        </div>
      ) : (
        <div className="space-y-2.5 max-h-[300px] overflow-y-auto pr-1">
          {dlqItems.map((item) => (
            <div
              key={item.id}
              className="bg-slate-950/80 border border-rose-900/40 rounded-lg p-3 text-xs flex flex-col sm:flex-row sm:items-center justify-between gap-3"
            >
              <div className="space-y-1 overflow-hidden">
                <div className="flex items-center gap-2">
                  <span className="font-mono font-bold text-rose-400">{item.record_id}</span>
                  <span className="px-1.5 py-0.2 rounded bg-slate-800 text-slate-400 text-[10px] font-mono">
                    attempts: {item.attempts}
                  </span>
                </div>
                <p className="text-rose-300 text-[11px] font-mono truncate">
                  Error: {item.error_reason}
                </p>
                {item.text && (
                  <p className="text-slate-500 text-[11px] truncate max-w-lg">
                    Text: "{item.text}"
                  </p>
                )}
              </div>

              <div className="flex items-center gap-2 flex-shrink-0">
                <button
                  onClick={() => onRetryItem(item.id)}
                  disabled={loading}
                  className="flex items-center gap-1 px-2.5 py-1 bg-blue-900/60 hover:bg-blue-800 text-blue-200 border border-blue-700 rounded text-[11px] font-medium transition cursor-pointer"
                >
                  <RotateCw className="w-3 h-3" />
                  Retry
                </button>
                <button
                  onClick={() => onPurgeItem(item.id)}
                  disabled={loading}
                  className="flex items-center gap-1 px-2.5 py-1 bg-slate-800 hover:bg-rose-950 text-slate-400 hover:text-rose-300 border border-slate-700 rounded text-[11px] font-medium transition cursor-pointer"
                >
                  <Trash2 className="w-3 h-3" />
                  Purge
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
