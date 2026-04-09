package memory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const embeddingModel = "openai/text-embedding-3-small"

// Embedder converts text to float32 vectors via OpenRouter
type Embedder struct {
	apiKey string
	http   *http.Client
}

func NewEmbedder(apiKey string) *Embedder {
	return &Embedder{apiKey: apiKey, http: &http.Client{}}
}

// Embed returns a vector for a single string
func (e *Embedder) Embed(text string) ([]float32, error) {
	vecs, err := e.EmbedBatch([]string{text})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}
	return vecs[0], nil
}

// EmbedBatch embeds multiple strings in one API call
func (e *Embedder) EmbedBatch(texts []string) ([][]float32, error) {
	body, _ := json.Marshal(map[string]any{
		"model": embeddingModel,
		"input": texts,
	})

	req, err := http.NewRequest("POST",
		"https://openrouter.ai/api/v1/embeddings",
		bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed request: %w", err)
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("embed API %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, fmt.Errorf("embed unmarshal: %w", err)
	}

	vecs := make([][]float32, len(result.Data))
	for i, d := range result.Data {
		vecs[i] = d.Embedding
	}
	return vecs, nil
}

// CosineSimilarity computes similarity between two vectors
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (sqrt(normA) * sqrt(normB))
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 50; i++ {
		z -= (z*z - x) / (2 * z)
	}
	return z
}
