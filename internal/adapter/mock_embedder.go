package adapter

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math"
	"strings"
	"sync"
)

// MockEmbedder produces deterministic unit-normalized float32 vectors for testing and simulation.
type MockEmbedder struct {
	mu             sync.RWMutex
	modelName      string
	dimensions     int
	seed           uint64
	documentPrefix string
	queryPrefix    string
}

// NewMockEmbedder creates an embedder projecting text into fixed-dimension unit vectors.
func NewMockEmbedder(modelName string, dimensions int) *MockEmbedder {
	h := sha256.Sum256([]byte(modelName))
	seed := binary.BigEndian.Uint64(h[:8])
	return &MockEmbedder{
		modelName:  modelName,
		dimensions: dimensions,
		seed:       seed,
	}
}

func (m *MockEmbedder) ModelName() string {
	return m.modelName
}

func (m *MockEmbedder) Dimensions() int {
	return m.dimensions
}

func (m *MockEmbedder) SetPrefixes(documentPrefix, queryPrefix string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.documentPrefix = documentPrefix
	m.queryPrefix = queryPrefix
}

func (m *MockEmbedder) EmbedTexts(ctx context.Context, texts []string) ([][]float32, int64, error) {
	m.mu.RLock()
	prefix := m.documentPrefix
	m.mu.RUnlock()

	vectors := make([][]float32, len(texts))
	var totalTokens int64
	for i, text := range texts {
		formatted := prefix + text
		vectors[i] = m.generateVector(formatted)
		tok := int64(len(formatted) / 4)
		if tok < 1 {
			tok = 1
		}
		totalTokens += tok
	}
	return vectors, totalTokens, nil
}

func (m *MockEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, int64, error) {
	m.mu.RLock()
	prefix := m.queryPrefix
	m.mu.RUnlock()

	formatted := prefix + text
	v := m.generateVector(formatted)
	tok := int64(len(formatted) / 4)
	if tok < 1 {
		tok = 1
	}
	return v, tok, nil
}

func (m *MockEmbedder) generateVector(text string) []float32 {
	vec := make([]float32, m.dimensions)
	words := strings.Fields(strings.ToLower(text))
	if len(words) == 0 {
		words = []string{"empty"}
	}

	for _, word := range words {
		h := sha256.Sum256([]byte(word))
		wordHash := binary.BigEndian.Uint64(h[:8]) ^ m.seed
		for i := 0; i < m.dimensions; i++ {
			wordHash = wordHash*6364136223846793005 + 1442695040888963407
			val := float32(float64(int64(wordHash)%1000) / 1000.0)
			vec[i] += val
		}
	}

	var sumSq float64
	for _, val := range vec {
		sumSq += float64(val * val)
	}
	if sumSq > 0 {
		norm := float32(math.Sqrt(sumSq))
		for i := range vec {
			vec[i] /= norm
		}
	}

	return vec
}
