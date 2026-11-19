package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/alexandrmotologa/embshift/internal/models"
	"github.com/alexandrmotologa/embshift/internal/queue"
)

// DualWriter orchestrates synchronous primary writes and asynchronous shadow replication.
type DualWriter struct {
	router *Router
	queue  *queue.SQLiteQueue
}

// NewDualWriter constructs a dual writer connected to the router and replication queue.
func NewDualWriter(router *Router, q *queue.SQLiteQueue) *DualWriter {
	return &DualWriter{
		router: router,
		queue:  q,
	}
}

// Upsert commits vectors synchronously to the primary store and enqueues shadow replication.
func (dw *DualWriter) Upsert(ctx context.Context, records []models.VectorRecord) error {
	if len(records) == 0 {
		return nil
	}

	primaryStore := dw.router.GetPrimaryStore()
	primaryEmbedder := dw.router.GetPrimaryEmbedder()

	// Ensure primary records have valid vectors
	var needEmbedding []int
	var textsToEmbed []string
	for i, r := range records {
		if len(r.Vector) == 0 && r.Text != "" {
			needEmbedding = append(needEmbedding, i)
			textsToEmbed = append(textsToEmbed, r.Text)
		}
	}

	if len(textsToEmbed) > 0 {
		vecs, _, err := primaryEmbedder.EmbedTexts(ctx, textsToEmbed)
		if err != nil {
			return fmt.Errorf("generate primary embeddings: %w", err)
		}
		for idx, vecIndex := range needEmbedding {
			records[vecIndex].Vector = vecs[idx]
		}
	}

	// 1. Synchronous primary commit
	if err := primaryStore.Upsert(ctx, records); err != nil {
		return fmt.Errorf("primary vector upsert failed: %w", err)
	}

	// 2. Asynchronous shadow replication enqueue
	for _, rec := range records {
		var payloadBytes []byte
		if rec.Payload != nil {
			payloadBytes, _ = json.Marshal(rec.Payload)
		}
		textVal := rec.Text
		if textVal == "" && rec.Payload != nil {
			if t, ok := rec.Payload["text"].(string); ok {
				textVal = t
			}
		}

		if err := dw.queue.Enqueue("upsert", rec.ID, payloadBytes, textVal); err != nil {
			log.Printf("[dual_write] warning: failed to enqueue replication event for %s: %v", rec.ID, err)
		}
	}

	return nil
}

// Delete commits deletions synchronously to the primary store and enqueues shadow deletions.
func (dw *DualWriter) Delete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	primaryStore := dw.router.GetPrimaryStore()
	if err := primaryStore.Delete(ctx, ids); err != nil {
		return fmt.Errorf("primary vector delete failed: %w", err)
	}

	for _, id := range ids {
		if err := dw.queue.Enqueue("delete", id, nil, ""); err != nil {
			log.Printf("[dual_write] warning: failed to enqueue deletion event for %s: %v", id, err)
		}
	}

	return nil
}

// StartReplicationWorker continuously dequeues and processes shadow replication events.
func (dw *DualWriter) StartReplicationWorker(ctx context.Context) {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			dw.drainReplicationBatch(ctx, 50)
		}
	}
}

func (dw *DualWriter) drainReplicationBatch(ctx context.Context, limit int) {
	events, err := dw.queue.DequeueBatch(limit)
	if err != nil || len(events) == 0 {
		return
	}

	secondaryStore := dw.router.GetSecondaryStore()
	secondaryEmbedder := dw.router.GetSecondaryEmbedder()

	var upsertEvents []models.ReplicationEvent
	var deleteIDs []string
	var ackIDs []int64

	for _, ev := range events {
		if ev.Operation == "delete" {
			deleteIDs = append(deleteIDs, ev.RecordID)
			ackIDs = append(ackIDs, ev.ID)
		} else if ev.Operation == "upsert" {
			upsertEvents = append(upsertEvents, ev)
		}
	}

	// Process deletes
	if len(deleteIDs) > 0 {
		if err := secondaryStore.Delete(ctx, deleteIDs); err != nil {
			log.Printf("[replication_worker] secondary delete error: %v", err)
		}
	}

	// Process upserts
	if len(upsertEvents) > 0 {
		texts := make([]string, len(upsertEvents))
		for i, ev := range upsertEvents {
			texts[i] = ev.Text
			if texts[i] == "" {
				texts[i] = "empty"
			}
		}

		vecs, _, err := secondaryEmbedder.EmbedTexts(ctx, texts)
		if err != nil {
			log.Printf("[replication_worker] secondary embedding error: %v", err)
			for _, ev := range upsertEvents {
				_ = dw.queue.Nack(ev.ID, err.Error())
			}
		} else {
			records := make([]models.VectorRecord, len(upsertEvents))
			for i, ev := range upsertEvents {
				var payload map[string]any
				if len(ev.Payload) > 0 {
					_ = json.Unmarshal(ev.Payload, &payload)
				}
				records[i] = models.VectorRecord{
					ID:      ev.RecordID,
					Vector:  vecs[i],
					Payload: payload,
					Text:    ev.Text,
				}
			}

			if err := secondaryStore.Upsert(ctx, records); err != nil {
				log.Printf("[replication_worker] secondary upsert error: %v", err)
				for _, ev := range upsertEvents {
					_ = dw.queue.Nack(ev.ID, err.Error())
				}
			} else {
				for _, ev := range upsertEvents {
					ackIDs = append(ackIDs, ev.ID)
					_ = dw.queue.MarkDocumentMigrated(ev.RecordID)
				}
			}
		}
	}

	if len(ackIDs) > 0 {
		_ = dw.queue.Ack(ackIDs)
	}
}
