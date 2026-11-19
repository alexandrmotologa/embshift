package queue

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSQLiteQueueWithDLQ(t *testing.T) {
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

	// 1. Test Enqueue
	err = q.Enqueue("upsert", "doc-1", []byte(`{"title":"hello"}`), "hello world text")
	if err != nil {
		t.Fatalf("enqueue error: %v", err)
	}

	size, err := q.GetQueueSize()
	if err != nil || size != 1 {
		t.Fatalf("expected queue size 1, got %d (err: %v)", size, err)
	}

	// 2. Test Dequeue
	events, err := q.DequeueBatch(10)
	if err != nil || len(events) != 1 {
		t.Fatalf("dequeue error: %v", err)
	}

	// 3. Test Nack with DLQ threshold (maxAttempts = 2)
	// Attempt 1 -> still in queue
	err = q.Nack(events[0].ID, "transient network error", 2)
	if err != nil {
		t.Fatalf("nack 1 error: %v", err)
	}
	dlqCount, _ := q.GetDLQCount()
	if dlqCount != 0 {
		t.Fatalf("expected 0 DLQ items after first attempt, got %d", dlqCount)
	}

	// Attempt 2 -> moved to DLQ
	err = q.Nack(events[0].ID, "permanent context window exceeded", 2)
	if err != nil {
		t.Fatalf("nack 2 error: %v", err)
	}

	dlqCount, _ = q.GetDLQCount()
	if dlqCount != 1 {
		t.Fatalf("expected 1 DLQ item after exceeding maxAttempts, got %d", dlqCount)
	}

	// Active queue should now be empty
	size, _ = q.GetQueueSize()
	if size != 0 {
		t.Fatalf("expected 0 items in active queue after moving to DLQ, got %d", size)
	}

	// Inspect DLQ items
	dlqItems, err := q.GetDLQItems(10)
	if err != nil || len(dlqItems) != 1 {
		t.Fatalf("expected 1 DLQ item, got %d (err: %v)", len(dlqItems), err)
	}
	if dlqItems[0].RecordID != "doc-1" || dlqItems[0].ErrorReason != "permanent context window exceeded" {
		t.Fatalf("unexpected DLQ item data: %+v", dlqItems[0])
	}

	// 4. Test Retry DLQ item
	err = q.RetryDLQItem(dlqItems[0].ID)
	if err != nil {
		t.Fatalf("retry DLQ item error: %v", err)
	}
	dlqCount, _ = q.GetDLQCount()
	if dlqCount != 0 {
		t.Fatalf("expected 0 DLQ items after retry, got %d", dlqCount)
	}
	size, _ = q.GetQueueSize()
	if size != 1 {
		t.Fatalf("expected active queue size 1 after retry, got %d", size)
	}

	// 5. Test Ack
	requeued, _ := q.DequeueBatch(10)
	_ = q.Ack([]int64{requeued[0].ID})
	size, _ = q.GetQueueSize()
	if size != 0 {
		t.Fatalf("expected 0 queue size after ack")
	}

	// 6. Test Purge DLQ item
	_ = q.Enqueue("upsert", "doc-2", nil, "poison text")
	evs, _ := q.DequeueBatch(1)
	_ = q.Nack(evs[0].ID, "bad syntax", 1) // immediately moves to DLQ
	dlqCount, _ = q.GetDLQCount()
	if dlqCount != 1 {
		t.Fatalf("expected 1 DLQ item, got %d", dlqCount)
	}
	items, _ := q.GetDLQItems(1)
	err = q.PurgeDLQItem(items[0].ID)
	if err != nil {
		t.Fatalf("purge DLQ item error: %v", err)
	}
	dlqCount, _ = q.GetDLQCount()
	if dlqCount != 0 {
		t.Fatalf("expected 0 DLQ items after purge")
	}
}
