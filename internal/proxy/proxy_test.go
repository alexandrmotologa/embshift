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

func TestProxyDualWriteAndCutover(t *testing.T) {
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

	// Start background replication worker
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

	// Verify legacy store has 2 records
	c, _ := legacyStore.Count(ctx)
	if c != 2 {
		t.Fatalf("expected legacy store count 2, got %d", c)
	}

	// Wait for replication worker to drain queue
	time.Sleep(150 * time.Millisecond)

	tc, _ := targetStore.Count(ctx)
	if tc != 2 {
		t.Fatalf("expected target store count 2 after replication, got %d", tc)
	}

	queueSize, _ := q.GetQueueSize()
	if queueSize != 0 {
		t.Fatalf("expected replication queue size 0, got %d", queueSize)
	}

	// 2. Query Mirroring
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
		t.Fatalf("expected at least 1 shadow comparison recorded")
	}

	// 3. Atomic Cutover
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

	// 4. Test Upsert post-cutover
	postRecords := []models.VectorRecord{
		{
			ID:   "doc-3",
			Text: "Post-cutover document written directly to target store",
		},
	}
	err = dualWriter.Upsert(ctx, postRecords)
	if err != nil {
		t.Fatalf("post-cutover upsert error: %v", err)
	}

	tc, _ = targetStore.Count(ctx)
	if tc != 3 {
		t.Fatalf("expected target count 3, got %d", tc)
	}

	// 5. Test Rollback
	err = router.ExecuteRollback()
	if err != nil {
		t.Fatalf("rollback error: %v", err)
	}
	if router.GetActivePrimary() != "legacy" {
		t.Fatalf("expected active primary to be legacy after rollback")
	}
}
