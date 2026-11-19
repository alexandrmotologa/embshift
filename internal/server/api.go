package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/alexandrmotologa/embshift/internal/backfill"
	"github.com/alexandrmotologa/embshift/internal/config"
	"github.com/alexandrmotologa/embshift/internal/models"
	"github.com/alexandrmotologa/embshift/internal/proxy"
	"github.com/alexandrmotologa/embshift/internal/queue"
	"github.com/alexandrmotologa/embshift/internal/shadow"
)

// Server coordinates HTTP endpoints for data plane proxy and migration control studio.
type Server struct {
	cfg           *config.Config
	router        *proxy.Router
	dualWriter    *proxy.DualWriter
	queryMirror   *proxy.QueryMirror
	orchestrator  *backfill.Orchestrator
	qualityGate   *shadow.QualityGate
	queue         *queue.SQLiteQueue
	broadcaster   *Broadcaster
	auditManifest *models.AuditManifest
	staticFS      fs.FS
	mu            sync.RWMutex
}

// NewServer initializes the HTTP control server.
func NewServer(
	cfg *config.Config,
	router *proxy.Router,
	dualWriter *proxy.DualWriter,
	queryMirror *proxy.QueryMirror,
	orchestrator *backfill.Orchestrator,
	qualityGate *shadow.QualityGate,
	q *queue.SQLiteQueue,
) *Server {
	return &Server{
		cfg:          cfg,
		router:       router,
		dualWriter:   dualWriter,
		queryMirror:  queryMirror,
		orchestrator: orchestrator,
		qualityGate:  qualityGate,
		queue:        q,
		broadcaster:  NewBroadcaster(),
	}
}

// SetStaticFS attaches the embedded React 19 single-page application filesystem.
func (s *Server) SetStaticFS(staticFS fs.FS) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.staticFS = staticFS
}

// Handler configures HTTP routes and returns the server handler with CORS middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Data Plane Proxy Endpoints
	mux.HandleFunc("POST /v1/vectors/upsert", s.handleUpsert)
	mux.HandleFunc("POST /v1/vectors/query", s.handleQuery)
	mux.HandleFunc("POST /v1/vectors/delete", s.handleDelete)

	// Migration Control Plane Endpoints
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("POST /api/backfill/start", s.handleBackfillStart)
	mux.HandleFunc("POST /api/backfill/pause", s.handleBackfillPause)
	mux.HandleFunc("POST /api/backfill/resume", s.handleBackfillResume)
	mux.HandleFunc("POST /api/cutover", s.handleCutover)
	mux.HandleFunc("POST /api/rollback", s.handleRollback)
	mux.HandleFunc("GET /api/audit", s.handleAudit)
	mux.HandleFunc("POST /api/simulate/query", s.handleSimulateQuery)
	mux.HandleFunc("POST /api/simulate/insert", s.handleSimulateInsert)
	mux.HandleFunc("GET /api/events", s.broadcaster.HandleSSE)

	// Embedded Static UI Assets
	s.mu.RLock()
	staticFS := s.staticFS
	s.mu.RUnlock()
	if staticFS != nil {
		mux.Handle("/", ServeStaticSPA(staticFS))
	}

	return withCORS(mux)
}

// StartBroadcasting runs a periodic ticker that pushes live telemetry updates to SSE listeners.
func (s *Server) StartBroadcasting(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			telemetry, err := s.collectTelemetry(ctx)
			if err == nil {
				s.broadcaster.Broadcast("telemetry", telemetry)
			}
		}
	}
}

