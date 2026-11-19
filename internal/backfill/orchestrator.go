package backfill

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/alexandrmotologa/embshift/internal/adapter"
	"github.com/alexandrmotologa/embshift/internal/config"
	"github.com/alexandrmotologa/embshift/internal/models"
	"github.com/alexandrmotologa/embshift/internal/proxy"
	"github.com/alexandrmotologa/embshift/internal/queue"
)

// Orchestrator coordinates the background chunked document migration.
type Orchestrator struct {
	mu             sync.RWMutex
	cfg            *config.Config
	legacyStore    adapter.VectorStore
	targetStore    adapter.VectorStore
	targetEmbedder adapter.Embedder
	queue          *queue.SQLiteQueue
	budget         *BudgetRegulator
	router         *proxy.Router
	cursor         string
	isRunning      bool
	isPaused       bool
	cancelFunc     context.CancelFunc
	totalLegacy    int64
	migratedCount  int64
	startedAt      *time.Time
}

// NewOrchestrator creates a backfill orchestrator instance.
func NewOrchestrator(
	cfg *config.Config,
	legacyStore, targetStore adapter.VectorStore,
	targetEmbedder adapter.Embedder,
	q *queue.SQLiteQueue,
	router *proxy.Router,
) *Orchestrator {
	pricePerMillion := cfg.Embedder.Target.PricePerMillionTokensUSD
	if pricePerMillion <= 0 {
		pricePerMillion = 0.13
	}

	budget := NewBudgetRegulator(cfg.Backfill, pricePerMillion)

	return &Orchestrator{
		cfg:            cfg,
		legacyStore:    legacyStore,
		targetStore:    targetStore,
		targetEmbedder: targetEmbedder,
		queue:          q,
		budget:         budget,
		router:         router,
	}
}

// Start launches the background backfill runner.
func (o *Orchestrator) Start(parentCtx context.Context) error {
	o.mu.Lock()
	if o.isRunning {
		o.mu.Unlock()
		return fmt.Errorf("backfill is already running")
	}

	ctx, cancel := context.WithCancel(parentCtx)
	o.cancelFunc = cancel
	o.isRunning = true
	o.isPaused = false
	now := time.Now()
	o.startedAt = &now
	o.router.SetState(models.StateBackfilling)
	o.mu.Unlock()

	go o.runLoop(ctx)
	return nil
}

// Pause pauses the backfill process while saving the current cursor.
func (o *Orchestrator) Pause() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.isRunning || o.isPaused {
		return
	}
	o.isPaused = true
	if o.cancelFunc != nil {
		o.cancelFunc()
	}
	o.isRunning = false
	o.router.SetState(models.StatePaused)
}

// Resume restarts a paused backfill process.
func (o *Orchestrator) Resume(parentCtx context.Context) error {
	o.mu.Lock()
	if o.isRunning {
		o.mu.Unlock()
		return fmt.Errorf("backfill is already running")
	}
	ctx, cancel := context.WithCancel(parentCtx)
	o.cancelFunc = cancel
	o.isRunning = true
	o.isPaused = false
	o.router.SetState(models.StateBackfilling)
	o.mu.Unlock()

	go o.runLoop(ctx)
	return nil
}

// IsRunning returns whether the backfill worker is currently processing documents.
func (o *Orchestrator) IsRunning() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.isRunning
}

// GetProgress returns document parity counters and completion ratio.
func (o *Orchestrator) GetProgress(ctx context.Context) (migrated int64, total int64, parity float64) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	tot, _ := o.legacyStore.Count(ctx)
	mig, _ := o.queue.CountMigratedDocuments()

	var ratio float64
	if tot > 0 {
		ratio = float64(mig) / float64(tot)
		if ratio > 1.0 {
			ratio = 1.0
		}
	} else if mig > 0 {
		ratio = 1.0
	}

	return mig, tot, ratio
}

// GetBudgetStats returns token usage and spend information.
func (o *Orchestrator) GetBudgetStats() (tokens int64, spentUSD float64, isExhausted bool) {
	return o.budget.GetStats()
}

func (o *Orchestrator) runLoop(ctx context.Context) {
	batchSize := o.cfg.Backfill.BatchSize
	if batchSize <= 0 {
		batchSize = 100
	}
	textField := o.cfg.Backfill.TextField
	if textField == "" {
		textField = "text"
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		o.mu.RLock()
		cursor := o.cursor
		o.mu.RUnlock()

		// Read chunk from legacy store
		records, nextCursor, err := o.legacyStore.Scan(ctx, cursor, batchSize)
		if err != nil {
			log.Printf("[backfill] legacy scan error: %v, retrying in 2s", err)
			time.Sleep(2 * time.Second)
			continue
		}

		if len(records) == 0 && nextCursor == "" {
			// Backfill scan completed
			o.mu.Lock()
			o.isRunning = false
			o.cursor = ""
			o.mu.Unlock()

			// Check if we are ready for cutover or shadow testing
			_, _, parity := o.GetProgress(ctx)
			if parity >= 0.999 {
				o.router.SetState(models.StateReadyForCutover)
			} else {
				o.router.SetState(models.StateShadowTesting)
			}
			return
		}

		// Filter unmigrated records
		var toMigrate []models.VectorRecord
		for _, rec := range records {
			migrated, _ := o.queue.IsDocumentMigrated(rec.ID)
			if !migrated {
				toMigrate = append(toMigrate, rec)
			}
		}

		if len(toMigrate) > 0 {
			texts := make([]string, len(toMigrate))
			for i, r := range toMigrate {
				text := r.Text
				if text == "" && r.Payload != nil {
					if t, ok := r.Payload[textField].(string); ok {
						text = t
					}
				}
				if text == "" {
					text = fmt.Sprintf("document-%s", r.ID)
				}
				texts[i] = text
			}

			// Token budget rate limiter check
			estimatedTokens := len(texts) * 50
			if err := o.budget.Acquire(ctx, estimatedTokens); err != nil {
				log.Printf("[backfill] rate/budget limit reached: %v", err)
				o.Pause()
				return
			}

			// Generate replacement embeddings using target model
			vecs, usedTokens, err := o.targetEmbedder.EmbedTexts(ctx, texts)
			if err != nil {
				log.Printf("[backfill] target embedding generation failed: %v", err)
				time.Sleep(1 * time.Second)
				continue
			}
			o.budget.RecordUsage(usedTokens)

			// Form target records
			targetRecords := make([]models.VectorRecord, len(toMigrate))
			for i, r := range toMigrate {
				targetRecords[i] = models.VectorRecord{
					ID:      r.ID,
					Vector:  vecs[i],
					Payload: r.Payload,
					Text:    texts[i],
				}
			}

			// Upsert to target store
			if err := o.targetStore.Upsert(ctx, targetRecords); err != nil {
				log.Printf("[backfill] target store upsert failed: %v", err)
				time.Sleep(1 * time.Second)
				continue
			}

			// Mark checkpoints
			for _, r := range toMigrate {
				_ = o.queue.MarkDocumentMigrated(r.ID)
			}
		}

		o.mu.Lock()
		o.cursor = nextCursor
		o.mu.Unlock()

		if nextCursor == "" {
			// Finished all records
			o.mu.Lock()
			o.isRunning = false
			o.mu.Unlock()
			o.router.SetState(models.StateReadyForCutover)
			return
		}
	}
}
