import React, { useState } from 'react'
import { Search, Sparkles, Send } from 'lucide-react'

interface QuerySimulatorProps {
  onSimulateQuery: (text: string) => Promise<void>
  loading: boolean
}

export const QuerySimulator: React.FC<QuerySimulatorProps> = ({ onSimulateQuery, loading }) => {
  const [queryText, setQueryText] = useState('')

  const exampleQueries = [
    'semantic vector database migration',
    'high dimensional cosine similarity',
    'distributed consensus algorithms in Go',
    'zero downtime dual write replication',
  ]

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!queryText.trim() || loading) return
    await onSimulateQuery(queryText.trim())
    setQueryText('')
  }

  const handleSelectExample = (ex: string) => {
    setQueryText(ex)
  }

  return (
    <div className="bg-slate-900/90 border border-slate-800 rounded-xl p-5 shadow-sm">
      <div className="flex items-center gap-2 mb-2">
        <Search className="w-4 h-4 text-blue-400" />
        <h3 className="text-sm font-semibold text-white">Live Search Query Simulator</h3>
      </div>
      <p className="text-xs text-slate-400 mb-3">
        Send an interactive search query to test real-time dual-evaluation and side-by-side rank matching.
      </p>

      <form onSubmit={handleSubmit} className="flex gap-2">
        <input
          type="text"
          value={queryText}
          onChange={(e) => setQueryText(e.target.value)}
          placeholder="Enter search phrase (e.g. distributed vector indexing)..."
          className="flex-1 bg-slate-950 border border-slate-800 rounded-lg px-3.5 py-2 text-xs text-white placeholder-slate-500 focus:outline-none focus:border-blue-500 font-mono"
        />
        <button
          type="submit"
          disabled={!queryText.trim() || loading}
          className="flex items-center gap-1.5 px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white rounded-lg text-xs font-medium cursor-pointer disabled:opacity-50"
        >
          <Send className="w-3.5 h-3.5" />
          Search
        </button>
      </form>

      {/* Quick click suggestions */}
      <div className="mt-3 flex items-center flex-wrap gap-2 text-[11px]">
        <span className="text-slate-500 flex items-center gap-1">
          <Sparkles className="w-3 h-3 text-amber-400" />
          Try:
        </span>
        {exampleQueries.map((ex) => (
          <button
            key={ex}
            type="button"
            onClick={() => handleSelectExample(ex)}
            className="px-2 py-0.5 rounded bg-slate-950 border border-slate-800 text-slate-400 hover:text-slate-200 hover:border-slate-700 font-mono transition cursor-pointer"
          >
            "{ex}"
          </button>
        ))}
      </div>
    </div>
  )
}
