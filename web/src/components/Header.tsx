import React from 'react'
import type { MigrationTelemetry } from '../types'
import { 
  Play, 
  Pause, 
  Database, 
  Zap, 
  RotateCcw,
  ShieldCheck,
  Activity
} from 'lucide-react'

interface HeaderProps {
  telemetry: MigrationTelemetry | null
  onStartBackfill: () => void
  onPauseBackfill: () => void
  onResumeBackfill: () => void
  onRollback: () => void
  onInjectSynthetic: () => void
  onOpenAudit: () => void
  loadingAction: boolean
}

export const Header: React.FC<HeaderProps> = ({
  telemetry,
  onStartBackfill,
  onPauseBackfill,
  onResumeBackfill,
  onRollback,
  onInjectSynthetic,
  onOpenAudit,
  loadingAction,
}) => {
  const state = telemetry?.state || 'idle'
  const isBackfilling = state === 'backfilling'
  const isPaused = state === 'paused'
  const isCutoverComplete = state === 'cutover_complete'
  const activePrimary = telemetry?.active_primary || 'legacy'

  const getStateBadge = () => {
    switch (state) {
      case 'idle':
        return <span className="px-2.5 py-1 rounded-full text-xs font-semibold bg-slate-800 text-slate-300 border border-slate-700">Idle</span>
      case 'backfilling':
        return <span className="px-2.5 py-1 rounded-full text-xs font-semibold bg-blue-900/60 text-blue-300 border border-blue-700 animate-pulse flex items-center gap-1.5"><Activity className="w-3 h-3 animate-spin" /> Backfilling</span>
      case 'paused':
        return <span className="px-2.5 py-1 rounded-full text-xs font-semibold bg-amber-900/60 text-amber-300 border border-amber-700">Paused</span>
      case 'shadow_testing':
        return <span className="px-2.5 py-1 rounded-full text-xs font-semibold bg-purple-900/60 text-purple-300 border border-purple-700">Shadow Testing</span>
      case 'ready_for_cutover':
        return <span className="px-2.5 py-1 rounded-full text-xs font-semibold bg-emerald-900/60 text-emerald-300 border border-emerald-600 flex items-center gap-1"><ShieldCheck className="w-3 h-3" /> Ready for Cutover</span>
      case 'cutover_complete':
        return <span className="px-2.5 py-1 rounded-full text-xs font-semibold bg-emerald-500/20 text-emerald-400 border border-emerald-500 flex items-center gap-1"><Zap className="w-3 h-3" /> Cutover Active</span>
      case 'rolled_back':
        return <span className="px-2.5 py-1 rounded-full text-xs font-semibold bg-rose-900/60 text-rose-300 border border-rose-700">Rolled Back</span>
      default:
        return null
    }
  }

  return (
    <header className="border-b border-slate-800 bg-slate-950/80 backdrop-blur sticky top-0 z-40 px-6 py-4">
      <div className="max-w-7xl mx-auto flex flex-col md:flex-row md:items-center md:justify-between gap-4">
        {/* Left: Brand & Status */}
        <div className="flex items-center gap-4">
          <div className="flex items-center gap-2.5">
            <div className="h-9 w-9 rounded-lg bg-blue-600/20 border border-blue-500/40 flex items-center justify-center text-blue-400 font-mono font-bold text-lg shadow-inner">
              es
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h1 className="text-xl font-bold tracking-tight text-white font-mono">embshift</h1>
                <span className="text-xs px-2 py-0.5 rounded bg-slate-800 text-slate-400 border border-slate-700 font-mono">v1.0.0</span>
              </div>
              <p className="text-xs text-slate-400">Zero-Downtime Vector Re-Embedding Engine</p>
            </div>
          </div>

          <div className="h-6 w-[1px] bg-slate-800 hidden md:block" />

          <div className="flex items-center gap-2">
            {getStateBadge()}
            
            <div className="flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-mono bg-slate-900 border border-slate-800">
              <span className="relative flex h-2 w-2">
                <span className={`animate-ping absolute inline-flex h-full w-full rounded-full opacity-75 ${activePrimary === 'target' ? 'bg-emerald-400' : 'bg-blue-400'}`}></span>
                <span className={`relative inline-flex rounded-full h-2 w-2 ${activePrimary === 'target' ? 'bg-emerald-500' : 'bg-blue-500'}`}></span>
              </span>
              <span className="text-slate-400">Primary:</span>
              <span className={`font-semibold uppercase tracking-wider ${activePrimary === 'target' ? 'text-emerald-400' : 'text-blue-400'}`}>
                {activePrimary}
              </span>
            </div>
          </div>
        </div>

        {/* Right: Controls */}
        <div className="flex items-center flex-wrap gap-2">
          {/* Backfill controls */}
          {state === 'idle' && (
            <button
              onClick={onStartBackfill}
              disabled={loadingAction}
              className="flex items-center gap-1.5 px-3.5 py-1.5 bg-blue-600 hover:bg-blue-500 active:bg-blue-700 text-white rounded-lg text-xs font-medium transition cursor-pointer disabled:opacity-50"
            >
              <Play className="w-3.5 h-3.5 fill-current" />
              Start Backfill
            </button>
          )}

          {isBackfilling && (
            <button
              onClick={onPauseBackfill}
              disabled={loadingAction}
              className="flex items-center gap-1.5 px-3.5 py-1.5 bg-amber-600 hover:bg-amber-500 text-white rounded-lg text-xs font-medium transition cursor-pointer disabled:opacity-50"
            >
              <Pause className="w-3.5 h-3.5 fill-current" />
              Pause Backfill
            </button>
          )}

          {isPaused && (
            <button
              onClick={onResumeBackfill}
              disabled={loadingAction}
              className="flex items-center gap-1.5 px-3.5 py-1.5 bg-emerald-600 hover:bg-emerald-500 text-white rounded-lg text-xs font-medium transition cursor-pointer disabled:opacity-50"
            >
              <Play className="w-3.5 h-3.5 fill-current" />
              Resume Backfill
            </button>
          )}

          {/* Synthetic data inject */}
          <button
            onClick={onInjectSynthetic}
            disabled={loadingAction}
            className="flex items-center gap-1.5 px-3.5 py-1.5 bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 rounded-lg text-xs font-medium transition cursor-pointer disabled:opacity-50"
            title="Inject simulated document writes to observe dual-writing"
          >
            <Database className="w-3.5 h-3.5 text-blue-400" />
            +10 Live Writes
          </button>

          {/* Rollback button if cutover completed */}
          {isCutoverComplete && (
            <button
              onClick={onRollback}
              disabled={loadingAction}
              className="flex items-center gap-1.5 px-3.5 py-1.5 bg-rose-950 hover:bg-rose-900 border border-rose-700 text-rose-300 rounded-lg text-xs font-medium transition cursor-pointer disabled:opacity-50"
            >
              <RotateCcw className="w-3.5 h-3.5" />
              Rollback to Legacy
            </button>
          )}

          {/* Audit manifest viewer */}
          {(isCutoverComplete || telemetry?.cutover_at) && (
            <button
              onClick={onOpenAudit}
              className="flex items-center gap-1.5 px-3.5 py-1.5 bg-slate-900 hover:bg-slate-800 border border-slate-700 text-slate-300 rounded-lg text-xs font-medium transition cursor-pointer"
            >
              <ShieldCheck className="w-3.5 h-3.5 text-emerald-400" />
              Audit Manifest
            </button>
          )}
        </div>
      </div>
    </header>
  )
}
