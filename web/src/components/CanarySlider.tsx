import React from 'react'
import { GitBranch, ShieldAlert, Check } from 'lucide-react'

interface CanarySliderProps {
  canaryWeight: number
  onSetCanaryWeight: (weight: number) => void
  onAbortCanary: () => void
  disabled: boolean
}

export const CanarySlider: React.FC<CanarySliderProps> = ({
  canaryWeight,
  onSetCanaryWeight,
  onAbortCanary,
  disabled,
}) => {
  const percentage = Math.round(canaryWeight * 100)
  const isCanaryActive = canaryWeight > 0

  const presets = [0, 0.1, 0.25, 0.5, 1.0]

  return (
    <div className="bg-slate-900/90 border border-slate-800 rounded-xl p-5 shadow-sm">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-3">
        <div className="flex items-center gap-2">
          <GitBranch className="w-4 h-4 text-purple-400" />
          <h3 className="text-sm font-semibold text-white">Canary Phased Routing</h3>
          {isCanaryActive && (
            <span className="px-2 py-0.5 rounded text-[11px] font-mono bg-purple-950 text-purple-300 border border-purple-800 animate-pulse">
              {percentage}% Live Split
            </span>
          )}
        </div>

        {isCanaryActive && (
          <button
            onClick={onAbortCanary}
            disabled={disabled}
            className="flex items-center gap-1.5 px-2.5 py-1 rounded bg-rose-950/80 hover:bg-rose-900 text-rose-300 border border-rose-700 text-xs font-medium cursor-pointer transition"
          >
            <ShieldAlert className="w-3.5 h-3.5" />
            Abort Canary (0%)
          </button>
        )}
      </div>

      <p className="text-xs text-slate-400 mb-4">
        Gradually route a fraction of production user search queries to the target collection before committing full cutover.
      </p>

      {/* Slider & Presets */}
      <div className="space-y-3">
        <div className="flex items-center gap-3">
          <input
            type="range"
            min="0"
            max="1"
            step="0.05"
            value={canaryWeight}
            disabled={disabled}
            onChange={(e) => onSetCanaryWeight(parseFloat(e.target.value))}
            className="w-full accent-purple-500 cursor-pointer disabled:opacity-50"
          />
          <span className="font-mono text-sm font-bold text-white w-12 text-right">
            {percentage}%
          </span>
        </div>

        <div className="flex items-center justify-between pt-1">
          <div className="flex items-center gap-1.5">
            <span className="text-[11px] text-slate-500 font-mono">Presets:</span>
            {presets.map((val) => {
              const pct = Math.round(val * 100)
              const isActive = Math.abs(canaryWeight - val) < 0.01
              return (
                <button
                  key={val}
                  type="button"
                  disabled={disabled}
                  onClick={() => onSetCanaryWeight(val)}
                  className={`px-2 py-0.5 rounded text-[11px] font-mono transition cursor-pointer ${
                    isActive
                      ? 'bg-purple-600 text-white font-semibold'
                      : 'bg-slate-950 text-slate-400 hover:text-white border border-slate-800'
                  }`}
                >
                  {pct}%
                </button>
              )
            })}
          </div>

          <div className="text-[11px] font-mono text-slate-400 flex items-center gap-1">
            <Check className="w-3 h-3 text-emerald-400" />
            Auto-Abort on &gt; 5% errors
          </div>
        </div>
      </div>
    </div>
  )
}
