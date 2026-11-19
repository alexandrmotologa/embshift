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
)

// HTTPEmbedder communicates with standard embedding endpoints (OpenAI, Voyage AI, Ollama).
type HTTPEmbedder struct {
	endpoint   string
	apiKey     string
	model      string
	dimensions int
	client     *http.Client
}

// NewHTTPEmbedder initializes an HTTP embedder client.
func NewHTTPEmbedder(endpoint, apiKey, model string, dimensions int) *HTTPEmbedder {
	cleanEndpoint := strings.TrimRight(endpoint, "/")
	if !strings.HasSuffix(cleanEndpoint, "/embeddings") && !strings.Contains(cleanEndpoint, "/v1") {
		cleanEndpoint = cleanEndpoint + "/v1/embeddings"
	} else if strings.HasSuffix(cleanEndpoint, "/v1") {
		cleanEndpoint = cleanEndpoint + "/embeddings"
	}

	return &HTTPEmbedder{
		endpoint:   cleanEndpoint,
		apiKey:     apiKey,
		model:      model,
		dimensions: dimensions,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (h *HTTPEmbedder) ModelName() string {
	return h.model
}

func (h *HTTPEmbedder) Dimensions() int {
	return h.dimensions
}

func (h *HTTPEmbedder) EmbedTexts(ctx context.Context, texts []string) ([][]float32, int64, error) {
	if len(texts) == 0 {
		return [][]float32{}, 0, nil
	}

	reqBody := map[string]any{
		"model": h.model,
		"input": texts,
	}
	if h.dimensions > 0 && strings.Contains(h.model, "text-embedding-3") {
		reqBody["dimensions"] = h.dimensions
	}

	bodyData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.endpoint, bytes.NewReader(bodyData))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.apiKey)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("embedder http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("embedder returned error (status %d): %s", resp.StatusCode, string(b))
	}

	var parsed struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Usage struct {
			TotalTokens int64 `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, 0, fmt.Errorf("decode embedder response: %w", err)
	}

	vectors := make([][]float32, len(texts))
	for _, item := range parsed.Data {
		if item.Index >= 0 && item.Index < len(vectors) {
			vectors[item.Index] = item.Embedding
		}
	}

	return vectors, parsed.Usage.TotalTokens, nil
}

func (h *HTTPEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, int64, error) {
	vecs, tokens, err := h.EmbedTexts(ctx, []string{text})
	if err != nil {
		return nil, 0, err
	}
	if len(vecs) == 0 {
		return nil, 0, fmt.Errorf("no embedding returned for query")
	}
	return vecs[0], tokens, nil
}
