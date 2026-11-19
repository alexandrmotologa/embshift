import React from 'react'
import type { MigrationTelemetry } from '../types'
import { DollarSign, Cpu } from 'lucide-react'

interface SpendTrackerProps {
  telemetry: MigrationTelemetry | null
}

export const SpendTracker: React.FC<SpendTrackerProps> = ({ telemetry }) => {
  const tokensConsumed = telemetry?.tokens_consumed || 0
  const spentUSD = telemetry?.estimated_cost_usd || 0.0
  const costLimitUSD = telemetry?.cost_limit_usd || 50.0

  const budgetUsagePercent = costLimitUSD > 0 ? (spentUSD / costLimitUSD) * 100 : 0

  return (
    <div className="bg-slate-900/90 border border-slate-800 rounded-xl p-5 shadow-sm flex flex-col justify-between">
      <div>
        <div className="flex items-center justify-between mb-3">
          <div className="flex items-center gap-2">
            <DollarSign className="w-4 h-4 text-emerald-400" />
            <h3 className="text-sm font-semibold text-white">Spend & Token Budget</h3>
          </div>
          <span className="text-xs font-mono text-slate-400">
            Cap: ${costLimitUSD.toFixed(2)}
          </span>
        </div>

        <div className="flex items-baseline gap-3 my-2">
          <span className="text-3xl font-bold font-mono tracking-tight text-white">
            ${spentUSD.toFixed(3)}
          </span>
          <span className="text-xs text-slate-400 font-mono">
            USD spent
          </span>
        </div>

        {/* Budget Bar */}
        <div className="mt-2 w-full h-2 bg-slate-950 rounded-full overflow-hidden border border-slate-800">
          <div
            className={`h-full rounded-full transition-all duration-300 ${
              budgetUsagePercent > 80 ? 'bg-rose-500' : 'bg-emerald-500'
            }`}
            style={{ width: `${Math.min(100, Math.max(0, budgetUsagePercent))}%` }}
          />
        </div>

        <div className="mt-3 py-2 px-3 rounded-lg bg-slate-950/70 border border-slate-800/80 flex items-center justify-between text-xs">
          <div className="flex items-center gap-1.5 text-slate-400">
            <Cpu className="w-3.5 h-3.5 text-slate-400" />
            <span>Tokens consumed:</span>
          </div>
          <span className="font-mono font-semibold text-white">
            {tokensConsumed.toLocaleString()}
          </span>
        </div>
      </div>

      <div className="mt-4 pt-3 border-t border-slate-800/80 flex items-center justify-between text-xs text-slate-400 font-mono">
        <span>Target: 3072d</span>
        <span>$0.13 / 1M tok</span>
      </div>
    </div>
  )
}
