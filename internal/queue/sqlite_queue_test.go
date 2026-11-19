package queue

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteQueue(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "embshift-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	q, err := NewSQLiteQueue(dbPath)
	if err != nil {
		t.Fatalf("new sqlite queue: %v", err)
	}
	defer q.Close()

	// Test Enqueue
	err = q.Enqueue("upsert", "doc-1", []byte(`{"title":"hello"}`), "hello world text")
	if err != nil {
		t.Fatalf("enqueue error: %v", err)
	}

	size, err := q.GetQueueSize()
	if err != nil || size != 1 {
		t.Fatalf("expected queue size 1, got %d (err: %v)", size, err)
	}

	// Test Dequeue
	events, err := q.DequeueBatch(10)
	if err != nil {
		t.Fatalf("dequeue error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].RecordID != "doc-1" || events[0].Text != "hello world text" {
		t.Fatalf("unexpected event data: %+v", events[0])
	}

	// Test Ack
	err = q.Ack([]int64{events[0].ID})
	if err != nil {
		t.Fatalf("ack error: %v", err)
	}

	size, err = q.GetQueueSize()
	if err != nil || size != 0 {
		t.Fatalf("expected queue size 0 after ack, got %d", size)
	}

	// Test Checkpoint
	migrated, err := q.IsDocumentMigrated("doc-1")
	if err != nil || migrated {
		t.Fatalf("expected not migrated yet, got %v", migrated)
	}

	err = q.MarkDocumentMigrated("doc-1")
	if err != nil {
		t.Fatalf("mark migrated error: %v", err)
	}

	migrated, err = q.IsDocumentMigrated("doc-1")
	if err != nil || !migrated {
		t.Fatalf("expected migrated true, got %v", migrated)
	}

	count, err := q.CountMigratedDocuments()
	if err != nil || count != 1 {
		t.Fatalf("expected 1 migrated document, got %d", count)
	}

	// Test Shadow query
	err = q.RecordShadowQuery("q-1", "test query", 0.85, 12.5, 9.2)
	if err != nil {
		t.Fatalf("record shadow query error: %v", err)
	}

	avg, total, err := q.GetAverageJaccard()
	if err != nil || total != 1 || avg < 0.84 || avg > 0.86 {
		t.Fatalf("expected avg 0.85 and total 1, got avg %f total %d", avg, total)
	}

	// Test lag calculation when empty
	lag, err := q.GetReplicationLagMs()
	if err != nil || lag != 0 {
		t.Fatalf("expected 0 lag for empty queue, got %f", lag)
	}

	// Test lag calculation with pending item
	time.Sleep(10 * time.Millisecond)
	_ = q.Enqueue("upsert", "doc-2", nil, "lag test")
	time.Sleep(15 * time.Millisecond)
	lag, err = q.GetReplicationLagMs()
	if err != nil || lag < 10 {
		t.Fatalf("expected lag >= 10ms, got %f", lag)
	}
}
