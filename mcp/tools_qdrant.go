package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// ── Additional HTTP helpers ───────────────────────────────────────────────────

func qdrantPut(path string, body any) (map[string]any, error) {
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPut, qdrantURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	var result map[string]any
	if err := json.Unmarshal(rb, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func qdrantDeleteReq(path string) (map[string]any, error) {
	req, err := http.NewRequest(http.MethodDelete, qdrantURL+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	var result map[string]any
	if err := json.Unmarshal(rb, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ── Argument parsing helpers ──────────────────────────────────────────────────

// parsePointID converts a string to uint64 if numeric, else returns the string as-is.
// Qdrant accepts either numeric IDs or UUID strings.
func parsePointID(idStr string) any {
	if id, err := strconv.ParseUint(idStr, 10, 64); err == nil {
		return id
	}
	return idStr
}

// stringArray converts []any (from JSON) to []string.
func stringArray(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, elem := range arr {
		if s, ok := elem.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// float32Array converts []any (from JSON) to []float32.
func float32Array(v any) []float32 {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]float32, len(arr))
	for i, elem := range arr {
		if f, ok := elem.(float64); ok {
			out[i] = float32(f)
		}
	}
	return out
}

// qdrantError extracts an error string from a Qdrant response envelope.
// Returns "" on success (status == "ok").
func qdrantError(data map[string]any) string {
	if status, ok := data["status"].(map[string]any); ok {
		if errStr, ok := status["error"].(string); ok && errStr != "" {
			return errStr
		}
	}
	return ""
}

// ── Collection management ─────────────────────────────────────────────────────

func toolCollectionInfo(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("collection_name is required")
	}
	data, err := qdrantGet("/collections/" + name)
	if err != nil {
		return "", fmt.Errorf("qdrant unreachable: %w", err)
	}
	if msg := qdrantError(data); msg != "" {
		return "", fmt.Errorf("collection error: %s", msg)
	}

	result, _ := data["result"].(map[string]any)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Collection: %s\n\n", name))

	if pc, ok := result["points_count"].(float64); ok {
		sb.WriteString(fmt.Sprintf("- **Points:** %d\n", int(pc)))
	}
	if ivc, ok := result["indexed_vectors_count"].(float64); ok {
		sb.WriteString(fmt.Sprintf("- **Indexed vectors:** %d\n", int(ivc)))
	}
	if sc, ok := result["segments_count"].(float64); ok {
		sb.WriteString(fmt.Sprintf("- **Segments:** %d\n", int(sc)))
	}
	if st, ok := result["status"].(string); ok {
		sb.WriteString(fmt.Sprintf("- **Status:** %s\n", st))
	}

	// Vector config (unnamed vectors layout)
	if config, ok := result["config"].(map[string]any); ok {
		if params, ok := config["params"].(map[string]any); ok {
			if vectors, ok := params["vectors"].(map[string]any); ok {
				if size, ok := vectors["size"].(float64); ok {
					sb.WriteString(fmt.Sprintf("- **Vector size:** %d\n", int(size)))
				}
				if dist, ok := vectors["distance"].(string); ok {
					sb.WriteString(fmt.Sprintf("- **Distance:** %s\n", dist))
				}
				if onDisk, ok := vectors["on_disk"].(bool); ok {
					sb.WriteString(fmt.Sprintf("- **On disk:** %v\n", onDisk))
				}
			}
			if onDiskPayload, ok := params["on_disk_payload"].(bool); ok {
				sb.WriteString(fmt.Sprintf("- **Payload on disk:** %v\n", onDiskPayload))
			}
		}
		if opt, ok := config["optimizer_config"].(map[string]any); ok {
			if thresh, ok := opt["indexing_threshold"].(float64); ok {
				sb.WriteString(fmt.Sprintf("- **Indexing threshold:** %d\n", int(thresh)))
			}
		}
	}

	return sb.String(), nil
}

func toolCreateCollection(name, distance string, vectorSize int, onDisk bool) (string, error) {
	if name == "" {
		return "", fmt.Errorf("collection_name is required")
	}
	if vectorSize <= 0 {
		return "", fmt.Errorf("vector_size must be a positive integer")
	}
	if distance == "" {
		distance = "Cosine"
	}
	switch distance {
	case "Cosine", "Euclid", "Dot", "Manhattan":
		// valid
	default:
		return "", fmt.Errorf("unsupported distance %q — use Cosine, Euclid, Dot, or Manhattan", distance)
	}

	body := map[string]any{
		"vectors": map[string]any{
			"size":     vectorSize,
			"distance": distance,
			"on_disk":  onDisk,
		},
	}
	data, err := qdrantPut("/collections/"+name, body)
	if err != nil {
		return "", fmt.Errorf("qdrant unreachable: %w", err)
	}
	if msg := qdrantError(data); msg != "" {
		return "", fmt.Errorf("create collection failed: %s", msg)
	}

	return fmt.Sprintf("## Collection Created\n\n**%s** created successfully.\n- Vector size: %d\n- Distance: %s\n- On disk: %v\n",
		name, vectorSize, distance, onDisk), nil
}

func toolDeleteCollection(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("collection_name is required")
	}
	data, err := qdrantDeleteReq("/collections/" + name)
	if err != nil {
		return "", fmt.Errorf("qdrant unreachable: %w", err)
	}
	if msg := qdrantError(data); msg != "" {
		return "", fmt.Errorf("delete collection failed: %s", msg)
	}
	return fmt.Sprintf("## Collection Deleted\n\n**%s** has been permanently deleted.\n", name), nil
}

// ── Point operations ──────────────────────────────────────────────────────────

func toolScrollPoints(collection string, limit int, offset string, withPayload, withVector bool, filter map[string]any) (string, error) {
	if collection == "" {
		return "", fmt.Errorf("collection_name is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	body := map[string]any{
		"limit":        limit,
		"with_payload": withPayload,
		"with_vector":  withVector,
	}
	if offset != "" {
		body["offset"] = parsePointID(offset)
	}
	if len(filter) > 0 {
		body["filter"] = filter
	}

	data, err := qdrantPost("/collections/"+collection+"/points/scroll", body)
	if err != nil {
		return "", fmt.Errorf("qdrant unreachable: %w", err)
	}
	if msg := qdrantError(data); msg != "" {
		return "", fmt.Errorf("scroll failed: %s", msg)
	}

	result, _ := data["result"].(map[string]any)
	points, _ := result["points"].([]any)
	nextOffset := result["next_page_offset"]

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Scroll: %s\n\n", collection))
	sb.WriteString(fmt.Sprintf("Returned **%d** points\n\n", len(points)))
	if nextOffset != nil {
		sb.WriteString(fmt.Sprintf("> **Next page offset:** `%v` — pass as `offset` to continue\n\n", nextOffset))
	} else {
		sb.WriteString("> End of collection (no more pages).\n\n")
	}

	for _, p := range points {
		pt, ok := p.(map[string]any)
		if !ok {
			continue
		}
		sb.WriteString(fmt.Sprintf("### Point `%v`\n", pt["id"]))
		if withPayload {
			if payload, ok := pt["payload"].(map[string]any); ok && len(payload) > 0 {
				formatPayload(&sb, payload)
			}
		}
		if withVector {
			if vec, ok := pt["vector"].([]any); ok {
				sb.WriteString(fmt.Sprintf("  _vector: [%d dims]_\n", len(vec)))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

func toolGetPoint(collection, pointID string, withPayload, withVector bool) (string, error) {
	if collection == "" || pointID == "" {
		return "", fmt.Errorf("collection_name and point_id are required")
	}

	// Qdrant REST: GET /collections/{name}/points/{id}
	// The id segment works for both numeric strings and UUIDs.
	data, err := qdrantGet(fmt.Sprintf("/collections/%s/points/%s", collection, pointID))
	if err != nil {
		return "", fmt.Errorf("qdrant unreachable: %w", err)
	}
	if msg := qdrantError(data); msg != "" {
		return "", fmt.Errorf("get point failed: %s", msg)
	}

	result, _ := data["result"].(map[string]any)
	if result == nil {
		return fmt.Sprintf("Point `%s` not found in `%s`.\n", pointID, collection), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Point `%v` — %s\n\n", result["id"], collection))
	if withPayload {
		if payload, ok := result["payload"].(map[string]any); ok && len(payload) > 0 {
			sb.WriteString("### Payload\n\n")
			formatPayload(&sb, payload)
		} else {
			sb.WriteString("_No payload._\n")
		}
	}
	if withVector {
		if vec, ok := result["vector"].([]any); ok {
			sb.WriteString(fmt.Sprintf("\n**Vector:** [%d dimensions]\n", len(vec)))
		}
	}
	return sb.String(), nil
}

func toolGetPoints(collection string, pointIDs []string, withPayload, withVector bool) (string, error) {
	if collection == "" || len(pointIDs) == 0 {
		return "", fmt.Errorf("collection_name and point_ids are required")
	}

	ids := make([]any, len(pointIDs))
	for i, id := range pointIDs {
		ids[i] = parsePointID(id)
	}

	body := map[string]any{
		"ids":          ids,
		"with_payload": withPayload,
		"with_vector":  withVector,
	}
	data, err := qdrantPost("/collections/"+collection+"/points", body)
	if err != nil {
		return "", fmt.Errorf("qdrant unreachable: %w", err)
	}
	if msg := qdrantError(data); msg != "" {
		return "", fmt.Errorf("get points failed: %s", msg)
	}

	result, _ := data["result"].([]any)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Points from `%s`\n\n", collection))
	sb.WriteString(fmt.Sprintf("Retrieved **%d** / %d requested\n\n", len(result), len(pointIDs)))

	for _, p := range result {
		pt, ok := p.(map[string]any)
		if !ok {
			continue
		}
		sb.WriteString(fmt.Sprintf("### Point `%v`\n", pt["id"]))
		if withPayload {
			if payload, ok := pt["payload"].(map[string]any); ok && len(payload) > 0 {
				formatPayload(&sb, payload)
			}
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

func toolCountPoints(collection string, filter map[string]any, exact bool) (string, error) {
	if collection == "" {
		return "", fmt.Errorf("collection_name is required")
	}

	body := map[string]any{"exact": exact}
	if len(filter) > 0 {
		body["filter"] = filter
	}

	data, err := qdrantPost("/collections/"+collection+"/points/count", body)
	if err != nil {
		return "", fmt.Errorf("qdrant unreachable: %w", err)
	}
	if msg := qdrantError(data); msg != "" {
		return "", fmt.Errorf("count failed: %s", msg)
	}

	result, _ := data["result"].(map[string]any)
	count, _ := result["count"].(float64)

	qualifier := "approximate"
	if exact {
		qualifier = "exact"
	}

	filterDesc := ""
	if len(filter) > 0 {
		b, _ := json.Marshal(filter)
		filterDesc = fmt.Sprintf("\nFilter: `%s`", string(b))
	}

	return fmt.Sprintf("## Count: %s\n\n**%d** points (%s)%s\n", collection, int(count), qualifier, filterDesc), nil
}

// ── Search operations ─────────────────────────────────────────────────────────

func toolSearch(collection string, queryVector []float32, limit int, filter map[string]any, scoreThreshold float32, withPayload, withVector bool) (string, error) {
	if collection == "" {
		return "", fmt.Errorf("collection_name is required")
	}
	if len(queryVector) == 0 {
		return "", fmt.Errorf("query_vector is required — provide a float array matching the collection's vector dimension")
	}
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	body := map[string]any{
		"vector":       queryVector,
		"limit":        limit,
		"with_payload": withPayload,
		"with_vector":  withVector,
	}
	if len(filter) > 0 {
		body["filter"] = filter
	}
	if scoreThreshold > 0 {
		body["score_threshold"] = scoreThreshold
	}

	data, err := qdrantPost("/collections/"+collection+"/points/search", body)
	if err != nil {
		return "", fmt.Errorf("qdrant unreachable: %w", err)
	}
	if msg := qdrantError(data); msg != "" {
		return "", fmt.Errorf("search failed: %s", msg)
	}

	result, _ := data["result"].([]any)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Vector Search: %s\n\n", collection))
	sb.WriteString(fmt.Sprintf("Query dims: %d | Results: **%d**\n\n", len(queryVector), len(result)))

	for i, r := range result {
		hit, ok := r.(map[string]any)
		if !ok {
			continue
		}
		score, _ := hit["score"].(float64)
		sb.WriteString(fmt.Sprintf("### %d. Point `%v`  score: %.4f\n", i+1, hit["id"], score))
		if withPayload {
			if payload, ok := hit["payload"].(map[string]any); ok && len(payload) > 0 {
				formatPayload(&sb, payload)
			}
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

func toolRecommend(collection string, positiveIDs, negativeIDs []string, limit int, filter map[string]any, withPayload bool) (string, error) {
	if collection == "" {
		return "", fmt.Errorf("collection_name is required")
	}
	if len(positiveIDs) == 0 {
		return "", fmt.Errorf("positive_ids requires at least one point ID")
	}
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	positive := make([]any, len(positiveIDs))
	for i, id := range positiveIDs {
		positive[i] = parsePointID(id)
	}
	negative := make([]any, len(negativeIDs))
	for i, id := range negativeIDs {
		negative[i] = parsePointID(id)
	}

	body := map[string]any{
		"positive":     positive,
		"negative":     negative,
		"limit":        limit,
		"with_payload": withPayload,
		"with_vector":  false,
	}
	if len(filter) > 0 {
		body["filter"] = filter
	}

	data, err := qdrantPost("/collections/"+collection+"/points/recommend", body)
	if err != nil {
		return "", fmt.Errorf("qdrant unreachable: %w", err)
	}
	if msg := qdrantError(data); msg != "" {
		return "", fmt.Errorf("recommend failed: %s", msg)
	}

	result, _ := data["result"].([]any)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Recommend: %s\n\n", collection))
	sb.WriteString(fmt.Sprintf("Similar to: [%s] | Results: **%d**\n\n",
		strings.Join(positiveIDs, ", "), len(result)))

	for i, r := range result {
		hit, ok := r.(map[string]any)
		if !ok {
			continue
		}
		score, _ := hit["score"].(float64)
		sb.WriteString(fmt.Sprintf("### %d. Point `%v`  score: %.4f\n", i+1, hit["id"], score))
		if withPayload {
			if payload, ok := hit["payload"].(map[string]any); ok && len(payload) > 0 {
				formatPayload(&sb, payload)
			}
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

// ── Payload operations ────────────────────────────────────────────────────────

func toolSetPayload(collection string, pointIDs []string, payload map[string]any) (string, error) {
	if collection == "" || len(pointIDs) == 0 {
		return "", fmt.Errorf("collection_name and point_ids are required")
	}
	if len(payload) == 0 {
		return "", fmt.Errorf("payload must not be empty")
	}

	ids := make([]any, len(pointIDs))
	for i, id := range pointIDs {
		ids[i] = parsePointID(id)
	}

	data, err := qdrantPost("/collections/"+collection+"/points/payload", map[string]any{
		"payload": payload,
		"points":  ids,
	})
	if err != nil {
		return "", fmt.Errorf("qdrant unreachable: %w", err)
	}
	if msg := qdrantError(data); msg != "" {
		return "", fmt.Errorf("set payload failed: %s", msg)
	}

	return fmt.Sprintf("## Payload Updated\n\n%d points in `%s` — keys set: %s\n",
		len(pointIDs), collection, strings.Join(keysOf(payload), ", ")), nil
}

func toolDeletePayload(collection string, pointIDs []string, keys []string) (string, error) {
	if collection == "" || len(pointIDs) == 0 || len(keys) == 0 {
		return "", fmt.Errorf("collection_name, point_ids, and keys are all required")
	}

	ids := make([]any, len(pointIDs))
	for i, id := range pointIDs {
		ids[i] = parsePointID(id)
	}

	data, err := qdrantPost("/collections/"+collection+"/points/payload/delete", map[string]any{
		"keys":   keys,
		"points": ids,
	})
	if err != nil {
		return "", fmt.Errorf("qdrant unreachable: %w", err)
	}
	if msg := qdrantError(data); msg != "" {
		return "", fmt.Errorf("delete payload failed: %s", msg)
	}

	return fmt.Sprintf("## Payload Keys Deleted\n\nRemoved [%s] from %d points in `%s`\n",
		strings.Join(keys, ", "), len(pointIDs), collection), nil
}

func toolClearPayload(collection string, pointIDs []string) (string, error) {
	if collection == "" || len(pointIDs) == 0 {
		return "", fmt.Errorf("collection_name and point_ids are required")
	}

	ids := make([]any, len(pointIDs))
	for i, id := range pointIDs {
		ids[i] = parsePointID(id)
	}

	data, err := qdrantPost("/collections/"+collection+"/points/payload/clear", map[string]any{
		"points": ids,
	})
	if err != nil {
		return "", fmt.Errorf("qdrant unreachable: %w", err)
	}
	if msg := qdrantError(data); msg != "" {
		return "", fmt.Errorf("clear payload failed: %s", msg)
	}

	return fmt.Sprintf("## Payload Cleared\n\nAll payload removed from %d points in `%s`\n",
		len(pointIDs), collection), nil
}

// ── Query operations ──────────────────────────────────────────────────────────

// toolQueryPoints is a filter-first query tool backed by Qdrant's scroll API.
// Qdrant scroll accepts full filter DSL (must/should/must_not, match, range, etc.)
// and returns a next_page_offset token for pagination.
func toolQueryPoints(collection string, filter map[string]any, limit int, offset string, withPayload, withVector bool, orderBy string) (string, error) {
	if collection == "" {
		return "", fmt.Errorf("collection_name is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	body := map[string]any{
		"limit":        limit,
		"with_payload": withPayload,
		"with_vector":  withVector,
	}
	if len(filter) > 0 {
		body["filter"] = filter
	}
	if offset != "" {
		body["offset"] = parsePointID(offset)
	}

	data, err := qdrantPost("/collections/"+collection+"/points/scroll", body)
	if err != nil {
		return "", fmt.Errorf("qdrant unreachable: %w", err)
	}
	if msg := qdrantError(data); msg != "" {
		return "", fmt.Errorf("query failed: %s", msg)
	}

	result, _ := data["result"].(map[string]any)
	points, _ := result["points"].([]any)
	nextOffset := result["next_page_offset"]

	// Optional client-side sort by a payload field
	if orderBy != "" && len(points) > 0 {
		type sortItem struct {
			pt  map[string]any
			key string
		}
		items := make([]sortItem, 0, len(points))
		for _, p := range points {
			if pt, ok := p.(map[string]any); ok {
				key := ""
				if payload, ok := pt["payload"].(map[string]any); ok {
					key = fmt.Sprintf("%v", payload[orderBy])
				}
				items = append(items, sortItem{pt, key})
			}
		}
		sort.Slice(items, func(i, j int) bool { return items[i].key < items[j].key })
		sorted := make([]any, len(items))
		for i, item := range items {
			sorted[i] = item.pt
		}
		points = sorted
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Query: %s\n\n", collection))
	if len(filter) > 0 {
		b, _ := json.Marshal(filter)
		sb.WriteString(fmt.Sprintf("Filter: `%s`\n\n", string(b)))
	}
	if orderBy != "" {
		sb.WriteString(fmt.Sprintf("Sorted by: `%s`\n\n", orderBy))
	}
	sb.WriteString(fmt.Sprintf("Results: **%d** points\n\n", len(points)))
	if nextOffset != nil {
		sb.WriteString(fmt.Sprintf("> **Next page:** `%v`\n\n", nextOffset))
	}

	for _, p := range points {
		pt, ok := p.(map[string]any)
		if !ok {
			continue
		}
		sb.WriteString(fmt.Sprintf("### Point `%v`\n", pt["id"]))
		if withPayload {
			if payload, ok := pt["payload"].(map[string]any); ok && len(payload) > 0 {
				formatPayload(&sb, payload)
			}
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

// ── Formatting helpers ────────────────────────────────────────────────────────

// formatPayload writes sorted payload key-value pairs to sb, truncating long values.
func formatPayload(sb *strings.Builder, payload map[string]any) {
	keys := make([]string, 0, len(payload))
	for k := range payload {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		val := fmt.Sprintf("%v", payload[k])
		if len(val) > 300 {
			val = val[:300] + "…"
		}
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", k, val))
	}
}

// keysOf returns sorted keys from a map.
func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
