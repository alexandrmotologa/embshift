package server

import (
	"fmt"
	"net/http"
	"strings"
)

// HandleMetrics outputs system telemetry in the Prometheus text exposition format.
func (s *Server) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	telemetry, err := s.collectTelemetry(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to collect telemetry: %v", err), http.StatusInternalServerError)
		return
	}

	var sb strings.Builder

	// Replication Lag
	sb.WriteString("# HELP embshift_replication_lag_milliseconds Replication queue age in milliseconds\n")
	sb.WriteString("# TYPE embshift_replication_lag_milliseconds gauge\n")
	sb.WriteString(fmt.Sprintf("embshift_replication_lag_milliseconds %.2f\n\n", telemetry.ReplicationLagMs))

	// Queue Pending Events
	sb.WriteString("# HELP embshift_queue_pending_events Pending events in SQLite replication buffer\n")
	sb.WriteString("# TYPE embshift_queue_pending_events gauge\n")
	sb.WriteString(fmt.Sprintf("embshift_queue_pending_events %d\n\n", telemetry.ReplicationQueueSize))

	// DLQ Count
	sb.WriteString("# HELP embshift_dlq_items_count Poison-pill items sequestered in Dead-Letter Queue\n")
	sb.WriteString("# TYPE embshift_dlq_items_count gauge\n")
	sb.WriteString(fmt.Sprintf("embshift_dlq_items_count %d\n\n", telemetry.DLQCount))

	// Backfilled Documents
	sb.WriteString("# HELP embshift_backfilled_documents_total Total documents successfully re-embedded\n")
	sb.WriteString("# TYPE embshift_backfilled_documents_total counter\n")
	sb.WriteString(fmt.Sprintf("embshift_backfilled_documents_total %d\n\n", telemetry.BackfilledCount))

	// Parity Ratio
	sb.WriteString("# HELP embshift_parity_ratio Ratio of migrated documents to total legacy corpus\n")
	sb.WriteString("# TYPE embshift_parity_ratio gauge\n")
	sb.WriteString(fmt.Sprintf("embshift_parity_ratio %.4f\n\n", telemetry.ParityRatio))

	// Tokens Consumed
	sb.WriteString("# HELP embshift_tokens_consumed_total Total tokens used by embedding models\n")
	sb.WriteString("# TYPE embshift_tokens_consumed_total counter\n")
	sb.WriteString(fmt.Sprintf("embshift_tokens_consumed_total %d\n\n", telemetry.TokensConsumed))

	// Estimated Cost USD
	sb.WriteString("# HELP embshift_estimated_cost_usd Cumulative dollar spend against budget cap\n")
	sb.WriteString("# TYPE embshift_estimated_cost_usd gauge\n")
	sb.WriteString(fmt.Sprintf("embshift_estimated_cost_usd %.4f\n\n", telemetry.EstimatedCostUSD))

	// Shadow Jaccard Overlap
	sb.WriteString("# HELP embshift_shadow_jaccard_ratio Rolling average semantic rank overlap on live traffic\n")
	sb.WriteString("# TYPE embshift_shadow_jaccard_ratio gauge\n")
	sb.WriteString(fmt.Sprintf("embshift_shadow_jaccard_ratio %.4f\n\n", telemetry.AverageJaccard))

	// Canary Weight
	sb.WriteString("# HELP embshift_canary_weight_ratio Fraction of production search traffic served by target store\n")
	sb.WriteString("# TYPE embshift_canary_weight_ratio gauge\n")
	sb.WriteString(fmt.Sprintf("embshift_canary_weight_ratio %.4f\n\n", telemetry.CanaryWeight))

	// Active Primary Indicator
	sb.WriteString("# HELP embshift_active_primary_info Active primary store routing status\n")
	sb.WriteString("# TYPE embshift_active_primary_info gauge\n")
	if telemetry.ActivePrimary == "target" {
		sb.WriteString("embshift_active_primary_info{store=\"legacy\"} 0\n")
		sb.WriteString("embshift_active_primary_info{store=\"target\"} 1\n")
	} else {
		sb.WriteString("embshift_active_primary_info{store=\"legacy\"} 1\n")
		sb.WriteString("embshift_active_primary_info{store=\"target\"} 0\n")
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(sb.String()))
}
