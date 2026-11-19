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

// QdrantStore interacts with Qdrant vector database via its HTTP REST API.
type QdrantStore struct {
	endpoint   string
	apiKey     string
	collection string
	dimensions int
	client     *http.Client
}

// NewQdrantStore creates an initialized Qdrant store client.
func NewQdrantStore(endpoint, apiKey, collection string, dimensions int) *QdrantStore {
	cleanEndpoint := strings.TrimRight(endpoint, "/")
	return &QdrantStore{
		endpoint:   cleanEndpoint,
		apiKey:     apiKey,
		collection: collection,
		dimensions: dimensions,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (q *QdrantStore) Name() string {
	return "qdrant:" + q.collection
}

func (q *QdrantStore) Dimensions() int {
	return q.dimensions
}

func (q *QdrantStore) Upsert(ctx context.Context, records []models.VectorRecord) error {
	type qdrantPoint struct {
		ID      string         `json:"id"`
		Vector  []float32      `json:"vector"`
		Payload map[string]any `json:"payload,omitempty"`
	}

	points := make([]qdrantPoint, 0, len(records))
	for _, rec := range records {
		payload := rec.Payload
		if payload == nil {
			payload = make(map[string]any)
		}
		if rec.Text != "" {
			payload["text"] = rec.Text
		}
		points = append(points, qdrantPoint{
			ID:      rec.ID,
			Vector:  rec.Vector,
			Payload: payload,
		})
	}

	bodyData, err := json.Marshal(map[string]any{"points": points})
	if err != nil {
		return fmt.Errorf("marshal qdrant points: %w", err)
	}

	url := fmt.Sprintf("%s/collections/%s/points?wait=true", q.endpoint, q.collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(bodyData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if q.apiKey != "" {
		req.Header.Set("api-key", q.apiKey)
	}

	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("qdrant upsert request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qdrant upsert failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (q *QdrantStore) Query(ctx context.Context, query models.SearchQuery) (*models.SearchResult, error) {
	start := time.Now()
	topK := query.TopK
	if topK <= 0 {
		topK = 10
	}

	reqPayload := map[string]any{
		"vector":       query.Vector,
		"limit":        topK,
		"with_payload": true,
	}

	bodyData, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/collections/%s/points/search", q.endpoint, q.collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if q.apiKey != "" {
		req.Header.Set("api-key", q.apiKey)
	}

	resp, err := q.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qdrant search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("qdrant search failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var parsed struct {
		Result []struct {
			ID      any            `json:"id"`
			Score   float32        `json:"score"`
			Payload map[string]any `json:"payload"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode qdrant search response: %w", err)
	}

	records := make([]models.ScoredRecord, len(parsed.Result))
	for i, r := range parsed.Result {
		records[i] = models.ScoredRecord{
			ID:      fmt.Sprintf("%v", r.ID),
			Score:   r.Score,
			Payload: r.Payload,
		}
	}

	return &models.SearchResult{
		Records:   records,
		LatencyMs: float64(time.Since(start).Microseconds()) / 1000.0,
		StoreName: q.Name(),
	}, nil
}

func (q *QdrantStore) Delete(ctx context.Context, ids []string) error {
	bodyData, err := json.Marshal(map[string]any{"points": ids})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/collections/%s/points/delete?wait=true", q.endpoint, q.collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if q.apiKey != "" {
		req.Header.Set("api-key", q.apiKey)
	}

	resp, err := q.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qdrant delete failed (status %d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (q *QdrantStore) Count(ctx context.Context) (int64, error) {
	bodyData, _ := json.Marshal(map[string]any{"exact": true})
	url := fmt.Sprintf("%s/collections/%s/points/count", q.endpoint, q.collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyData))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if q.apiKey != "" {
		req.Header.Set("api-key", q.apiKey)
	}

	resp, err := q.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var parsed struct {
		Result struct {
			Count int64 `json:"count"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return 0, err
	}
	return parsed.Result.Count, nil
}

func (q *QdrantStore) Scan(ctx context.Context, cursor string, limit int) ([]models.VectorRecord, string, error) {
	if limit <= 0 {
		limit = 100
	}

	scrollReq := map[string]any{
		"limit":        limit,
		"with_payload": true,
		"with_vector":  true,
	}
	if cursor != "" {
		scrollReq["offset"] = cursor
	}

	bodyData, err := json.Marshal(scrollReq)
	if err != nil {
		return nil, "", err
	}

	url := fmt.Sprintf("%s/collections/%s/points/scroll", q.endpoint, q.collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyData))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if q.apiKey != "" {
		req.Header.Set("api-key", q.apiKey)
	}

	resp, err := q.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	var parsed struct {
		Result struct {
			Points []struct {
				ID      any            `json:"id"`
				Vector  []float32      `json:"vector"`
				Payload map[string]any `json:"payload"`
			} `json:"points"`
			NextPageOffset any `json:"next_page_offset"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, "", err
	}

	records := make([]models.VectorRecord, len(parsed.Result.Points))
	for i, p := range parsed.Result.Points {
		text := ""
		if t, ok := p.Payload["text"].(string); ok {
			text = t
		}
		records[i] = models.VectorRecord{
			ID:      fmt.Sprintf("%v", p.ID),
			Vector:  p.Vector,
			Payload: p.Payload,
			Text:    text,
		}
	}

	nextCursor := ""
	if parsed.Result.NextPageOffset != nil {
		nextCursor = fmt.Sprintf("%v", parsed.Result.NextPageOffset)
	}

	return records, nextCursor, nil
}

func (q *QdrantStore) Close() error {
	q.client.CloseIdleConnections()
	return nil
}
