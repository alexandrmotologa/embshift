package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/alexandrmotologa/embshift/internal/models"
)

// PineconeStore interacts with Pinecone vector database via its HTTP REST API.
type PineconeStore struct {
	endpoint   string
	apiKey     string
	collection string
	dimensions int
	client     *http.Client
}

// NewPineconeStore creates an initialized Pinecone client.
func NewPineconeStore(endpoint, apiKey, collection string, dimensions int) *PineconeStore {
	cleanEndpoint := strings.TrimRight(endpoint, "/")
	return &PineconeStore{
		endpoint:   cleanEndpoint,
		apiKey:     apiKey,
		collection: collection,
		dimensions: dimensions,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (p *PineconeStore) Name() string {
	return "pinecone:" + p.collection
}

func (p *PineconeStore) Dimensions() int {
	return p.dimensions
}

func (p *PineconeStore) Upsert(ctx context.Context, records []models.VectorRecord) error {
	type pineconeVector struct {
		ID       string         `json:"id"`
		Values   []float32      `json:"values"`
		Metadata map[string]any `json:"metadata,omitempty"`
	}

	vectors := make([]pineconeVector, 0, len(records))
	for _, rec := range records {
		meta := rec.Payload
		if meta == nil {
			meta = make(map[string]any)
		}
		if rec.Text != "" {
			meta["text"] = rec.Text
		}
		vectors = append(vectors, pineconeVector{
			ID:       rec.ID,
			Values:   rec.Vector,
			Metadata: meta,
		})
	}

	bodyData, err := json.Marshal(map[string]any{"vectors": vectors})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/vectors/upsert", p.endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Api-Key", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("pinecone upsert request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pinecone upsert failed (status %d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (p *PineconeStore) Query(ctx context.Context, q models.SearchQuery) (*models.SearchResult, error) {
	start := time.Now()
	topK := q.TopK
	if topK <= 0 {
		topK = 10
	}

	reqPayload := map[string]any{
		"vector":          q.Vector,
		"topK":            topK,
		"includeMetadata": true,
	}

	bodyData, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/query", p.endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Api-Key", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pinecone query request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("pinecone query failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var parsed struct {
		Matches []struct {
			ID       string         `json:"id"`
			Score    float32        `json:"score"`
			Metadata map[string]any `json:"metadata"`
		} `json:"matches"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode pinecone query: %w", err)
	}

	records := make([]models.ScoredRecord, len(parsed.Matches))
	for i, m := range parsed.Matches {
		records[i] = models.ScoredRecord{
			ID:      m.ID,
			Score:   m.Score,
			Payload: m.Metadata,
		}
	}

	return &models.SearchResult{
		Records:   records,
		LatencyMs: float64(time.Since(start).Microseconds()) / 1000.0,
		StoreName: p.Name(),
	}, nil
}

func (p *PineconeStore) Delete(ctx context.Context, ids []string) error {
	bodyData, err := json.Marshal(map[string]any{"ids": ids})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/vectors/delete", p.endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Api-Key", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pinecone delete failed (status %d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (p *PineconeStore) Count(ctx context.Context) (int64, error) {
	url := fmt.Sprintf("%s/describe_index_stats", p.endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte("{}")))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Api-Key", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var parsed struct {
		TotalVectorCount int64 `json:"totalVectorCount"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return 0, err
	}
	return parsed.TotalVectorCount, nil
}

func (p *PineconeStore) Scan(ctx context.Context, cursor string, limit int) ([]models.VectorRecord, string, error) {
	if limit <= 0 {
		limit = 100
	}

	reqPayload := map[string]any{
		"limit": limit,
	}
	if cursor != "" {
		reqPayload["paginationToken"] = cursor
	}

	bodyData, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, "", err
	}

	url := fmt.Sprintf("%s/vectors/list", p.endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyData))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Api-Key", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	var parsed struct {
		Vectors []struct {
			ID string `json:"id"`
		} `json:"vectors"`
		Pagination struct {
			Next string `json:"next"`
		} `json:"pagination"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, "", err
	}

	records := make([]models.VectorRecord, len(parsed.Vectors))
	for i, v := range parsed.Vectors {
		records[i] = models.VectorRecord{
			ID: v.ID,
		}
	}

	return records, parsed.Pagination.Next, nil
}

func (p *PineconeStore) Close() error {
	p.client.CloseIdleConnections()
	return nil
}
