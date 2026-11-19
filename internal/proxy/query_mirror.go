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
	"github.com/alexandrmotologa/embshift/internal/shadow"
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

// Query executes search with canary splitting support and asynchronous shadow evaluation.
func (qm *QueryMirror) Query(ctx context.Context, q models.SearchQuery) (*models.SearchResult, error) {
	canaryWeight := qm.router.GetCanaryWeight()

	// 1. Check if query should take Canary path to target store
	if canaryWeight > 0 && rand.Float64() < canaryWeight {
		targetStore := qm.router.GetSecondaryStore()
		targetEmbedder := qm.router.GetSecondaryEmbedder()
		if qm.router.GetActivePrimary() == "target" {
			targetStore = qm.router.GetPrimaryStore()
			targetEmbedder = qm.router.GetPrimaryEmbedder()
		}

		targetVec := q.Vector
		if len(targetVec) == 0 && q.Text != "" {
			vec, _, err := targetEmbedder.EmbedQuery(ctx, q.Text)
			if err == nil {
				targetVec = vec
			}
		}

		if len(targetVec) > 0 {
			targetResult, err := targetStore.Query(ctx, models.SearchQuery{
				Vector: targetVec,
				Text:   q.Text,
				TopK:   q.TopK,
				Filter: q.Filter,
			})

			if err == nil {
				targetResult.IsCanary = true
				qm.router.RecordCanaryResult(false)

				// Mirror back to legacy in background for drift scoring
				if q.Text != "" {
					go qm.runShadowComparison(context.Background(), q.Text, q.TopK, targetResult, true)
				}
				return targetResult, nil
			}

			// Target search failed, record canary error and fall back to legacy
			qm.router.RecordCanaryResult(true)
		}
	}

	// 2. Normal Primary search
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

	primaryResult, err := primaryStore.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("primary query failed: %w", err)
	}

	// 3. Sampled asynchronous shadow evaluation
	sampleRatio := qm.cfg.Shadow.SampleRatio
	if sampleRatio <= 0 {
		sampleRatio = 0.25
	}

	if q.Text != "" && (sampleRatio >= 1.0 || rand.Float64() <= sampleRatio) {
		go qm.runShadowComparison(context.Background(), q.Text, q.TopK, primaryResult, false)
	}

	return primaryResult, nil
}

func (qm *QueryMirror) runShadowComparison(ctx context.Context, queryText string, topK int, baselineResult *models.SearchResult, baselineIsTarget bool) {
	secondaryStore := qm.router.GetSecondaryStore()
	secondaryEmbedder := qm.router.GetSecondaryEmbedder()

	if topK <= 0 {
		topK = 10
	}

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

	var legacyRecords, targetRecords []models.ScoredRecord
	var legacyLatency, targetLatency float64

	if baselineIsTarget {
		targetRecords = baselineResult.Records
		targetLatency = baselineResult.LatencyMs
		legacyRecords = secResult.Records
		legacyLatency = secResult.LatencyMs
	} else {
		legacyRecords = baselineResult.Records
		legacyLatency = baselineResult.LatencyMs
		targetRecords = secResult.Records
		targetLatency = secResult.LatencyMs
	}

	jaccard := shadow.ComputeJaccardAtK(legacyRecords, targetRecords)
	ndcg := shadow.ComputeNDCGAtK(legacyRecords, targetRecords, topK)

	queryID := fmt.Sprintf("q-%d", time.Now().UnixNano())
	comparison := models.ShadowComparison{
		QueryID:           queryID,
		QueryText:         queryText,
		LegacyResults:     legacyRecords,
		TargetResults:     targetRecords,
		JaccardSimilarity: jaccard,
		NDCGAtK:           ndcg,
		LegacyLatencyMs:   legacyLatency,
		TargetLatencyMs:   targetLatency,
		Timestamp:         time.Now().UTC(),
	}

	_ = qm.queue.RecordShadowQuery(queryID, queryText, jaccard, legacyLatency, targetLatency)

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
