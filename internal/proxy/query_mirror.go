package proxy

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/alexandrmotologa/embshift/internal/config"
	"github.com/alexandrmotologa/embshift/internal/models"
	"github.com/alexandrmotologa/embshift/internal/queue"
)

// QueryMirror routes production queries to the primary store while asynchronously mirroring traffic to the shadow store.
type QueryMirror struct {
	router         *Router
	cfg            *config.Config
	queue          *queue.SQLiteQueue
	mu             sync.RWMutex
	recentShadows  []models.ShadowComparison
	maxRecentCount int
}

// NewQueryMirror initializes a query traffic splitter with recent comparison history buffer.
func NewQueryMirror(router *Router, cfg *config.Config, q *queue.SQLiteQueue) *QueryMirror {
	return &QueryMirror{
		router:         router,
		cfg:            cfg,
		queue:          q,
		recentShadows:  make([]models.ShadowComparison, 0, 50),
		maxRecentCount: 50,
	}
}

// Query executes search against the active primary and asynchronously mirrors to shadow store.
func (qm *QueryMirror) Query(ctx context.Context, q models.SearchQuery) (*models.SearchResult, error) {
	primaryStore := qm.router.GetPrimaryStore()
	primaryEmbedder := qm.router.GetPrimaryEmbedder()

	queryVec := q.Vector
	if len(queryVec) == 0 && q.Text != "" {
		vec, _, err := primaryEmbedder.EmbedQuery(ctx, q.Text)
		if err != nil {
			return nil, fmt.Errorf("embed primary query: %w", err)
		}
		queryVec = vec
		q.Vector = vec
	}

	// 1. Synchronous primary search
	primaryResult, err := primaryStore.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("primary query failed: %w", err)
	}

	// 2. Sampled asynchronous shadow evaluation
	sampleRatio := qm.cfg.Shadow.SampleRatio
	if sampleRatio <= 0 {
		sampleRatio = 0.25
	}

	if q.Text != "" && (sampleRatio >= 1.0 || rand.Float64() <= sampleRatio) {
		go qm.runShadowComparison(context.Background(), q.Text, q.TopK, primaryResult)
	}

	return primaryResult, nil
}

func (qm *QueryMirror) runShadowComparison(ctx context.Context, queryText string, topK int, primaryResult *models.SearchResult) {
	secondaryStore := qm.router.GetSecondaryStore()
	secondaryEmbedder := qm.router.GetSecondaryEmbedder()

	if topK <= 0 {
		topK = 10
	}

	// Embed query with secondary embedder
	secVec, _, err := secondaryEmbedder.EmbedQuery(ctx, queryText)
	if err != nil {
		return
	}

	secResult, err := secondaryStore.Query(ctx, models.SearchQuery{
		Vector: secVec,
		Text:   queryText,
		TopK:   topK,
	})
	if err != nil {
		return
	}

	// Calculate Jaccard similarity @ K
	jaccard := calculateJaccardSimilarity(primaryResult.Records, secResult.Records)

	queryID := fmt.Sprintf("q-%d", time.Now().UnixNano())
	comparison := models.ShadowComparison{
		QueryID:           queryID,
		QueryText:         queryText,
		LegacyResults:     primaryResult.Records,
		TargetResults:     secResult.Records,
		JaccardSimilarity: jaccard,
		LegacyLatencyMs:   primaryResult.LatencyMs,
		TargetLatencyMs:   secResult.LatencyMs,
		Timestamp:         time.Now().UTC(),
	}

	// Persist to SQLite
	_ = qm.queue.RecordShadowQuery(queryID, queryText, jaccard, primaryResult.LatencyMs, secResult.LatencyMs)

	// Append to recent in-memory circular buffer
	qm.mu.Lock()
	if len(qm.recentShadows) >= qm.maxRecentCount {
		qm.recentShadows = qm.recentShadows[1:]
	}
	qm.recentShadows = append(qm.recentShadows, comparison)
	qm.mu.Unlock()
}

// GetRecentShadowComparisons returns the latest mirrored query evaluation results.
func (qm *QueryMirror) GetRecentShadowComparisons() []models.ShadowComparison {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	out := make([]models.ShadowComparison, len(qm.recentShadows))
	copy(out, qm.recentShadows)
	return out
}

func calculateJaccardSimilarity(a, b []models.ScoredRecord) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}

	setA := make(map[string]struct{}, len(a))
	for _, rec := range a {
		setA[rec.ID] = struct{}{}
	}

	intersection := 0
	union := make(map[string]struct{}, len(a)+len(b))
	for k := range setA {
		union[k] = struct{}{}
	}

	for _, rec := range b {
		union[rec.ID] = struct{}{}
		if _, exists := setA[rec.ID]; exists {
			intersection++
		}
	}

	if len(union) == 0 {
		return 0.0
	}

	return float64(intersection) / float64(len(union))
}
