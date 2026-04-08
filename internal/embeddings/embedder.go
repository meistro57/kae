// Package embeddings provides text-to-vector embedding.
// When EMBEDDINGS_URL and EMBEDDINGS_KEY are configured it calls any
// OpenAI-compatible /v1/embeddings endpoint (e.g. OpenAI, Azure, Ollama).
// Otherwise it falls back to API-free feature hashing.
package embeddings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"net/http"
	"strings"
	"time"
)

// Embedder converts text to a float32 vector.
type Embedder interface {
	Embed(text string) ([]float32, error)
	Dim() int
}

// New returns an APIEmbedder when url and key are provided, otherwise a HashEmbedder.
func New(url, key, model string) Embedder {
	if url != "" && key != "" {
		if model == "" {
			model = "text-embedding-3-small"
		}
		return &APIEmbedder{
			url:   strings.TrimRight(url, "/"),
			key:   key,
			model: model,
			dim:   1536,
			http:  &http.Client{Timeout: 30 * time.Second},
		}
	}
	return HashEmbedder{}
}

// ── API embedder ──────────────────────────────────────────────────────────────

// APIEmbedder calls any OpenAI-compatible /v1/embeddings endpoint.
type APIEmbedder struct {
	url   string
	key   string
	model string
	dim   int
	http  *http.Client
}

func (a *APIEmbedder) Dim() int { return a.dim }

func (a *APIEmbedder) Embed(text string) ([]float32, error) {
	body, _ := json.Marshal(map[string]any{
		"input": text,
		"model": a.model,
	})
	req, _ := http.NewRequest("POST", a.url+"/v1/embeddings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.key)

	resp, err := a.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embeddings API: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("embeddings API decode: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("embeddings API: %s", result.Error.Message)
	}
	if len(result.Data) == 0 || len(result.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("embeddings API returned empty result")
	}
	return result.Data[0].Embedding, nil
}

// ── Feature-hash fallback ─────────────────────────────────────────────────────

const hashDim = 128

// HashEmbedder uses feature hashing (random indexing) — no external service.
type HashEmbedder struct{}

func (HashEmbedder) Dim() int { return hashDim }

func (HashEmbedder) Embed(text string) ([]float32, error) {
	vec := make([]float32, hashDim)
	words := strings.Fields(strings.ToLower(text))
	if len(words) == 0 {
		return vec, nil
	}
	for _, word := range words {
		h1 := fnv.New32a()
		h1.Write([]byte(word))
		idx := int(h1.Sum32() % uint32(hashDim))

		h2 := fnv.New32a()
		h2.Write([]byte(word + "\x00"))
		if h2.Sum32()%2 == 0 {
			vec[idx] += 1
		} else {
			vec[idx] -= 1
		}
	}
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	if norm > 0 {
		scale := float32(1.0 / math.Sqrt(norm))
		for i := range vec {
			vec[i] *= scale
		}
	}
	return vec, nil
}
