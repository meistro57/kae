package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"time"
)

const Collection = "kae_nodes"

// VectorDim must match embeddings.Dim.
const VectorDim = 128

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

// EnsureCollection creates the collection if it doesn't exist.
func (c *Client) EnsureCollection(name string, dim int) error {
	// check first
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
	req, _ := http.NewRequest("PUT", c.base+"/collections/"+name, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp2, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		return fmt.Errorf("qdrant create collection: status %d", resp2.StatusCode)
	}
	return nil
}

// Upsert stores a single node vector.
func (c *Client) Upsert(name, nodeID string, vector []float32, payload map[string]any) error {
	body, _ := json.Marshal(map[string]any{
		"points": []map[string]any{
			{
				"id":      hashID(nodeID),
				"vector":  vector,
				"payload": payload,
			},
		},
	})
	req, _ := http.NewRequest("PUT", c.base+"/collections/"+name+"/points?wait=true", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("qdrant upsert: status %d", resp.StatusCode)
	}
	return nil
}

// Search returns the labels of the k most similar nodes.
func (c *Client) Search(name string, vector []float32, k int) ([]string, error) {
	body, _ := json.Marshal(map[string]any{
		"vector":       vector,
		"limit":        k,
		"with_payload": true,
	})
	req, _ := http.NewRequest("POST", c.base+"/collections/"+name+"/points/search", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
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

func hashID(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}
