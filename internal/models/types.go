package models

import "time"

// MigrationState represents the current phase of the migration lifecycle.
type MigrationState string

const (
	StateIdle             MigrationState = "idle"
	StateBackfilling      MigrationState = "backfilling"
	StatePaused           MigrationState = "paused"
	StateShadowTesting    MigrationState = "shadow_testing"
	StateReadyForCutover  MigrationState = "ready_for_cutover"
	StateCutoverComplete  MigrationState = "cutover_complete"
	StateRolledBack       MigrationState = "rolled_back"
)

// VectorRecord represents an entity in a vector database collection.
type VectorRecord struct {
	ID        string         `json:"id"`
	Vector    []float32      `json:"vector,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
	Text      string         `json:"text,omitempty"`
	CreatedAt time.Time      `json:"created_at,omitempty"`
	UpdatedAt time.Time      `json:"updated_at,omitempty"`
}

// SearchQuery represents a retrieval request across vector stores.
type SearchQuery struct {
	Vector []float32      `json:"vector,omitempty"`
	Text   string         `json:"text,omitempty"`
	TopK   int            `json:"top_k"`
	Filter map[string]any `json:"filter,omitempty"`
}

// ScoredRecord represents a vector search match with similarity score.
type ScoredRecord struct {
	ID      string         `json:"id"`
	Score   float32        `json:"score"`
	Vector  []float32      `json:"vector,omitempty"`
	Payload map[string]any `json:"payload,omitempty"`
}

// SearchResult wraps scored records returned from a store search.
type SearchResult struct {
	Records   []ScoredRecord `json:"records"`
	LatencyMs float64        `json:"latency_ms"`
	StoreName string         `json:"store_name"`
	ModelName string         `json:"model_name"`
}

// ReplicationEvent represents an operation queued for asynchronous target replication.
type ReplicationEvent struct {
	ID        int64     `json:"id"`
	Operation string    `json:"operation"` // "upsert" or "delete"
	RecordID  string    `json:"record_id"`
	Payload   []byte    `json:"payload,omitempty"`
	Text      string    `json:"text,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Attempts  int       `json:"attempts"`
	LastError string    `json:"last_error,omitempty"`
}

// ShadowComparison records the side-by-side search performance on a mirrored query.
type ShadowComparison struct {
	QueryID           string         `json:"query_id"`
	QueryText         string         `json:"query_text"`
	LegacyResults     []ScoredRecord `json:"legacy_results"`
	TargetResults     []ScoredRecord `json:"target_results"`
	JaccardSimilarity float64        `json:"jaccard_similarity"`
	LegacyLatencyMs   float64        `json:"legacy_latency_ms"`
	TargetLatencyMs   float64        `json:"target_latency_ms"`
	Timestamp         time.Time      `json:"timestamp"`
}

// MigrationTelemetry holds real-time counters and operational status.
type MigrationTelemetry struct {
	State                MigrationState `json:"state"`
	ActivePrimary        string         `json:"active_primary"`
	LegacyCount          int64          `json:"legacy_count"`
	TargetCount          int64          `json:"target_count"`
	BackfilledCount      int64          `json:"backfilled_count"`
	ParityRatio          float64        `json:"parity_ratio"`
	ReplicationLagMs     float64        `json:"replication_lag_ms"`
	ReplicationQueueSize int64          `json:"replication_queue_size"`
	TokensConsumed       int64          `json:"tokens_consumed"`
	EstimatedCostUSD     float64        `json:"estimated_cost_usd"`
	CostLimitUSD         float64        `json:"cost_limit_usd"`
	AverageJaccard       float64        `json:"average_jaccard"`
	EvaluatedQueries     int64          `json:"evaluated_queries"`
	CutoverEligible      bool           `json:"cutover_eligible"`
	CutoverAt            *time.Time     `json:"cutover_at,omitempty"`
	ReverseReplication   bool           `json:"reverse_replication"`
	RecentShadowQueries  []ShadowComparison `json:"recent_shadow_queries,omitempty"`
}

// AuditManifest represents the permanent record generated upon migration cutover.
type AuditManifest struct {
	MigrationID             string    `json:"migration_id"`
	Timestamp               time.Time `json:"timestamp"`
	LegacyStore             string    `json:"legacy_store"`
	TargetStore             string    `json:"target_store"`
	LegacyModel             string    `json:"legacy_model"`
	TargetModel             string    `json:"target_model"`
	LegacyDimensions        int       `json:"legacy_dimensions"`
	TargetDimensions        int       `json:"target_dimensions"`
	TotalDocumentsMigrated  int64     `json:"total_documents_migrated"`
	FinalParityRatio        float64   `json:"final_parity_ratio"`
	AverageJaccardOverlap   float64   `json:"average_jaccard_overlap"`
	EvaluatedShadowQueries  int64     `json:"evaluated_shadow_queries"`
	TotalTokensConsumed     int64     `json:"total_tokens_consumed"`
	TotalCostUSD            float64   `json:"total_cost_usd"`
	CutoverExecutionTimeUTC time.Time `json:"cutover_execution_time_utc"`
	IntegritySHA256         string    `json:"integrity_sha256"`
}
