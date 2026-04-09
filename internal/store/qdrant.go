package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	CollectionChunks = "kae_chunks"  // raw ingested source chunks
	CollectionNodes  = "kae_nodes"   // graph nodes across all runs
	VectorDim        = 1536          // OpenRouter text-embedding-3-small
)

// Client wraps the Qdrant REST API
type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// ── Chunk storage ─────────────────────────────────────────────────────────────

// Chunk is a source passage stored in Qdrant
type Chunk struct {
	ID       string
	Text     string
	Source   string  // URL or title
	Topic    string  // concept this chunk relates to
	RunID    string  // which KAE run produced this
	Vector   []float32
}

// StoreChunk upserts a text chunk with its embedding
func (c *Client) StoreChunk(chunk *Chunk) error {
	point := map[string]any{
		"id": pointID(chunk.ID),
		"vector": chunk.Vector,
		"payload": map[string]any{
			"text":   chunk.Text,
			"source": chunk.Source,
			"topic":  chunk.Topic,
			"run_id": chunk.RunID,
		},
	}
	return c.upsertPoints(CollectionChunks, []map[string]any{point})
}

// SearchChunks finds the top-k semantically similar chunks to a query vector
func (c *Client) SearchChunks(vector []float32, topK int, filter map[string]any) ([]*Chunk, error) {
	body := map[string]any{
		"vector":       vector,
		"limit":        topK,
		"with_payload": true,
	}
	if filter != nil {
		body["filter"] = filter
	}

	data, err := c.post(fmt.Sprintf("/collections/%s/points/search", CollectionChunks), body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Result []struct {
			ID      any            `json:"id"`
			Score   float64        `json:"score"`
			Payload map[string]any `json:"payload"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("search unmarshal: %w", err)
	}

	chunks := make([]*Chunk, 0, len(result.Result))
	for _, r := range result.Result {
		chunks = append(chunks, &Chunk{
			ID:     fmt.Sprintf("%v", r.ID),
			Text:   strVal(r.Payload, "text"),
			Source: strVal(r.Payload, "source"),
			Topic:  strVal(r.Payload, "topic"),
			RunID:  strVal(r.Payload, "run_id"),
		})
	}
	return chunks, nil
}

// ── Node persistence ──────────────────────────────────────────────────────────

// NodeRecord is a persisted graph node across runs
type NodeRecord struct {
	ID         string
	Label      string
	Domain     string
	RunID      string
	Weight     float64
	Anomaly    bool
	Sources    []string
	Cycle      int
	Vector     []float32  // embedding of the node label
}

// StoreNode persists a graph node with its embedding for cross-run comparison
func (c *Client) StoreNode(node *NodeRecord) error {
	point := map[string]any{
		"id": pointID(node.ID),
		"vector": node.Vector,
		"payload": map[string]any{
			"label":   node.Label,
			"domain":  node.Domain,
			"run_id":  node.RunID,
			"weight":  node.Weight,
			"anomaly": node.Anomaly,
			"sources": node.Sources,
			"cycle":   node.Cycle,
		},
	}
	return c.upsertPoints(CollectionNodes, []map[string]any{point})
}

// FindSimilarNodes finds semantically similar nodes — used for cross-run convergence
func (c *Client) FindSimilarNodes(vector []float32, topK int, excludeRunID string) ([]*NodeRecord, error) {
	body := map[string]any{
		"vector":       vector,
		"limit":        topK,
		"with_payload": true,
	}

	// Optionally exclude the current run to find cross-run matches
	if excludeRunID != "" {
		body["filter"] = map[string]any{
			"must_not": []map[string]any{
				{"key": "run_id", "match": map[string]any{"value": excludeRunID}},
			},
		}
	}

	data, err := c.post(fmt.Sprintf("/collections/%s/points/search", CollectionNodes), body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Result []struct {
			ID      any            `json:"id"`
			Score   float64        `json:"score"`
			Payload map[string]any `json:"payload"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("node search unmarshal: %w", err)
	}

	nodes := make([]*NodeRecord, 0, len(result.Result))
	for _, r := range result.Result {
		nodes = append(nodes, &NodeRecord{
			ID:     fmt.Sprintf("%v", r.ID),
			Label:  strVal(r.Payload, "label"),
			Domain: strVal(r.Payload, "domain"),
			RunID:  strVal(r.Payload, "run_id"),
			Weight: floatVal(r.Payload, "weight"),
			Anomaly: boolVal(r.Payload, "anomaly"),
			Cycle:  intVal(r.Payload, "cycle"),
		})
	}
	return nodes, nil
}

// ── Collection management ─────────────────────────────────────────────────────

// EnsureCollections creates collections if they don't exist
func (c *Client) EnsureCollections() error {
	for _, name := range []string{CollectionChunks, CollectionNodes} {
		if err := c.ensureCollection(name); err != nil {
			return fmt.Errorf("ensure %s: %w", name, err)
		}
	}
	return nil
}

func (c *Client) ensureCollection(name string) error {
	// Check if exists
	resp, err := c.http.Get(c.baseURL + "/collections/" + name)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode == 200 {
		return nil // already exists
	}

	// Create it
	body := map[string]any{
		"vectors": map[string]any{
			"size":     VectorDim,
			"distance": "Cosine",
		},
	}
	_, err = c.put("/collections/"+name, body)
	return err
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

func (c *Client) upsertPoints(collection string, points []map[string]any) error {
	_, err := c.put(
		fmt.Sprintf("/collections/%s/points", collection),
		map[string]any{"points": points},
	)
	return err
}

func (c *Client) post(path string, body any) ([]byte, error) {
	return c.do("POST", path, body)
}

func (c *Client) put(path string, body any) ([]byte, error) {
	return c.do("PUT", path, body)
}

func (c *Client) do(method, path string, body any) ([]byte, error) {
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(method, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qdrant %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("qdrant %s %s → %d: %s", method, path, resp.StatusCode, data)
	}
	return data, nil
}

// ── Payload helpers ───────────────────────────────────────────────────────────

func strVal(m map[string]any, k string) string {
	if v, ok := m[k]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func floatVal(m map[string]any, k string) float64 {
	if v, ok := m[k]; ok {
		if f, ok := v.(float64); ok {
			return f
		}
	}
	return 0
}

func intVal(m map[string]any, k string) int {
	if v, ok := m[k]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return 0
}

func boolVal(m map[string]any, k string) bool {
	if v, ok := m[k]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// pointID converts a string key to a uint64 for Qdrant compatibility
func pointID(s string) uint64 {
	h := uint64(14695981039346656037)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}
