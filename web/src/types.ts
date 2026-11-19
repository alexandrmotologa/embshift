export type MigrationState =
  | 'idle'
  | 'backfilling'
  | 'paused'
  | 'shadow_testing'
  | 'canary_routing'
  | 'ready_for_cutover'
  | 'cutover_complete'
  | 'rolled_back'

export interface ScoredRecord {
  id: string
  score: number
  vector?: number[]
  payload?: Record<string, unknown>
}

export interface ShadowComparison {
  query_id: string
  query_text: string
  legacy_results: ScoredRecord[]
  target_results: ScoredRecord[]
  jaccard_similarity: number
  ndcg_at_k: number
  legacy_latency_ms: number
  target_latency_ms: number
  timestamp: string
}

export interface DLQItem {
  id: number
  record_id: string
  operation: string
  text?: string
  payload_json?: string
  error_reason: string
  failed_at: string
  attempts: number
}

export interface MigrationTelemetry {
  state: MigrationState
  active_primary: 'legacy' | 'target'
  canary_weight: number
  legacy_count: number
  target_count: number
  backfilled_count: number
  parity_ratio: number
  replication_lag_ms: number
  replication_queue_size: number
  dlq_count: number
  tokens_consumed: number
  estimated_cost_usd: number
  cost_limit_usd: number
  average_jaccard: number
  evaluated_queries: number
  cutover_eligible: boolean
  cutover_at?: string
  reverse_replication: boolean
  recent_shadow_queries?: ShadowComparison[]
}

export interface AuditManifest {
  migration_id: string
  timestamp: string
  legacy_store: string
  target_store: string
  legacy_model: string
  target_model: string
  legacy_dimensions: number
  target_dimensions: number
  total_documents_migrated: number
  final_parity_ratio: number
  average_jaccard_overlap: number
  evaluated_shadow_queries: number
  total_tokens_consumed: number
  total_cost_usd: number
  cutover_execution_time_utc: string
  integrity_sha256: string
}
