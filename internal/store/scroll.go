package store

import (
	"encoding/json"
	"fmt"
)

// AnomalyNode is an anomaly-tagged node fetched from Qdrant via scroll.
type AnomalyNode struct {
	ID     string
	Label  string
	RunID  string
	Weight float64
	Notes  string
	Vector []float32
}

// FetchAnomalyNodes retrieves up to limit anomaly-tagged nodes using the
// Qdrant scroll API.  If limit ≤ 0 it defaults to 256.
func (c *Client) FetchAnomalyNodes(limit int) ([]*AnomalyNode, error) {
	if limit <= 0 {
		limit = 256
	}

	body := map[string]any{
		"filter": map[string]any{
			"must": []map[string]any{
				{"key": "anomaly", "match": map[string]any{"value": true}},
			},
		},
		"limit":        limit,
		"with_payload": true,
		"with_vector":  true,
	}

	data, err := c.post(fmt.Sprintf("/collections/%s/points/scroll", CollectionNodes), body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Result struct {
			Points []struct {
				ID      any            `json:"id"`
				Payload map[string]any `json:"payload"`
				Vector  []float32      `json:"vector"`
			} `json:"points"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("scroll unmarshal: %w", err)
	}

	nodes := make([]*AnomalyNode, 0, len(result.Result.Points))
	for _, p := range result.Result.Points {
		nodes = append(nodes, &AnomalyNode{
			ID:     fmt.Sprintf("%v", p.ID),
			Label:  strVal(p.Payload, "label"),
			RunID:  strVal(p.Payload, "run_id"),
			Weight: floatVal(p.Payload, "weight"),
			Notes:  strVal(p.Payload, "notes"),
			Vector: p.Vector,
		})
	}
	return nodes, nil
}
