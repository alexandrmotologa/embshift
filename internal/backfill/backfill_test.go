package backfill

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexandrmotologa/embshift/internal/adapter"
	"github.com/alexandrmotologa/embshift/internal/config"
	"github.com/alexandrmotologa/embshift/internal/models"
	"github.com/alexandrmotologa/embshift/internal/proxy"
	"github.com/alexandrmotologa/embshift/internal/queue"
)

func TestBackfillOrchestrator(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "embshift-backfill-test-*")
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
	cfg.Backfill.BatchSize = 10
	cfg.Backfill.TokensPerMinute = 5000000 // High TPM so test is fast
	cfg.Backfill.MaxBudgetUSD = 10.0

	legacyStore := adapter.NewMockStore("legacy-store", 1536)
	targetStore := adapter.NewMockStore("target-store", 3072)
	legacyEmbedder := adapter.NewMockEmbedder("text-embedding-ada-002", 1536)
	targetEmbedder := adapter.NewMockEmbedder("text-embedding-3-large", 3072)

	router := proxy.NewRouter(&cfg, legacyStore, targetStore, legacyEmbedder, targetEmbedder, q)

	// Populate legacy store with 25 test documents
	ctx := context.Background()
	var docs []models.VectorRecord
	for i := 1; i <= 25; i++ {
		text := fmt.Sprintf("High-dimensional vector embedding test document number %d", i)
		vec, _, _ := legacyEmbedder.EmbedQuery(ctx, text)
		docs = append(docs, models.VectorRecord{
			ID:      fmt.Sprintf("doc-%d", i),
			Vector:  vec,
			Text:    text,
			Payload: map[string]any{"index": i},
		})
	}
	_ = legacyStore.Upsert(ctx, docs)

	orchestrator := NewOrchestrator(&cfg, legacyStore, targetStore, targetEmbedder, q, router)

	// Verify initial progress
	mig, tot, parity := orchestrator.GetProgress(ctx)
	if tot != 25 || mig != 0 || parity != 0.0 {
		t.Fatalf("unexpected initial progress: mig=%d, tot=%d, parity=%f", mig, tot, parity)
	}

	// Start backfill
	err = orchestrator.Start(ctx)
	if err != nil {
		t.Fatalf("start backfill error: %v", err)
	}

	// Wait for backfill to finish
	timeout := time.After(3 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	completed := false
	for !completed {
		select {
		case <-timeout:
			t.Fatalf("backfill timed out")
		case <-ticker.C:
			mig, tot, parity = orchestrator.GetProgress(ctx)
			if mig == 25 && parity >= 0.999 {
				completed = true
			}
		}
	}

	// Verify target store count and dimensions
	targetCount, _ := targetStore.Count(ctx)
	if targetCount != 25 {
		t.Fatalf("expected 25 records in target store, got %d", targetCount)
	}

	tokens, spent, _ := orchestrator.GetBudgetStats()
	if tokens <= 0 || spent <= 0.0 {
		t.Fatalf("expected recorded token usage and spend, got tokens=%d, spent=%f", tokens, spent)
	}

	if router.GetState() != models.StateReadyForCutover {
		t.Fatalf("expected state ready_for_cutover, got %s", router.GetState())
	}
}
