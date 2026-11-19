package adapter

import "context"

// Embedder defines the common contract for generating vector representations from text.
type Embedder interface {
	ModelName() string
	Dimensions() int
	EmbedTexts(ctx context.Context, texts []string) ([][]float32, int64, error)
	EmbedQuery(ctx context.Context, text string) ([]float32, int64, error)
	SetPrefixes(documentPrefix, queryPrefix string)
}
