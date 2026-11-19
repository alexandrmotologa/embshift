import React, { useState } from 'react'
import type { MigrationTelemetry } from '../types'
import { Eye, ChevronRight, ChevronDown, Clock, Filter, AlertTriangle } from 'lucide-react'

interface ShadowQueryViewProps {
  telemetry: MigrationTelemetry | null
}

type DriftFilter = 'all' | 'high_drift' | 'moderate' | 'strong'

export const ShadowQueryView: React.FC<ShadowQueryViewProps> = ({ telemetry }) => {
  const recentQueries = telemetry?.recent_shadow_queries || []
  const avgJaccard = telemetry?.average_jaccard || 0.0
  const evaluatedCount = telemetry?.evaluated_queries || 0
  const [expandedId, setExpandedId] = useState<string | null>(null)
  const [filter, setFilter] = useState<DriftFilter>('all')

  const toggleExpand = (id: string) => {
    setExpandedId(prev => (prev === id ? null : id))
  }

  // Filter queries based on semantic drift
  const filteredQueries = recentQueries.filter((comp) => {
    const j = comp.jaccard_similarity
    if (filter === 'high_drift') return j < 0.35
    if (filter === 'moderate') return j >= 0.35 && j < 0.70
    if (filter === 'strong') return j >= 0.70
    return true
  })

  return (
    <div className="bg-slate-900/90 border border-slate-800 rounded-xl p-5 shadow-sm">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-4">
        <div>
          <div className="flex items-center gap-2">
            <Eye className="w-4 h-4 text-purple-400" />
            <h3 className="text-base font-semibold text-white">Shadow Query Evaluation & Semantic Drift</h3>
          </div>
          <p className="text-xs text-slate-400 mt-1">
            Mirrored production search ranking comparison, NDCG gain, and semantic drift detection
          </p>
        </div>

        <div className="flex items-center gap-4 text-xs font-mono">
          <div className="flex items-center gap-1.5 text-slate-300">
            <span>Evaluated: {evaluatedCount} queries</span>
          </div>
          <div className="flex items-center gap-1.5 px-2.5 py-1 rounded bg-purple-950/70 border border-purple-800/80 text-purple-300">
            <span>Rolling Jaccard:</span>
            <span className="font-bold text-white">{(avgJaccard * 100).toFixed(1)}%</span>
          </div>
        </div>
      </div>

      {/* Drift Filter Tabs */}
      <div className="flex items-center gap-2 mb-3 pb-2 border-b border-slate-800/80 text-xs">
        <span className="text-slate-500 font-mono flex items-center gap-1 text-[11px]">
          <Filter className="w-3 h-3" />
          Filter:
        </span>
        <button
          type="button"
          onClick={() => setFilter('all')}
          className={`px-2.5 py-0.5 rounded text-[11px] font-mono transition cursor-pointer ${
            filter === 'all'
              ? 'bg-slate-800 text-white font-semibold'
              : 'text-slate-400 hover:text-white'
          }`}
        >
          All ({recentQueries.length})
        </button>
        <button
          type="button"
          onClick={() => setFilter('high_drift')}
          className={`px-2.5 py-0.5 rounded text-[11px] font-mono transition cursor-pointer flex items-center gap-1 ${
            filter === 'high_drift'
              ? 'bg-rose-950 text-rose-300 border border-rose-800 font-semibold'
              : 'text-rose-400/80 hover:text-rose-300'
          }`}
        >
          <AlertTriangle className="w-3 h-3 text-rose-400" />
          High Drift &lt; 35%
        </button>
        <button
          type="button"
          onClick={() => setFilter('moderate')}
          className={`px-2.5 py-0.5 rounded text-[11px] font-mono transition cursor-pointer ${
            filter === 'moderate'
              ? 'bg-amber-950 text-amber-300 border border-amber-800 font-semibold'
              : 'text-amber-400/80 hover:text-amber-300'
          }`}
        >
          Moderate (35-70%)
        </button>
        <button
          type="button"
          onClick={() => setFilter('strong')}
          className={`px-2.5 py-0.5 rounded text-[11px] font-mono transition cursor-pointer ${
            filter === 'strong'
              ? 'bg-emerald-950 text-emerald-300 border border-emerald-800 font-semibold'
              : 'text-emerald-400/80 hover:text-emerald-300'
          }`}
        >
          Strong &gt; 70%
        </button>
      </div>

      {recentQueries.length === 0 ? (
        <div className="py-10 text-center text-slate-500 text-xs border border-dashed border-slate-800 rounded-lg">
          No mirrored production queries captured yet. Use the Query Simulator below or send requests to :8080/v1/vectors/query.
        </div>
      ) : filteredQueries.length === 0 ? (
        <div className="py-6 text-center text-slate-500 text-xs bg-slate-950/40 rounded-lg border border-slate-800/60 font-mono">
          No mirrored queries matching the selected drift filter.
        </div>
      ) : (
        <div className="space-y-2.5 max-h-[420px] overflow-y-auto pr-1">
          {filteredQueries.map((comp) => {
            const isExpanded = expandedId === comp.query_id
            const jaccardPct = Math.round(comp.jaccard_similarity * 100)
            const ndcgPct = Math.round((comp.ndcg_at_k || 0) * 100)

            return (
              <div
                key={comp.query_id}
                className="bg-slate-950/70 border border-slate-800/80 rounded-lg p-3 hover:border-slate-700 transition"
              >
                <div
                  className="flex items-center justify-between cursor-pointer select-none"
                  onClick={() => toggleExpand(comp.query_id)}
                >
                  <div className="flex items-center gap-2 overflow-hidden">
                    {isExpanded ? (
                      <ChevronDown className="w-4 h-4 text-slate-400 flex-shrink-0" />
                    ) : (
                      <ChevronRight className="w-4 h-4 text-slate-400 flex-shrink-0" />
                    )}
                    <span className="text-xs font-medium text-slate-200 truncate">
                      "{comp.query_text}"
                    </span>
                  </div>

                  <div className="flex items-center gap-3 text-xs font-mono flex-shrink-0">
                    <span className="text-slate-400 flex items-center gap-1">
                      <Clock className="w-3 h-3 text-slate-500" />
                      {comp.legacy_latency_ms.toFixed(1)}ms / {comp.target_latency_ms.toFixed(1)}ms
                    </span>
                    <span className="px-2 py-0.5 rounded text-[11px] font-semibold bg-slate-900 border border-slate-800 text-slate-300">
                      NDCG: {ndcgPct}%
                    </span>
                    <span
                      className={`px-2 py-0.5 rounded text-[11px] font-semibold ${
                        jaccardPct >= 70
                          ? 'bg-emerald-950 text-emerald-300 border border-emerald-800'
                          : jaccardPct >= 35
                          ? 'bg-amber-950 text-amber-300 border border-amber-800'
                          : 'bg-rose-950 text-rose-300 border border-rose-800'
                      }`}
                    >
                      Jaccard: {jaccardPct}%
                    </span>
                  </div>
                </div>

                {/* Expanded side-by-side results */}
                {isExpanded && (
                  <div className="mt-3 pt-3 border-t border-slate-800/80 grid grid-cols-1 md:grid-cols-2 gap-3 text-xs">
                    {/* Legacy Results */}
                    <div className="bg-slate-900/60 p-2.5 rounded border border-slate-800">
                      <div className="font-mono text-[11px] text-blue-400 font-semibold mb-2 flex items-center justify-between">
                        <span>Legacy Store (1536d Ada-002)</span>
                        <span className="text-slate-400">{comp.legacy_latency_ms.toFixed(1)} ms</span>
                      </div>
                      <div className="space-y-1.5">
                        {comp.legacy_results.map((rec, i) => (
                          <div key={rec.id} className="flex items-center justify-between text-slate-300 font-mono text-[11px] bg-slate-950/50 p-1.5 rounded">
                            <span className="truncate max-w-[200px]">{i + 1}. {rec.id}</span>
                            <span className="text-slate-400">score: {rec.score.toFixed(3)}</span>
                          </div>
                        ))}
                      </div>
                    </div>

                    {/* Target Results */}
                    <div className="bg-slate-900/60 p-2.5 rounded border border-slate-800">
                      <div className="font-mono text-[11px] text-purple-400 font-semibold mb-2 flex items-center justify-between">
                        <span>Target Store (3072d 3-Large)</span>
                        <span className="text-slate-400">{comp.target_latency_ms.toFixed(1)} ms</span>
                      </div>
                      <div className="space-y-1.5">
                        {comp.target_results.map((rec, i) => (
                          <div key={rec.id} className="flex items-center justify-between text-slate-300 font-mono text-[11px] bg-slate-950/50 p-1.5 rounded">
                            <span className="truncate max-w-[200px]">{i + 1}. {rec.id}</span>
                            <span className="text-purple-300 font-medium">score: {rec.score.toFixed(3)}</span>
                          </div>
                        ))}
                      </div>
                    </div>
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
