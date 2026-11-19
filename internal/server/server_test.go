package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexandrmotologa/embshift/internal/adapter"
	"github.com/alexandrmotologa/embshift/internal/backfill"
	"github.com/alexandrmotologa/embshift/internal/config"
	"github.com/alexandrmotologa/embshift/internal/models"
	"github.com/alexandrmotologa/embshift/internal/proxy"
	"github.com/alexandrmotologa/embshift/internal/queue"
	"github.com/alexandrmotologa/embshift/internal/shadow"
)

func setupTestServer(t *testing.T) (*Server, http.Handler, func()) {
	tempDir, err := os.MkdirTemp("", "embshift-server-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}

	dbPath := filepath.Join(tempDir, "test.db")
	q, err := queue.NewSQLiteQueue(dbPath)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Cutover.RequireZeroPendingQueue = false
	cfg.Cutover.MaxReplicationLagMs = 1000.0

	legacyStore := adapter.NewMockStore("legacy", 1536)
	targetStore := adapter.NewMockStore("target", 3072)
	legacyEmbedder := adapter.NewMockEmbedder("ada-002", 1536)
	targetEmbedder := adapter.NewMockEmbedder("3-large", 3072)

	router := proxy.NewRouter(&cfg, legacyStore, targetStore, legacyEmbedder, targetEmbedder, q)
	dualWriter := proxy.NewDualWriter(router, q)
	queryMirror := proxy.NewQueryMirror(router, &cfg, q)
	orchestrator := backfill.NewOrchestrator(&cfg, legacyStore, targetStore, targetEmbedder, q, router)
	qualityGate := shadow.NewQualityGate(&cfg, q)

	srv := NewServer(&cfg, router, dualWriter, queryMirror, orchestrator, qualityGate, q)
	handler := srv.Handler()

	cleanup := func() {
		q.Close()
		os.RemoveAll(tempDir)
	}

	return srv, handler, cleanup
}

func TestServerEndpoints(t *testing.T) {
	_, handler, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. Test GET /api/status
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var status models.MigrationTelemetry
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.ActivePrimary != "legacy" {
		t.Fatalf("expected active primary legacy, got %s", status.ActivePrimary)
	}

	// 2. Test POST /v1/vectors/upsert
	upsertBody := map[string]any{
		"records": []map[string]any{
			{
				"id":   "vec-1",
				"text": "Neural vector indexing with HNSW graph exploration",
			},
		},
	}
	b, _ := json.Marshal(upsertBody)
	req = httptest.NewRequest(http.MethodPost, "/v1/vectors/upsert", bytes.NewReader(b))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected upsert 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	// 3. Test POST /v1/vectors/query
	queryBody := map[string]any{
		"text":  "neural vector indexing",
		"top_k": 2,
	}
	b, _ = json.Marshal(queryBody)
	req = httptest.NewRequest(http.MethodPost, "/v1/vectors/query", bytes.NewReader(b))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected query 200, got %d", rec.Code)
	}

	var queryResult models.SearchResult
	if err := json.NewDecoder(rec.Body).Decode(&queryResult); err != nil {
		t.Fatalf("decode query result: %v", err)
	}
	if len(queryResult.Records) == 0 {
		t.Fatalf("expected at least 1 record from query")
	}

	// 4. Test POST /api/simulate/insert
	simBody := map[string]any{"count": 5}
	b, _ = json.Marshal(simBody)
	req = httptest.NewRequest(http.MethodPost, "/api/simulate/insert", bytes.NewReader(b))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected simulate insert 200, got %d", rec.Code)
	}

	// 5. Test POST /api/simulate/query
	simQBody := map[string]any{"text": "simulated query text", "top_k": 3}
	b, _ = json.Marshal(simQBody)
	req = httptest.NewRequest(http.MethodPost, "/api/simulate/query", bytes.NewReader(b))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected simulate query 200, got %d", rec.Code)
	}

	// 6. Test POST /v1/vectors/delete
	delBody := map[string]any{
		"ids": []string{"vec-1"},
	}
	b, _ = json.Marshal(delBody)
	req = httptest.NewRequest(http.MethodPost, "/v1/vectors/delete", bytes.NewReader(b))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected delete 200, got %d", rec.Code)
	}
}
