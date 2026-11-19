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

// PgVectorStore interacts with a PostgreSQL pgvector instance via a lightweight REST proxy or HTTP SQL bridge (e.g. PostgREST).
type PgVectorStore struct {
	endpoint   string
	apiKey     string
	table      string
	dimensions int
	client     *http.Client
}

// NewPgVectorStore creates an initialized pgvector store client.
func NewPgVectorStore(endpoint, apiKey, table string, dimensions int) *PgVectorStore {
	cleanEndpoint := strings.TrimRight(endpoint, "/")
	return &PgVectorStore{
		endpoint:   cleanEndpoint,
		apiKey:     apiKey,
		table:      table,
		dimensions: dimensions,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (p *PgVectorStore) Name() string {
	return "pgvector:" + p.table
}

func (p *PgVectorStore) Dimensions() int {
	return p.dimensions
}

func (p *PgVectorStore) Upsert(ctx context.Context, records []models.VectorRecord) error {
	type pgRecord struct {
		ID       string         `json:"id"`
		Embedding string        `json:"embedding"`
		Payload  map[string]any `json:"payload,omitempty"`
		Text     string         `json:"text,omitempty"`
	}

	payloads := make([]pgRecord, 0, len(records))
	for _, r := range records {
		payloads = append(payloads, pgRecord{
			ID:        r.ID,
			Embedding: formatVector(r.Vector),
			Payload:   r.Payload,
			Text:      r.Text,
		})
	}

	bodyData, err := json.Marshal(payloads)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/%s", p.endpoint, p.table)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "resolution=merge-duplicates")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("pgvector upsert request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pgvector upsert failed (%d): %s", resp.StatusCode, string(b))
	}
	return nil
}

func (p *PgVectorStore) Query(ctx context.Context, q models.SearchQuery) (*models.SearchResult, error) {
	start := time.Now()
	topK := q.TopK
	if topK <= 0 {
		topK = 10
	}

	reqPayload := map[string]any{
		"query_vector": formatVector(q.Vector),
		"match_count":  topK,
	}

	bodyData, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/rpc/match_%s", p.endpoint, p.table)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pgvector query request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("pgvector query failed (%d): %s", resp.StatusCode, string(b))
	}

	var results []struct {
		ID      string         `json:"id"`
		Score   float32        `json:"similarity"`
		Payload map[string]any `json:"payload"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}

	scored := make([]models.ScoredRecord, len(results))
	for i, r := range results {
		scored[i] = models.ScoredRecord{
			ID:      r.ID,
			Score:   r.Score,
			Payload: r.Payload,
		}
	}

	return &models.SearchResult{
		Records:   scored,
		LatencyMs: float64(time.Since(start).Microseconds()) / 1000.0,
		StoreName: p.Name(),
	}, nil
}

func (p *PgVectorStore) Delete(ctx context.Context, ids []string) error {
	inList := strings.Join(ids, ",")
	url := fmt.Sprintf("%s/%s?id=in.(%s)", p.endpoint, p.table, inList)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pgvector delete failed (%d): %s", resp.StatusCode, string(b))
	}
	return nil
}

func (p *PgVectorStore) Count(ctx context.Context) (int64, error) {
	url := fmt.Sprintf("%s/%s?select=count", p.endpoint, p.table)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Prefer", "count=exact")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var res []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return 0, err
	}
	return int64(len(res)), nil
}

func (p *PgVectorStore) Scan(ctx context.Context, cursor string, limit int) ([]models.VectorRecord, string, error) {
	if limit <= 0 {
		limit = 100
	}
	offset := 0
	if cursor != "" {
		fmt.Sscanf(cursor, "%d", &offset)
	}

	url := fmt.Sprintf("%s/%s?limit=%d&offset=%d&order=id.asc", p.endpoint, p.table, limit, offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	var rows []struct {
		ID      string         `json:"id"`
		Payload map[string]any `json:"payload"`
		Text    string         `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, "", err
	}

	records := make([]models.VectorRecord, len(rows))
	for i, r := range rows {
		records[i] = models.VectorRecord{
			ID:      r.ID,
			Payload: r.Payload,
			Text:    r.Text,
		}
	}

	var nextCursor string
	if len(rows) == limit {
		nextCursor = fmt.Sprintf("%d", offset+limit)
	}

	return records, nextCursor, nil
}

func (p *PgVectorStore) Close() error {
	p.client.CloseIdleConnections()
	return nil
}

func formatVector(vec []float32) string {
	var sb strings.Builder
	sb.WriteByte('[')
	for i, v := range vec {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(fmt.Sprintf("%g", v))
	}
	sb.WriteByte(']')
	return sb.String()
}
