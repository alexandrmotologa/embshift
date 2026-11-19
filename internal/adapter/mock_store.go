package adapter

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/alexandrmotologa/embshift/internal/models"
)

// MockStore provides an in-memory thread-safe vector store with cosine similarity ranking.
type MockStore struct {
	mu         sync.RWMutex
	name       string
	dimensions int
	records    map[string]models.VectorRecord
	order      []string
}

// NewMockStore initializes a mock vector collection with designated dimensions.
func NewMockStore(name string, dimensions int) *MockStore {
	return &MockStore{
		name:       name,
		dimensions: dimensions,
		records:    make(map[string]models.VectorRecord),
		order:      make([]string, 0),
	}
}

func (m *MockStore) Name() string {
	return m.name
}

func (m *MockStore) Dimensions() int {
	return m.dimensions
}

func (m *MockStore) Upsert(ctx context.Context, records []models.VectorRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, rec := range records {
		if rec.ID == "" {
			continue
		}
		if len(rec.Vector) != m.dimensions && len(rec.Vector) > 0 {
			return fmt.Errorf("dimension mismatch: expected %d, got %d", m.dimensions, len(rec.Vector))
		}
		if _, exists := m.records[rec.ID]; !exists {
			m.order = append(m.order, rec.ID)
		}
		if rec.CreatedAt.IsZero() {
			rec.CreatedAt = time.Now()
		}
		rec.UpdatedAt = time.Now()
		m.records[rec.ID] = rec
	}
	return nil
}

func (m *MockStore) Query(ctx context.Context, q models.SearchQuery) (*models.SearchResult, error) {
	start := time.Now()
	m.mu.RLock()
	defer m.mu.RUnlock()

	topK := q.TopK
	if topK <= 0 {
		topK = 10
	}

	scored := make([]models.ScoredRecord, 0, len(m.records))
	for _, rec := range m.records {
		score := cosineSimilarity(q.Vector, rec.Vector)
		scored = append(scored, models.ScoredRecord{
			ID:      rec.ID,
			Score:   score,
			Vector:  rec.Vector,
			Payload: rec.Payload,
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	if len(scored) > topK {
		scored = scored[:topK]
	}

	return &models.SearchResult{
		Records:   scored,
		LatencyMs: float64(time.Since(start).Microseconds()) / 1000.0,
		StoreName: m.name,
	}, nil
}

func (m *MockStore) Delete(ctx context.Context, ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	toDelete := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		toDelete[id] = struct{}{}
		delete(m.records, id)
	}

	newOrder := make([]string, 0, len(m.records))
	for _, id := range m.order {
		if _, deleted := toDelete[id]; !deleted {
			newOrder = append(newOrder, id)
		}
	}
	m.order = newOrder
	return nil
}

func (m *MockStore) Count(ctx context.Context) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return int64(len(m.records)), nil
}

func (m *MockStore) Scan(ctx context.Context, cursor string, limit int) ([]models.VectorRecord, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	startIndex := 0
	if cursor != "" {
		if idx, err := strconv.Atoi(cursor); err == nil && idx >= 0 {
			startIndex = idx
		}
	}

	if limit <= 0 {
		limit = 50
	}

	if startIndex >= len(m.order) {
		return []models.VectorRecord{}, "", nil
	}

	endIndex := startIndex + limit
	var nextCursor string
	if endIndex >= len(m.order) {
		endIndex = len(m.order)
		nextCursor = ""
	} else {
		nextCursor = strconv.Itoa(endIndex)
	}

	result := make([]models.VectorRecord, 0, endIndex-startIndex)
	for i := startIndex; i < endIndex; i++ {
		id := m.order[i]
		if rec, ok := m.records[id]; ok {
			result = append(result, rec)
		}
	}

	return result, nextCursor, nil
}

func (m *MockStore) Close() error {
	return nil
}

func cosineSimilarity(a, b []float32) float32 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0.0
	}

	var dot, normA, normB float64
	for i := 0; i < len(a); i++ {
		valA := float64(a[i])
		valB := float64(b[i])
		dot += valA * valB
		normA += valA * valA
		normB += valB * valB
	}

	if normA == 0 || normB == 0 {
		return 0.0
	}

	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}
