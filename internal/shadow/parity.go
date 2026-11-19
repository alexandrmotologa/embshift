package shadow

import (
	"context"

	"github.com/alexandrmotologa/embshift/internal/adapter"
	"github.com/alexandrmotologa/embshift/internal/config"
	"github.com/alexandrmotologa/embshift/internal/queue"
)

// QualityGate evaluates whether vector collections have satisfied all cutover invariants.
type QualityGate struct {
	cfg   *config.Config
	queue *queue.SQLiteQueue
}

// NewQualityGate initializes a quality gate checker.
func NewQualityGate(cfg *config.Config, q *queue.SQLiteQueue) *QualityGate {
	return &QualityGate{
		cfg:   cfg,
		queue: q,
	}
}

// EvaluationReport details the readiness status for traffic cutover.
type EvaluationReport struct {
	Eligible        bool     `json:"eligible"`
	ParityRatio     float64  `json:"parity_ratio"`
	ReplicationLag  float64  `json:"replication_lag_ms"`
	QueuePending    int64    `json:"queue_pending"`
	AverageJaccard  float64  `json:"average_jaccard"`
	EvaluatedCount  int64    `json:"evaluated_count"`
	BlockingReasons []string `json:"blocking_reasons"`
}

// Evaluate performs a comprehensive inspection of parity, lag, and quality scores.
func (qg *QualityGate) Evaluate(ctx context.Context, legacyStore, targetStore adapter.VectorStore) (*EvaluationReport, error) {
	legacyCount, err := legacyStore.Count(ctx)
	if err != nil {
		return nil, err
	}

	migratedCount, err := qg.queue.CountMigratedDocuments()
	if err != nil {
		return nil, err
	}

	var parity float64
	if legacyCount > 0 {
		parity = float64(migratedCount) / float64(legacyCount)
		if parity > 1.0 {
			parity = 1.0
		}
	} else if migratedCount > 0 {
		parity = 1.0
	}

	lagMs, err := qg.queue.GetReplicationLagMs()
	if err != nil {
		return nil, err
	}

	pendingQueue, err := qg.queue.GetQueueSize()
	if err != nil {
		return nil, err
	}

	avgJaccard, shadowCount, err := qg.queue.GetAverageJaccard()
	if err != nil {
		return nil, err
	}

	var blocking []string

	reqParity := qg.cfg.Cutover.RequireParityRatio
	if reqParity <= 0 {
		reqParity = 1.0
	}
	if parity < reqParity {
		blocking = append(blocking, "Parity ratio is below required threshold (100% required)")
	}

	if lagMs > qg.cfg.Cutover.MaxReplicationLagMs {
		blocking = append(blocking, "Replication lag exceeds maximum allowed threshold")
	}

	if qg.cfg.Cutover.RequireZeroPendingQueue && pendingQueue > 0 {
		blocking = append(blocking, "Replication queue still has uncommitted events pending")
	}

	minJaccard := qg.cfg.Shadow.MinJaccardThreshold
	if minJaccard > 0 && shadowCount > 5 && avgJaccard < minJaccard {
		blocking = append(blocking, "Average shadow rank overlap is below configured threshold")
	}

	return &EvaluationReport{
		Eligible:        len(blocking) == 0,
		ParityRatio:     parity,
		ReplicationLag:  lagMs,
		QueuePending:    pendingQueue,
		AverageJaccard:  avgJaccard,
		EvaluatedCount:  shadowCount,
		BlockingReasons: blocking,
	}, nil
}
