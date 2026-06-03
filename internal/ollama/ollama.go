// Package ollama provides a minimal client for the Ollama HTTP embedding API.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
)

const defaultModel = "nomic-embed-text"

// EmbedDim is the Matryoshka truncation target (matches the poi_vec column).
const EmbedDim = 256

// Client calls the Ollama /api/embed endpoint.
type Client struct {
	baseURL string
	model   string
	http    *http.Client
}

// New creates a Client targeting the given Ollama base URL (e.g. "http://ollama:11434").
func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		model:   defaultModel,
		http:    &http.Client{},
	}
}

// Embed returns EmbedDim-dimensional normalized vectors for each text.
// Use task prefix "search_query: " for query texts and "search_document: " for
// document texts to match the nomic-embed-text convention.
func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(map[string]any{
		"model": c.model,
		"input": texts,
	})
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: embed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		return nil, fmt.Errorf("ollama: embed status %d: %s", resp.StatusCode, buf.String())
	}

	var result struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ollama: embed decode: %w", err)
	}
	if len(result.Embeddings) != len(texts) {
		return nil, fmt.Errorf("ollama: expected %d embeddings, got %d", len(texts), len(result.Embeddings))
	}

	for i, vec := range result.Embeddings {
		result.Embeddings[i] = truncateNormalize(vec, EmbedDim)
	}
	return result.Embeddings, nil
}

// EmbedOne embeds a single text and returns a EmbedDim-dim normalized vector.
func (c *Client) EmbedOne(ctx context.Context, text string) ([]float32, error) {
	vecs, err := c.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return vecs[0], nil
}

func truncateNormalize(vec []float32, dim int) []float32 {
	if len(vec) > dim {
		vec = vec[:dim]
	}
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i, v := range vec {
			vec[i] = float32(float64(v) / norm)
		}
	}
	return vec
}