func (s *Server) handleUpsert(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Records []models.VectorRecord `json:"records"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, fmt.Sprintf("invalid request payload: %v", err), http.StatusBadRequest)
		return
	}

	if err := s.dualWriter.Upsert(r.Context(), body.Records); err != nil {
		http.Error(w, fmt.Sprintf("upsert failed: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"upserted": len(body.Records),
	})
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	var q models.SearchQuery
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		http.Error(w, fmt.Sprintf("invalid query payload: %v", err), http.StatusBadRequest)
		return
	}

	result, err := s.queryMirror.Query(r.Context(), q)
	if err != nil {
		http.Error(w, fmt.Sprintf("query failed: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, fmt.Sprintf("invalid delete payload: %v", err), http.StatusBadRequest)
		return
	}

	if err := s.dualWriter.Delete(r.Context(), body.IDs); err != nil {
		http.Error(w, fmt.Sprintf("delete failed: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"deleted": len(body.IDs),
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	telemetry, err := s.collectTelemetry(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("collect telemetry: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, telemetry)
}

func (s *Server) handleBackfillStart(w http.ResponseWriter, r *http.Request) {
	if err := s.orchestrator.Start(context.Background()); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "backfill_started"})
}

func (s *Server) handleBackfillPause(w http.ResponseWriter, r *http.Request) {
	s.orchestrator.Pause()
	writeJSON(w, http.StatusOK, map[string]string{"status": "backfill_paused"})
}

func (s *Server) handleBackfillResume(w http.ResponseWriter, r *http.Request) {
	if err := s.orchestrator.Resume(context.Background()); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "backfill_resumed"})
}

func (s *Server) handleCutover(w http.ResponseWriter, r *http.Request) {
	manifest, err := s.router.ExecuteCutover()
	if err != nil {
		http.Error(w, fmt.Sprintf("cutover failed: %v", err), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	s.auditManifest = manifest
	s.mu.Unlock()

	// Write audit manifest to disk
	auditData, _ := json.MarshalIndent(manifest, "", "  ")
	_ = os.WriteFile("audit.json", auditData, 0644)

	s.broadcaster.Broadcast("cutover", manifest)
	writeJSON(w, http.StatusOK, manifest)
}

func (s *Server) handleRollback(w http.ResponseWriter, r *http.Request) {
	if err := s.router.ExecuteRollback(); err != nil {
		http.Error(w, fmt.Sprintf("rollback failed: %v", err), http.StatusBadRequest)
		return
	}
	s.broadcaster.Broadcast("rollback", map[string]string{"status": "rolled_back"})
	writeJSON(w, http.StatusOK, map[string]string{"status": "rolled_back"})
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	manifest := s.auditManifest
	s.mu.RUnlock()

	if manifest == nil {
		if data, err := os.ReadFile("audit.json"); err == nil {
			var m models.AuditManifest
			if json.Unmarshal(data, &m) == nil {
				manifest = &m
			}
		}
	}

	if manifest == nil {
		http.Error(w, "no cutover audit manifest available", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, manifest)
}

func (s *Server) handleSimulateQuery(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
		TopK int    `json:"top_k"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Text == "" {
		http.Error(w, "missing or invalid query text", http.StatusBadRequest)
		return
	}

	topK := body.TopK
	if topK <= 0 {
		topK = 5
	}

	res, err := s.queryMirror.Query(r.Context(), models.SearchQuery{
		Text: body.Text,
		TopK: topK,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleSimulateInsert(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Count int `json:"count"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	count := body.Count
	if count <= 0 {
		count = 5
	}
	if count > 500 {
		count = 500
	}

	records := make([]models.VectorRecord, count)
	now := time.Now().UnixNano()
	for i := 0; i < count; i++ {
		records[i] = models.VectorRecord{
			ID:   fmt.Sprintf("sim-%d-%d", now, i),
			Text: fmt.Sprintf("Simulated runtime document chunk index %d created during active migration", i),
			Payload: map[string]any{
				"type":      "simulation",
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			},
		}
	}

	if err := s.dualWriter.Upsert(r.Context(), records); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "inserted",
		"count":    count,
		"records":  records,
	})
}

func (s *Server) collectTelemetry(ctx context.Context) (*models.MigrationTelemetry, error) {
	legacyStore := s.router.GetPrimaryStore()
	targetStore := s.router.GetSecondaryStore()
	if s.router.GetActivePrimary() == "target" {
		legacyStore = s.router.GetSecondaryStore()
		targetStore = s.router.GetPrimaryStore()
	}

	legacyCount, _ := legacyStore.Count(ctx)
	targetCount, _ := targetStore.Count(ctx)
	migratedCount, total, parity := s.orchestrator.GetProgress(ctx)
	if total == 0 {
		total = legacyCount
	}

	lagMs, _ := s.queue.GetReplicationLagMs()
	queueSize, _ := s.queue.GetQueueSize()
	tokens, spent, _ := s.orchestrator.GetBudgetStats()
	avgJaccard, shadowCount, _ := s.queue.GetAverageJaccard()

	report, _ := s.qualityGate.Evaluate(ctx, legacyStore, targetStore)
	cutoverEligible := false
	if report != nil {
		cutoverEligible = report.Eligible
	}

	recent := s.queryMirror.GetRecentShadowComparisons()

	return &models.MigrationTelemetry{
		State:                s.router.GetState(),
		ActivePrimary:        s.router.GetActivePrimary(),
		LegacyCount:          legacyCount,
		TargetCount:          targetCount,
		BackfilledCount:      migratedCount,
		ParityRatio:          parity,
		ReplicationLagMs:     lagMs,
		ReplicationQueueSize: queueSize,
		TokensConsumed:       tokens,
		EstimatedCostUSD:     spent,
		CostLimitUSD:         s.cfg.Backfill.MaxBudgetUSD,
		AverageJaccard:       avgJaccard,
		EvaluatedQueries:     shadowCount,
		CutoverEligible:      cutoverEligible,
		CutoverAt:            s.router.GetCutoverTime(),
		ReverseReplication:   s.router.IsReverseReplicationEnabled(),
		RecentShadowQueries:  recent,
	}, nil
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
