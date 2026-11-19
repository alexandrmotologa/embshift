package proxy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexandrmotologa/embshift/internal/adapter"
	"github.com/alexandrmotologa/embshift/internal/config"
	"github.com/alexandrmotologa/embshift/internal/models"
	"github.com/alexandrmotologa/embshift/internal/queue"
)

func TestProxyDualWriteCanaryAndCutover(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "embshift-proxy-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	q, err := queue.NewSQLiteQueue(dbPath)
	if err != nil {
		t.Fatalf("new sqlite queue: %v", err)
	}
	defer q.Close()

	cfg := config.DefaultConfig()
	cfg.Cutover.RequireZeroPendingQueue = true
	cfg.Cutover.MaxReplicationLagMs = 500.0

	legacyStore := adapter.NewMockStore("legacy-store", 1536)
	targetStore := adapter.NewMockStore("target-store", 3072)
	legacyEmbedder := adapter.NewMockEmbedder("text-embedding-ada-002", 1536)
	targetEmbedder := adapter.NewMockEmbedder("text-embedding-3-large", 3072)

	router := NewRouter(&cfg, legacyStore, targetStore, legacyEmbedder, targetEmbedder, q)
	dualWriter := NewDualWriter(router, q)
	queryMirror := NewQueryMirror(router, &cfg, q)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go dualWriter.StartReplicationWorker(ctx)

	// 1. Ingestion: Dual Write Upsert
	records := []models.VectorRecord{
		{
			ID:      "doc-1",
			Text:    "Artificial intelligence algorithms in semantic retrieval systems",
			Payload: map[string]any{"source": "arxiv"},
		},
		{
			ID:      "doc-2",
			Text:    "Distributed database consensus and vector indexing architectures",
			Payload: map[string]any{"source": "ieee"},
		},
	}

	err = dualWriter.Upsert(ctx, records)
	if err != nil {
		t.Fatalf("upsert error: %v", err)
	}

	time.Sleep(150 * time.Millisecond)

	tc, _ := targetStore.Count(ctx)
	if tc != 2 {
		t.Fatalf("expected target store count 2, got %d", tc)
	}

	// 2. Test Canary Routing (Set Canary to 100% temporarily to verify route)
	err = router.SetCanaryWeight(1.0)
	if err != nil {
		t.Fatalf("set canary weight error: %v", err)
	}
	if router.GetState() != models.StateCanaryRouting {
		t.Fatalf("expected state canary_routing, got %s", router.GetState())
	}

	canaryResult, err := queryMirror.Query(ctx, models.SearchQuery{
		Text: "artificial intelligence",
		TopK: 2,
	})
	if err != nil {
		t.Fatalf("canary query error: %v", err)
	}
	if !canaryResult.IsCanary {
		t.Fatalf("expected IsCanary to be true when canary weight is 1.0")
	}

	// Reset canary to 0
	router.AbortCanary()
	if router.GetCanaryWeight() != 0.0 {
		t.Fatalf("expected canary weight 0 after abort")
	}

	// 3. Query Mirroring on normal path
	qRes, err := queryMirror.Query(ctx, models.SearchQuery{
		Text: "semantic retrieval algorithms",
		TopK: 2,
	})
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(qRes.Records) == 0 {
		t.Fatalf("expected search records from primary")
	}

	time.Sleep(100 * time.Millisecond)
	recent := queryMirror.GetRecentShadowComparisons()
	if len(recent) == 0 {
		t.Fatalf("expected shadow comparisons recorded")
	}

	// 4. Atomic Cutover
	manifest, err := router.ExecuteCutover()
	if err != nil {
		t.Fatalf("execute cutover error: %v", err)
	}
	if manifest == nil || manifest.LegacyDimensions != 1536 || manifest.TargetDimensions != 3072 {
		t.Fatalf("invalid audit manifest: %+v", manifest)
	}
	if router.GetActivePrimary() != "target" {
		t.Fatalf("expected active primary to be target after cutover")
	}

	// 5. Rollback
	err = router.ExecuteRollback()
	if err != nil {
		t.Fatalf("rollback error: %v", err)
	}
	if router.GetActivePrimary() != "legacy" {
		t.Fatalf("expected active primary to be legacy after rollback")
	}
}
