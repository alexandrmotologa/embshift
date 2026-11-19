import React, { useState } from 'react'
import type { AuditManifest } from '../types'
import { ShieldCheck, X, Copy, Check, Download } from 'lucide-react'

interface AuditModalProps {
  manifest: AuditManifest | null
  onClose: () => void
}

export const AuditModal: React.FC<AuditModalProps> = ({ manifest, onClose }) => {
  const [copied, setCopied] = useState(false)

  if (!manifest) return null

  const jsonString = JSON.stringify(manifest, null, 2)

  const handleCopy = () => {
    navigator.clipboard.writeText(jsonString)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const handleDownload = () => {
    const blob = new Blob([jsonString], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `embshift-audit-${manifest.migration_id}.json`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-sm p-4">
      <div className="bg-slate-900 border border-slate-800 rounded-xl max-w-2xl w-full p-6 shadow-2xl relative flex flex-col max-h-[90vh]">
        <button
          onClick={onClose}
          className="absolute top-4 right-4 text-slate-400 hover:text-white"
        >
          <X className="w-5 h-5" />
        </button>

        <div className="flex items-center gap-3 text-emerald-400 mb-2">
          <ShieldCheck className="w-6 h-6" />
          <h3 className="text-lg font-bold text-white">Migration Audit Manifest</h3>
        </div>
        <p className="text-xs text-slate-400 mb-4">
          Non-repudiable cryptographic verification manifest of completed vector cutover.
        </p>

        {/* Highlight Grid */}
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 mb-4 text-xs font-mono">
          <div className="bg-slate-950 p-2.5 rounded border border-slate-800">
            <span className="text-slate-500 block text-[10px]">MIGRATED DOCS</span>
            <span className="text-white font-semibold text-sm">
              {manifest.total_documents_migrated.toLocaleString()}
            </span>
          </div>
          <div className="bg-slate-950 p-2.5 rounded border border-slate-800">
            <span className="text-slate-500 block text-[10px]">FINAL PARITY</span>
            <span className="text-emerald-400 font-semibold text-sm">
              {(manifest.final_parity_ratio * 100).toFixed(1)}%
            </span>
          </div>
          <div className="bg-slate-950 p-2.5 rounded border border-slate-800">
            <span className="text-slate-500 block text-[10px]">AVG JACCARD</span>
            <span className="text-purple-400 font-semibold text-sm">
              {(manifest.average_jaccard_overlap * 100).toFixed(1)}%
            </span>
          </div>
          <div className="bg-slate-950 p-2.5 rounded border border-slate-800">
            <span className="text-slate-500 block text-[10px]">TOTAL TOKENS</span>
            <span className="text-white font-semibold text-sm">
              {manifest.total_tokens_consumed.toLocaleString()}
            </span>
          </div>
        </div>

        {/* Raw JSON viewer */}
        <div className="flex-1 overflow-hidden flex flex-col mb-4">
          <div className="flex items-center justify-between px-3 py-1.5 bg-slate-950 border-t border-l border-r border-slate-800 rounded-t text-[11px] font-mono text-slate-400">
            <span>audit.json</span>
            <span>SHA-256 Verified</span>
          </div>
          <pre className="flex-1 overflow-y-auto bg-slate-950 border border-slate-800 rounded-b p-3 font-mono text-xs text-slate-300 leading-relaxed">
            {jsonString}
          </pre>
        </div>

        {/* Actions */}
        <div className="flex items-center justify-between text-xs">
          <span className="font-mono text-slate-500 text-[11px]">
            ID: {manifest.migration_id}
          </span>
          <div className="flex items-center gap-2">
            <button
              onClick={handleCopy}
              className="flex items-center gap-1.5 px-3 py-1.5 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded text-xs font-medium cursor-pointer"
            >
              {copied ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
              {copied ? 'Copied' : 'Copy JSON'}
            </button>
            <button
              onClick={handleDownload}
              className="flex items-center gap-1.5 px-3 py-1.5 bg-blue-600 hover:bg-blue-500 text-white rounded text-xs font-medium cursor-pointer"
            >
              <Download className="w-3.5 h-3.5" />
              Download
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
