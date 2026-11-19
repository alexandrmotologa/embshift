import React from 'react'
import type { MigrationTelemetry } from '../types'
import { ArrowRightLeft, Shield, Gauge, Check } from 'lucide-react'

interface DualWriteLagGaugeProps {
  telemetry: MigrationTelemetry | null
}

export const DualWriteLagGauge: React.FC<DualWriteLagGaugeProps> = ({ telemetry }) => {
  const lagMs = telemetry?.replication_lag_ms || 0
  const queueSize = telemetry?.replication_queue_size || 0
  const activePrimary = telemetry?.active_primary || 'legacy'

  const getLagColor = () => {
    if (lagMs <= 25 && queueSize === 0) return 'text-emerald-400'
    if (lagMs <= 100) return 'text-amber-400'
    return 'text-rose-400'
  }

  const getLagBadge = () => {
    if (lagMs <= 25 && queueSize === 0) {
      return <span className="text-emerald-400 font-mono text-xs flex items-center gap-1"><Check className="w-3 h-3" /> Synchronized</span>
    }
    if (lagMs <= 100) {
      return <span className="text-amber-400 font-mono text-xs">Draining queue</span>
    }
    return <span className="text-rose-400 font-mono text-xs">High replication lag</span>
  }

  return (
    <div className="bg-slate-900/90 border border-slate-800 rounded-xl p-5 shadow-sm flex flex-col justify-between">
      <div>
        <div className="flex items-center justify-between mb-3">
          <div className="flex items-center gap-2">
            <Gauge className="w-4 h-4 text-purple-400" />
            <h3 className="text-sm font-semibold text-white">Dual-Write Replication Lag</h3>
          </div>
          {getLagBadge()}
        </div>

        <div className="flex items-baseline gap-3 my-2">
          <span className={`text-3xl font-bold font-mono tracking-tight ${getLagColor()}`}>
            {lagMs.toFixed(1)} <span className="text-sm font-normal text-slate-400">ms</span>
          </span>
          <span className="text-xs text-slate-400 font-mono">
            Δt queue age
          </span>
        </div>

        <div className="mt-3 py-2 px-3 rounded-lg bg-slate-950/70 border border-slate-800/80 flex items-center justify-between text-xs">
          <span className="text-slate-400">Pending buffer events:</span>
          <span className="font-mono font-semibold text-white">
            {queueSize} {queueSize === 1 ? 'event' : 'events'}
          </span>
        </div>
      </div>

      <div className="mt-4 pt-3 border-t border-slate-800/80 flex items-center justify-between text-xs text-slate-400">
        <div className="flex items-center gap-1.5 font-mono">
          <ArrowRightLeft className="w-3.5 h-3.5 text-blue-400" />
          <span>
            {activePrimary === 'legacy' ? 'Primary: Legacy → Target' : 'Primary: Target → Legacy'}
          </span>
        </div>
        <div className="flex items-center gap-1 text-slate-400 font-mono">
          <Shield className="w-3 h-3 text-emerald-400" />
          <span>SQLite WAL</span>
        </div>
      </div>
    </div>
  )
}
