package shadow

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexandrmotologa/embshift/internal/adapter"
	"github.com/alexandrmotologa/embshift/internal/config"
	"github.com/alexandrmotologa/embshift/internal/models"
	"github.com/alexandrmotologa/embshift/internal/queue"
)

func TestShadowEvaluationAndQualityGate(t *testing.T) {
	// Test Jaccard
	a := []models.ScoredRecord{
		{ID: "doc-1", Score: 0.9},
		{ID: "doc-2", Score: 0.8},
		{ID: "doc-3", Score: 0.7},
	}
	b := []models.ScoredRecord{
		{ID: "doc-1", Score: 0.95},
		{ID: "doc-2", Score: 0.85},
		{ID: "doc-4", Score: 0.75},
	}

	jaccard := ComputeJaccardAtK(a, b)
	// Intersection: doc-1, doc-2 (2)
	// Union: doc-1, doc-2, doc-3, doc-4 (4)
	// Jaccard = 2/4 = 0.5
	if jaccard != 0.5 {
		t.Fatalf("expected jaccard 0.5, got %f", jaccard)
	}

	// Test NDCG
	ndcg := ComputeNDCGAtK(a, b, 3)
	if ndcg <= 0.0 || ndcg > 1.0 {
		t.Fatalf("unexpected ndcg: %f", ndcg)
	}

	// Test Quality Gate
	tempDir, err := os.MkdirTemp("", "embshift-shadow-test-*")
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
	gate := NewQualityGate(&cfg, q)

	legacyStore := adapter.NewMockStore("legacy", 1536)
	targetStore := adapter.NewMockStore("target", 3072)

	_ = legacyStore.Upsert(context.Background(), []models.VectorRecord{
		{ID: "doc-1", Vector: make([]float32, 1536)},
		{ID: "doc-2", Vector: make([]float32, 1536)},
	})

	// Initial check: 0 migrated out of 2 => not eligible
	report, err := gate.Evaluate(context.Background(), legacyStore, targetStore)
	if err != nil {
		t.Fatalf("evaluate error: %v", err)
	}
	if report.Eligible {
		t.Fatalf("expected not eligible due to 0 parity")
	}

	// Mark both migrated
	_ = q.MarkDocumentMigrated("doc-1")
	_ = q.MarkDocumentMigrated("doc-2")

	report, err = gate.Evaluate(context.Background(), legacyStore, targetStore)
	if err != nil {
		t.Fatalf("evaluate error: %v", err)
	}
	if !report.Eligible {
		t.Fatalf("expected eligible when parity is 1.0, reasons: %v", report.BlockingReasons)
	}
}
