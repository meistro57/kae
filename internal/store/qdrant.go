package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"time"
)

const Collection = "kae_nodes"

type Client struct {
	base string
	http *http.Client
}

func NewClient(base string) *Client {
	return &Client{
		base: base,
		http: &http.Client{Timeout: 10 * time.Second},
	}
}

// Point is a single vector record for batch operations.
type Point struct {
	ID      string
	Vector  []float32
	Payload map[string]any
}

// Ping returns nil if Qdrant is reachable.
func (c *Client) Ping() error {
	resp, err := c.http.Get(c.base + "/")
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("qdrant ping: status %d", resp.StatusCode)
	}
	return nil
}

// EnsureCollection creates the collection if it doesn't exist, then creates
// keyword payload indexes on "domain" and "label" (must be done before HNSW builds).
func (c *Client) EnsureCollection(name string, dim int) error {
	resp, err := c.http.Get(c.base + "/collections/" + name)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode == 200 {
		return nil // already exists
	}

	body, _ := json.Marshal(map[string]any{
		"vectors": map[string]any{
			"size":     dim,
			"distance": "Cosine",
		},
	})
	resp2, err := c.doWithRetry("PUT", c.base+"/collections/"+name, body)
	if err != nil {
		return err
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		return fmt.Errorf("qdrant create collection: status %d", resp2.StatusCode)
	}

	// Create payload indexes before any vectors are indexed — adding them later
	// breaks the filterable vector index per Qdrant best practices.
	_ = c.CreatePayloadIndex(name, "domain")
	_ = c.CreatePayloadIndex(name, "label")
	return nil
}

// CreatePayloadIndex creates a keyword index on field in collection.
func (c *Client) CreatePayloadIndex(name, field string) error {
	body, _ := json.Marshal(map[string]any{
		"field_name":   field,
		"field_schema": "keyword",
	})
	resp, err := c.doWithRetry("PUT", c.base+"/collections/"+name+"/index", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 202 {
		return fmt.Errorf("qdrant create index %q: status %d", field, resp.StatusCode)
	}
	return nil
}

// Upsert stores a single node vector. Delegates to UpsertBatch.
func (c *Client) Upsert(name, nodeID string, vector []float32, payload map[string]any) error {
	return c.UpsertBatch(name, []Point{{ID: nodeID, Vector: vector, Payload: payload}})
}

// UpsertBatch stores multiple points in one request with retry.
// Recommended batch size: 64–256 points per Qdrant best practices.
func (c *Client) UpsertBatch(name string, points []Point) error {
	type pointBody struct {
		ID      uint64         `json:"id"`
		Vector  []float32      `json:"vector"`
		Payload map[string]any `json:"payload"`
	}
	pts := make([]pointBody, len(points))
	for i, p := range points {
		pts[i] = pointBody{
			ID:      hashID(p.ID),
			Vector:  p.Vector,
			Payload: p.Payload,
		}
	}
	body, _ := json.Marshal(map[string]any{"points": pts})
	resp, err := c.doWithRetry("PUT", c.base+"/collections/"+name+"/points?wait=true", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("qdrant upsert batch: status %d", resp.StatusCode)
	}
	return nil
}

// Search returns the labels of the k most similar nodes.
// hnsw_ef is set to max(k*4, 64) per search-speed best practices.
func (c *Client) Search(name string, vector []float32, k int) ([]string, error) {
	ef := k * 4
	if ef < 64 {
		ef = 64
	}
	body, _ := json.Marshal(map[string]any{
		"vector":       vector,
		"limit":        k,
		"with_payload": true,
		"params": map[string]any{
			"hnsw_ef": ef,
		},
	})
	resp, err := c.doWithRetry("POST", c.base+"/collections/"+name+"/points/search", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Result []struct {
			Payload map[string]any `json:"payload"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	labels := make([]string, 0, len(result.Result))
	for _, r := range result.Result {
		if label, ok := r.Payload["label"].(string); ok {
			labels = append(labels, label)
		}
	}
	return labels, nil
}

// VectorCount returns the number of vectors stored in the collection.
func (c *Client) VectorCount(name string) (int64, error) {
	resp, err := c.http.Get(c.base + "/collections/" + name)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var info struct {
		Result struct {
			PointsCount int64 `json:"points_count"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return 0, err
	}
	return info.Result.PointsCount, nil
}

// doWithRetry executes an HTTP request with up to 3 attempts and exponential backoff.
// Retries on network errors and 5xx responses.
func (c *Client) doWithRetry(method, url string, body []byte) (*http.Response, error) {
	delays := []time.Duration{100 * time.Millisecond, 300 * time.Millisecond}
	var lastErr error
	for attempt := 0; attempt <= len(delays); attempt++ {
		if attempt > 0 {
			time.Sleep(delays[attempt-1])
		}
		req, err := http.NewRequest(method, url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode >= 500 {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("qdrant: status %d", resp.StatusCode)
			continue
		}
		return resp, nil
	}
	return nil, lastErr
}

func hashID(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}
