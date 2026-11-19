package shadow

import (
	"math"

	"github.com/alexandrmotologa/embshift/internal/models"
)

// ComputeJaccardAtK calculates semantic overlap between two result sets of top-K records.
func ComputeJaccardAtK(legacy, target []models.ScoredRecord) float64 {
	if len(legacy) == 0 && len(target) == 0 {
		return 1.0
	}
	if len(legacy) == 0 || len(target) == 0 {
		return 0.0
	}

	legacySet := make(map[string]struct{}, len(legacy))
	for _, r := range legacy {
		legacySet[r.ID] = struct{}{}
	}

	intersection := 0
	union := make(map[string]struct{}, len(legacy)+len(target))
	for k := range legacySet {
		union[k] = struct{}{}
	}

	for _, r := range target {
		union[r.ID] = struct{}{}
		if _, exists := legacySet[r.ID]; exists {
			intersection++
		}
	}

	if len(union) == 0 {
		return 0.0
	}

	return float64(intersection) / float64(len(union))
}

// ComputeNDCGAtK calculates Normalized Discounted Cumulative Gain at rank K.
// Using legacy ranking as relevance baseline (relevance = 1.0 / log2(rank + 1)).
func ComputeNDCGAtK(legacy, target []models.ScoredRecord, k int) float64 {
	if len(legacy) == 0 || len(target) == 0 || k <= 0 {
		return 0.0
	}

	if k > len(legacy) {
		k = len(legacy)
	}
	if k > len(target) {
		k = len(target)
	}

	// Assign relevance based on position in legacy ranking
	relevance := make(map[string]float64, len(legacy))
	for i, r := range legacy {
		relevance[r.ID] = float64(len(legacy) - i)
	}

	// Compute DCG for target ranking
	var dcg float64
	for i := 0; i < k; i++ {
		rel := relevance[target[i].ID]
		dcg += rel / math.Log2(float64(i+2))
	}

	// Compute Ideal DCG (IDCG) for legacy ranking
	var idcg float64
	for i := 0; i < k; i++ {
		rel := relevance[legacy[i].ID]
		idcg += rel / math.Log2(float64(i+2))
	}

	if idcg == 0 {
		return 0.0
	}

	ndcg := dcg / idcg
	if ndcg > 1.0 {
		ndcg = 1.0
	}
	return ndcg
}

// LatencyDeltaMs returns target latency minus legacy latency.
func LatencyDeltaMs(legacyMs, targetMs float64) float64 {
	return targetMs - legacyMs
}
