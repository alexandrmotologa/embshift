package adapter

import (
	"context"

	"github.com/alexandrmotologa/embshift/internal/models"
)

// VectorStore defines the common contract for interacting with vector databases.
type VectorStore interface {
	Name() string
	Dimensions() int
	Upsert(ctx context.Context, records []models.VectorRecord) error
	Query(ctx context.Context, query models.SearchQuery) (*models.SearchResult, error)
	Delete(ctx context.Context, ids []string) error
	Count(ctx context.Context) (int64, error)
	Scan(ctx context.Context, cursor string, limit int) (records []models.VectorRecord, nextCursor string, err error)
	Close() error
}
