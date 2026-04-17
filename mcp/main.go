package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ── MCP protocol types ────────────────────────────────────────────────────────

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type ToolDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

type Property struct {
	Type        string    `json:"type"`
	Description string    `json:"description"`
	Items       *Property `json:"items,omitempty"`
}

// ── Qdrant client ─────────────────────────────────────────────────────────────

var qdrantURL = func() string {
	if u := os.Getenv("QDRANT_URL"); u != "" {
		return u
	}
	return "http://localhost:6333"
}()

func qdrantGet(path string) (map[string]any, error) {
	resp, err := http.Get(qdrantURL + path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var result map[string]any
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func qdrantPost(path string, body any) (map[string]any, error) {
	b, _ := json.Marshal(body)
	resp, err := http.Post(qdrantURL+path, "application/json", bytes.NewReader(b))
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

// ── Tool implementations ──────────────────────────────────────────────────────

func toolCollections() (string, error) {
	data, err := qdrantGet("/collections")
	if err != nil {
		return "", fmt.Errorf("qdrant unreachable: %w", err)
	}

	result, _ := data["result"].(map[string]any)
	collections, _ := result["collections"].([]any)

	var sb strings.Builder
	sb.WriteString("## Qdrant Collections\n\n")

	for _, c := range collections {
		col := c.(map[string]any)
		name := col["name"].(string)

		// Get point count for each collection
		info, err := qdrantGet("/collections/" + name)
		count := "unknown"
		if err == nil {
			if r, ok := info["result"].(map[string]any); ok {
				if pc, ok := r["points_count"].(float64); ok {
					count = fmt.Sprintf("%d", int(pc))
				}
			}
		}
		sb.WriteString(fmt.Sprintf("- **%s** — %s vectors\n", name, count))
	}

	return sb.String(), nil
}

func toolTopNodes(runID string, limit int) (string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Scroll through kae_nodes filtered by run_id
	body := map[string]any{
		"limit":        limit * 3, // fetch more to sort by weight
		"with_payload": true,
		"with_vector":  false,
	}
	if runID != "" {
		body["filter"] = map[string]any{
			"must": []map[string]any{
				{"key": "run_id", "match": map[string]any{"value": runID}},
			},
		}
	}

	data, err := qdrantPost("/collections/kae_nodes/points/scroll", body)
	if err != nil {
		return "", fmt.Errorf("scroll failed: %w", err)
	}

	result, _ := data["result"].(map[string]any)
	points, _ := result["points"].([]any)

	type node struct {
		Label   string
		Weight  float64
		Anomaly bool
		RunID   string
		Domain  string
		Cycle   int
	}

	nodes := make([]node, 0, len(points))
	for _, p := range points {
		pt := p.(map[string]any)
		payload, _ := pt["payload"].(map[string]any)
		n := node{
			Label:   strVal(payload, "label"),
			Weight:  floatVal(payload, "weight"),
			Anomaly: boolVal(payload, "anomaly"),
			RunID:   strVal(payload, "run_id"),
			Domain:  strVal(payload, "domain"),
			Cycle:   intVal(payload, "cycle"),
		}
		if n.Label != "" {
			nodes = append(nodes, n)
		}
	}

	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Weight > nodes[j].Weight
	})
	if len(nodes) > limit {
		nodes = nodes[:limit]
	}

	var sb strings.Builder
	title := "## Top KAE Nodes"
	if runID != "" {
		title += " — Run: " + runID
	}
	sb.WriteString(title + "\n\n")
	sb.WriteString(fmt.Sprintf("Showing top %d by weight\n\n", len(nodes)))

	for i, n := range nodes {
		anomalyFlag := ""
		if n.Anomaly {
			anomalyFlag = " ⚠"
		}
		sb.WriteString(fmt.Sprintf("%d. **%s**%s\n", i+1, n.Label, anomalyFlag))
		sb.WriteString(fmt.Sprintf("   weight: %.2f | domain: %s | cycle: %d | run: %s\n\n",
			n.Weight, n.Domain, n.Cycle, n.RunID))
	}

	return sb.String(), nil
}

func toolListRuns() (string, error) {
	// Scroll all nodes and collect unique run IDs
	data, err := qdrantPost("/collections/kae_nodes/points/scroll", map[string]any{
		"limit":        1000,
		"with_payload": true,
		"with_vector":  false,
	})
	if err != nil {
		return "", fmt.Errorf("scroll failed: %w", err)
	}

	result, _ := data["result"].(map[string]any)
	points, _ := result["points"].([]any)

	runs := make(map[string]struct {
		count   int
		anomaly int
		maxW    float64
		seed    string
	})

	for _, p := range points {
		pt := p.(map[string]any)
		payload, _ := pt["payload"].(map[string]any)
		runID := strVal(payload, "run_id")
		if runID == "" {
			continue
		}
		r := runs[runID]
		r.count++
		w := floatVal(payload, "weight")
		if w > r.maxW {
			r.maxW = w
		}
		if boolVal(payload, "anomaly") {
			r.anomaly++
		}
		runs[runID] = r
	}

	var sb strings.Builder
	sb.WriteString("## KAE Runs in Qdrant\n\n")
	sb.WriteString(fmt.Sprintf("Total runs: %d\n\n", len(runs)))

	for id, r := range runs {
		sb.WriteString(fmt.Sprintf("**%s**\n", id))
		sb.WriteString(fmt.Sprintf("  nodes: %d | anomalies: %d | max weight: %.2f\n\n",
			r.count, r.anomaly, r.maxW))
	}

	return sb.String(), nil
}

func toolSearchChunks(query string, limit int) (string, error) {
	if limit <= 0 || limit > 20 {
		limit = 5
	}

	// We need to embed the query — use a simple keyword scroll instead
	// since we don't have the embedder available in this binary
	// Filter by text content containing query keywords
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return "", fmt.Errorf("empty query")
	}

	data, err := qdrantPost("/collections/kae_chunks/points/scroll", map[string]any{
		"limit":        200,
		"with_payload": true,
		"with_vector":  false,
	})
	if err != nil {
		return "", fmt.Errorf("scroll failed: %w", err)
	}

	result, _ := data["result"].(map[string]any)
	points, _ := result["points"].([]any)

	type match struct {
		text   string
		source string
		topic  string
		score  int
	}

	var matches []match
	for _, p := range points {
		pt := p.(map[string]any)
		payload, _ := pt["payload"].(map[string]any)
		text := strings.ToLower(strVal(payload, "text"))
		score := 0
		for _, word := range words {
			if strings.Contains(text, word) {
				score++
			}
		}
		if score > 0 {
			matches = append(matches, match{
				text:   strVal(payload, "text"),
				source: strVal(payload, "source"),
				topic:  firstNonEmpty(strVal(payload, "semantic_domain"), strVal(payload, "run_topic"), strVal(payload, "topic")),
				score:  score,
			})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].score > matches[j].score
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Chunk Search: \"%s\"\n\n", query))
	sb.WriteString(fmt.Sprintf("Found %d matching chunks\n\n", len(matches)))

	for i, m := range matches {
		sb.WriteString(fmt.Sprintf("### Match %d (score: %d)\n", i+1, m.score))
		sb.WriteString(fmt.Sprintf("**Source:** %s | **Topic:** %s\n\n", m.source, m.topic))
		preview := m.text
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		sb.WriteString(preview + "\n\n---\n\n")
	}

	return sb.String(), nil
}

func toolCompareRuns(runIDs []string) (string, error) {
	if len(runIDs) < 2 {
		return "", fmt.Errorf("need at least 2 run IDs to compare")
	}

	// Fetch nodes for each run
	runNodes := make(map[string][]string)
	for _, runID := range runIDs {
		data, err := qdrantPost("/collections/kae_nodes/points/scroll", map[string]any{
			"limit":        500,
			"with_payload": true,
			"with_vector":  false,
			"filter": map[string]any{
				"must": []map[string]any{
					{"key": "run_id", "match": map[string]any{"value": runID}},
				},
			},
		})
		if err != nil {
			continue
		}
		result, _ := data["result"].(map[string]any)
		points, _ := result["points"].([]any)

		for _, p := range points {
			pt := p.(map[string]any)
			payload, _ := pt["payload"].(map[string]any)
			label := normalizeLabel(strVal(payload, "label"))
			if label != "" {
				runNodes[runID] = append(runNodes[runID], label)
			}
		}
	}

	// Find overlap
	type nodeCount struct {
		label string
		runs  []string
	}

	nodeSets := make(map[string]map[string]bool)
	for runID, labels := range runNodes {
		nodeSets[runID] = make(map[string]bool)
		for _, l := range labels {
			nodeSets[runID][l] = true
		}
	}

	allLabels := make(map[string][]string)
	for runID, labels := range nodeSets {
		for label := range labels {
			allLabels[label] = append(allLabels[label], runID)
		}
	}

	var shared []nodeCount
	var unique []nodeCount
	for label, runs := range allLabels {
		if len(runs) >= 2 {
			shared = append(shared, nodeCount{label, runs})
		} else {
			unique = append(unique, nodeCount{label, runs})
		}
	}

	sort.Slice(shared, func(i, j int) bool {
		return len(shared[i].runs) > len(shared[j].runs)
	})

	var sb strings.Builder
	sb.WriteString("## Cross-Run Convergence\n\n")
	sb.WriteString(fmt.Sprintf("Runs compared: %s\n\n", strings.Join(runIDs, ", ")))

	totalNodes := len(allLabels)
	overlapPct := 0.0
	if totalNodes > 0 {
		overlapPct = float64(len(shared)) / float64(totalNodes) * 100
	}
	sb.WriteString(fmt.Sprintf("**Overlap score: %.1f%%** (%d shared / %d total)\n\n", overlapPct, len(shared), totalNodes))

	if len(shared) > 0 {
		sb.WriteString("### Converged Concepts\n\n")
		for _, n := range shared {
			sb.WriteString(fmt.Sprintf("- **%s** (in %d/%d runs)\n", n.label, len(n.runs), len(runIDs)))
		}
	} else {
		sb.WriteString("No converged concepts found. Runs may need more cycles.\n")
	}

	return sb.String(), nil
}

func toolStartRun(seed string, cycles int, model string) (string, error) {
	// Find kae binary path (same directory as this mcp binary, parent of mcp dir)
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("could not locate executable: %w", err)
	}

	kaePath := filepath.Join(filepath.Dir(exePath), "..", "kae")
	if _, err := os.Stat(kaePath); err != nil {
		return "", fmt.Errorf("kae binary not found at %s: %w", kaePath, err)
	}

	// Build command
	args := []string{"-seed", seed, "-cycles", fmt.Sprintf("%d", cycles), "-headless"}
	if model != "" {
		args = append(args, "-model", model)
	}

	cmd := exec.Command(kaePath, args...)
	cmd.Dir = filepath.Join(filepath.Dir(exePath), "..") // ~/kae

	// Capture both stdout and stderr
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	// Run with timeout (cycles * 90 seconds per cycle max)
	timeoutSec := cycles * 90
	if timeoutSec > 600 {
		timeoutSec = 600 // max 10 minutes
	}

	// Start and wait with timeout
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start kae: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			return "", fmt.Errorf("kae run failed: %w\nstderr:\n%s", err, stderrBuf.String())
		}
	case <-time.After(time.Duration(timeoutSec) * time.Second):
		cmd.Process.Kill()
		return "", fmt.Errorf("kae run timed out after %d seconds", timeoutSec)
	}

	// Combine output
	output := stderrBuf.String()
	if stdoutBuf.Len() > 0 {
		output = stdoutBuf.String() + "\n" + output
	}

	// Extract report from output (look for markdown report pattern)
	lines := strings.Split(output, "\n")
	var reportLines []string
	inReport := false
	for _, line := range lines {
		if strings.HasPrefix(line, "## Cycle") {
			inReport = true
		}
		if inReport {
			reportLines = append(reportLines, line)
		}
	}

	if len(reportLines) > 0 {
		report := strings.Join(reportLines, "\n")
		return fmt.Sprintf("## KAE Run Completed\n\nSeed: `%s` | Cycles: %d\n\n%s", seed, cycles, report), nil
	}

	// Fallback: return last 50 lines of output
	start := len(lines) - 50
	if start < 0 {
		start = 0
	}
	summary := strings.Join(lines[start:], "\n")
	return fmt.Sprintf("## KAE Run Output (summary)\n\nSeed: `%s` | Cycles: %d\n\n```\n%s\n```", seed, cycles, summary), nil
}

func toolMetaAttractors(minRuns int) (string, error) {
	body := map[string]any{
		"limit":        500,
		"with_payload": true,
		"with_vector":  false,
		"filter": map[string]any{
			"must": []map[string]any{
				{"key": "occurrence_count", "range": map[string]any{"gte": minRuns}},
			},
		},
	}
	data, err := qdrantPost("/collections/kae_meta_graph/points/scroll", body)
	if err != nil {
		return "", fmt.Errorf("meta-graph scroll failed: %w", err)
	}

	result, _ := data["result"].(map[string]any)
	points, _ := result["points"].([]any)

	type attractor struct {
		Concept     string
		Occurrences int
		TotalWeight float64
		AvgAnomaly  float64
		Domains     []string
	}

	attrs := make([]attractor, 0, len(points))
	for _, p := range points {
		pt := p.(map[string]any)
		payload, _ := pt["payload"].(map[string]any)
		domains := []string{}
		if ds, ok := payload["domains"].([]any); ok {
			for _, d := range ds {
				if s, ok := d.(string); ok {
					domains = append(domains, s)
				}
			}
		}
		attrs = append(attrs, attractor{
			Concept:     strVal(payload, "concept"),
			Occurrences: intVal(payload, "occurrence_count"),
			TotalWeight: floatVal(payload, "total_weight"),
			AvgAnomaly:  floatVal(payload, "avg_anomaly"),
			Domains:     domains,
		})
	}

	sort.Slice(attrs, func(i, j int) bool {
		return attrs[i].Occurrences > attrs[j].Occurrences
	})

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Meta-Graph Attractors (min %d runs)\n\n", minRuns))
	if len(attrs) == 0 {
		sb.WriteString("No attractors found yet. Run more KAE cycles to build the meta-graph.\n")
		return sb.String(), nil
	}
	sb.WriteString(fmt.Sprintf("Found **%d attractor concepts** converging across runs.\n\n", len(attrs)))
	for i, a := range attrs {
		sb.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, a.Concept))
		sb.WriteString(fmt.Sprintf("   runs: %d | weight: %.1f | avg anomaly: %.2f | domains: %s\n\n",
			a.Occurrences, a.TotalWeight, a.AvgAnomaly, strings.Join(a.Domains, ", ")))
	}
	return sb.String(), nil
}

func toolDomainAnalysis() (string, error) {
	data, err := qdrantPost("/collections/kae_meta_graph/points/scroll", map[string]any{
		"limit":        2000,
		"with_payload": true,
		"with_vector":  false,
	})
	if err != nil {
		return "", fmt.Errorf("meta-graph scroll failed: %w", err)
	}

	result, _ := data["result"].(map[string]any)
	points, _ := result["points"].([]any)

	type nodeInfo struct {
		concept string
		domains []string
		weight  float64
	}

	nodes := make([]nodeInfo, 0, len(points))
	for _, p := range points {
		pt := p.(map[string]any)
		payload, _ := pt["payload"].(map[string]any)
		domains := []string{}
		if ds, ok := payload["domains"].([]any); ok {
			for _, d := range ds {
				if s, ok := d.(string); ok && s != "" {
					domains = append(domains, s)
				}
			}
		}
		nodes = append(nodes, nodeInfo{
			concept: strVal(payload, "concept"),
			domains: domains,
			weight:  floatVal(payload, "total_weight"),
		})
	}

	// Find bridges
	type bridge struct {
		concept string
		d1, d2  string
		weight  float64
	}
	var bridges []bridge
	allDomains := map[string]bool{}
	bridgedPairs := map[string]bool{}

	for _, n := range nodes {
		for _, d := range n.domains {
			allDomains[d] = true
		}
		if len(n.domains) >= 2 {
			for i := 0; i < len(n.domains); i++ {
				for j := i + 1; j < len(n.domains); j++ {
					d1, d2 := n.domains[i], n.domains[j]
					if d1 > d2 {
						d1, d2 = d2, d1
					}
					bridgedPairs[d1+"||"+d2] = true
					bridges = append(bridges, bridge{n.concept, d1, d2, n.weight})
				}
			}
		}
	}
	sort.Slice(bridges, func(i, j int) bool { return bridges[i].weight > bridges[j].weight })

	// Find moats
	domainList := make([]string, 0, len(allDomains))
	for d := range allDomains {
		domainList = append(domainList, d)
	}
	sort.Strings(domainList)

	var sb strings.Builder
	sb.WriteString("## Domain Boundary Analysis\n\n")

	sb.WriteString(fmt.Sprintf("### Cross-Domain Bridges (%d)\n\n", len(bridges)))
	if len(bridges) == 0 {
		sb.WriteString("No bridge concepts yet.\n\n")
	} else {
		for _, b := range bridges {
			sb.WriteString(fmt.Sprintf("- **%s** bridges `%s` ↔ `%s` (weight %.1f)\n", b.concept, b.d1, b.d2, b.weight))
		}
		sb.WriteString("\n")
	}

	moatCount := 0
	var moatLines []string
	for i := 0; i < len(domainList); i++ {
		for j := i + 1; j < len(domainList); j++ {
			d1, d2 := domainList[i], domainList[j]
			if !bridgedPairs[d1+"||"+d2] {
				moatLines = append(moatLines, fmt.Sprintf("- `%s` ↔ `%s`", d1, d2))
				moatCount++
			}
		}
	}
	sb.WriteString(fmt.Sprintf("### Potential Domain Moats (%d)\n\n", moatCount))
	if moatCount == 0 {
		sb.WriteString("All domain pairs are bridged.\n")
	} else {
		sb.WriteString("Domain pairs with no bridging concepts:\n\n")
		for _, l := range moatLines {
			sb.WriteString(l + "\n")
		}
	}
	return sb.String(), nil
}

// ── MCP server loop ───────────────────────────────────────────────────────────

var tools = []ToolDef{
	{
		Name:        "qdrant_collections",
		Description: "List all Qdrant collections with vector counts. Use this first to understand what data is available.",
		InputSchema: InputSchema{Type: "object", Properties: map[string]Property{}},
	},
	{
		Name:        "qdrant_list_runs",
		Description: "List all KAE runs stored in Qdrant with node counts and anomaly counts per run.",
		InputSchema: InputSchema{Type: "object", Properties: map[string]Property{}},
	},
	{
		Name:        "qdrant_top_nodes",
		Description: "Get the highest-weight emergent concept nodes from KAE runs. Optionally filter by run_id.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"run_id": {Type: "string", Description: "Optional KAE run ID to filter by (e.g. 'run_1744123456'). Leave empty for all runs."},
				"limit":  {Type: "integer", Description: "Number of top nodes to return (default 20, max 100)"},
			},
		},
	},
	{
		Name:        "qdrant_search_chunks",
		Description: "Search ingested source chunks by keyword. Returns matching passages with their source URLs and topics.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"query": {Type: "string", Description: "Keywords to search for in ingested source passages"},
				"limit": {Type: "integer", Description: "Number of results to return (default 5, max 20)"},
			},
			Required: []string{"query"},
		},
	},
	{
		Name:        "qdrant_compare_runs",
		Description: "Compare multiple KAE runs to find concepts that converged independently. Shows overlap score and shared nodes.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"run_ids": {Type: "string", Description: "Comma-separated list of run IDs to compare (e.g. 'run_111,run_222')"},
			},
			Required: []string{"run_ids"},
		},
	},
	{
		Name:        "kae_start_run",
		Description: "Start a new KAE archaeology run in headless mode. Returns the run report.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"seed":   {Type: "string", Description: "Seed topic to explore (e.g. 'consciousness', 'quantum gravity')"},
				"cycles": {Type: "integer", Description: "Number of cycles to run (default 3, max 10)"},
				"model":  {Type: "string", Description: "Optional model override (e.g. 'deepseek/deepseek-r1', 'google/gemini-2.5-flash')"},
			},
			Required: []string{"seed"},
		},
	},
	{
		Name:        "kae_meta_attractors",
		Description: "Show attractor concepts from the persistent meta-graph — concepts that emerged independently across multiple KAE runs.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"min_runs": {Type: "integer", Description: "Minimum number of runs a concept must appear in to be an attractor (default 3)"},
			},
		},
	},
	{
		Name:        "kae_domain_analysis",
		Description: "Show domain bridge and moat analysis — concepts that connect domains (bridges) and domain pairs with no connecting concepts (moats).",
		InputSchema: InputSchema{Type: "object", Properties: map[string]Property{}},
	},

	// ── Collection management ───────────────────────────────────────────────
	{
		Name:        "qdrant_collection_info",
		Description: "Get detailed info about a specific Qdrant collection: vector size, distance metric, point count, segment count, and status.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"collection_name": {Type: "string", Description: "Name of the collection to inspect"},
			},
			Required: []string{"collection_name"},
		},
	},
	{
		Name:        "qdrant_create_collection",
		Description: "Create a new Qdrant collection with the specified vector configuration.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"collection_name": {Type: "string", Description: "Name for the new collection"},
				"vector_size":     {Type: "integer", Description: "Vector dimension (e.g. 1536 for OpenAI, 768 for BERT)"},
				"distance":        {Type: "string", Description: "Distance metric: Cosine (default), Euclid, Dot, or Manhattan"},
				"on_disk":         {Type: "boolean", Description: "Store vectors on disk to reduce RAM (default: false)"},
			},
			Required: []string{"collection_name", "vector_size"},
		},
	},
	{
		Name:        "qdrant_delete_collection",
		Description: "PERMANENTLY DELETE a Qdrant collection and all its data. IRREVERSIBLE — double-check the name.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"collection_name": {Type: "string", Description: "Name of the collection to DELETE permanently"},
			},
			Required: []string{"collection_name"},
		},
	},

	// ── Point operations ────────────────────────────────────────────────────
	{
		Name:        "qdrant_scroll_points",
		Description: "Browse points in any Qdrant collection with pagination. Returns a next_offset token to fetch subsequent pages.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"collection_name": {Type: "string", Description: "Collection to scroll"},
				"limit":           {Type: "integer", Description: "Points per page (default: 10, max: 100)"},
				"offset":          {Type: "string", Description: "Pagination token from a previous response (omit for first page)"},
				"with_payload":    {Type: "boolean", Description: "Include payload fields (default: true)"},
				"with_vector":     {Type: "boolean", Description: "Include raw vectors — use sparingly (default: false)"},
				"filter":          {Type: "object", Description: `Qdrant filter DSL e.g. {"must":[{"key":"run_id","match":{"value":"run_123"}}]}`},
			},
			Required: []string{"collection_name"},
		},
	},
	{
		Name:        "qdrant_get_point",
		Description: "Retrieve a single point by ID from any Qdrant collection.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"collection_name": {Type: "string", Description: "Collection name"},
				"point_id":        {Type: "string", Description: "Point ID — numeric string (e.g. '12345') or UUID"},
				"with_payload":    {Type: "boolean", Description: "Include payload (default: true)"},
				"with_vector":     {Type: "boolean", Description: "Include vector (default: false)"},
			},
			Required: []string{"collection_name", "point_id"},
		},
	},
	{
		Name:        "qdrant_get_points",
		Description: "Retrieve multiple specific points by ID from a Qdrant collection in one request.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"collection_name": {Type: "string", Description: "Collection name"},
				"point_ids":       {Type: "array", Description: "Array of point IDs (numeric strings or UUIDs)", Items: &Property{Type: "string"}},
				"with_payload":    {Type: "boolean", Description: "Include payloads (default: true)"},
				"with_vector":     {Type: "boolean", Description: "Include vectors (default: false)"},
			},
			Required: []string{"collection_name", "point_ids"},
		},
	},
	{
		Name:        "qdrant_count_points",
		Description: "Count points in a Qdrant collection, optionally scoped to a filter.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"collection_name": {Type: "string", Description: "Collection to count"},
				"filter":          {Type: "object", Description: "Qdrant filter DSL (optional, omit to count all points)"},
				"exact":           {Type: "boolean", Description: "Exact count vs approximate (default: true; set false for very large collections)"},
			},
			Required: []string{"collection_name"},
		},
	},

	// ── Search ──────────────────────────────────────────────────────────────
	{
		Name:        "qdrant_search",
		Description: "Vector similarity search in a Qdrant collection. Requires a query_vector matching the collection's vector dimension.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"collection_name": {Type: "string", Description: "Collection to search"},
				"query_vector":    {Type: "array", Description: "Query vector — float array matching the collection's vector size", Items: &Property{Type: "number"}},
				"limit":           {Type: "integer", Description: "Number of results (default: 10, max: 100)"},
				"score_threshold": {Type: "number", Description: "Minimum similarity score (0–1 for Cosine); omit to return all"},
				"filter":          {Type: "object", Description: "Qdrant filter DSL (optional)"},
				"with_payload":    {Type: "boolean", Description: "Include payloads (default: true)"},
				"with_vector":     {Type: "boolean", Description: "Include vectors (default: false)"},
			},
			Required: []string{"collection_name", "query_vector"},
		},
	},
	{
		Name:        "qdrant_recommend",
		Description: "Recommend points similar to given examples (positive IDs) and dissimilar from others (negative IDs) using existing collection vectors.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"collection_name": {Type: "string", Description: "Collection to search"},
				"positive_ids":    {Type: "array", Description: "Point IDs to find results similar to (at least one required)", Items: &Property{Type: "string"}},
				"negative_ids":    {Type: "array", Description: "Point IDs to steer away from (optional)", Items: &Property{Type: "string"}},
				"limit":           {Type: "integer", Description: "Number of results (default: 10)"},
				"filter":          {Type: "object", Description: "Qdrant filter DSL (optional)"},
				"with_payload":    {Type: "boolean", Description: "Include payloads (default: true)"},
			},
			Required: []string{"collection_name", "positive_ids"},
		},
	},

	// ── Payload management ──────────────────────────────────────────────────
	{
		Name:        "qdrant_set_payload",
		Description: "Set or merge payload fields on specific points. Unmodified payload keys are preserved.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"collection_name": {Type: "string", Description: "Collection name"},
				"point_ids":       {Type: "array", Description: "Point IDs to update", Items: &Property{Type: "string"}},
				"payload":         {Type: "object", Description: "Key-value pairs to set (merged into existing payload)"},
			},
			Required: []string{"collection_name", "point_ids", "payload"},
		},
	},
	{
		Name:        "qdrant_delete_payload",
		Description: "Remove specific payload keys from points. Other payload fields are preserved.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"collection_name": {Type: "string", Description: "Collection name"},
				"point_ids":       {Type: "array", Description: "Point IDs to update", Items: &Property{Type: "string"}},
				"keys":            {Type: "array", Description: "Payload key names to remove", Items: &Property{Type: "string"}},
			},
			Required: []string{"collection_name", "point_ids", "keys"},
		},
	},
	{
		Name:        "qdrant_clear_payload",
		Description: "Remove ALL payload from specific points (vectors are preserved).",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"collection_name": {Type: "string", Description: "Collection name"},
				"point_ids":       {Type: "array", Description: "Point IDs to clear payload from", Items: &Property{Type: "string"}},
			},
			Required: []string{"collection_name", "point_ids"},
		},
	},

	// ── Advanced querying ───────────────────────────────────────────────────
	{
		Name:        "qdrant_query_points",
		Description: "Query any Qdrant collection with the full filter DSL (must/should/must_not, match, range, text). Returns paginated results with optional client-side sort.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"collection_name": {Type: "string", Description: "Collection to query"},
				"filter":          {Type: "object", Description: `Qdrant filter DSL e.g. {"must":[{"key":"weight","range":{"gte":3.0}}]}`},
				"limit":           {Type: "integer", Description: "Max results (default: 10, max: 100)"},
				"offset":          {Type: "string", Description: "Pagination token from a previous response"},
				"with_payload":    {Type: "boolean", Description: "Include payloads (default: true)"},
				"with_vector":     {Type: "boolean", Description: "Include vectors (default: false)"},
				"order_by":        {Type: "string", Description: "Payload field name to sort results by (client-side ascending sort)"},
			},
			Required: []string{"collection_name"},
		},
	},
}

func handleRequest(req JSONRPCRequest) JSONRPCResponse {
	switch req.Method {
	case "initialize":
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "kae_qdrant_mcp", "version": "1.0.0"},
			},
		}

	case "tools/list":
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{"tools": tools},
		}

	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return errResponse(req.ID, -32600, "invalid params")
		}

		result, err := dispatchTool(params.Name, params.Arguments)
		if err != nil {
			return JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]any{
					"content": []map[string]any{{"type": "text", "text": "Error: " + err.Error()}},
					"isError": true,
				},
			}
		}
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"content": []map[string]any{{"type": "text", "text": result}},
			},
		}

	case "notifications/initialized":
		return JSONRPCResponse{} // no response needed

	default:
		return errResponse(req.ID, -32601, "method not found: "+req.Method)
	}
}

func dispatchTool(name string, args map[string]any) (string, error) {
	switch name {
	case "qdrant_collections":
		return toolCollections()

	case "qdrant_list_runs":
		return toolListRuns()

	case "qdrant_top_nodes":
		runID, _ := args["run_id"].(string)
		limit := 20
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}
		return toolTopNodes(runID, limit)

	case "qdrant_search_chunks":
		query, _ := args["query"].(string)
		limit := 5
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}
		return toolSearchChunks(query, limit)

	case "qdrant_compare_runs":
		runIDsStr, _ := args["run_ids"].(string)
		parts := strings.Split(runIDsStr, ",")
		var runIDs []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				runIDs = append(runIDs, p)
			}
		}
		return toolCompareRuns(runIDs)

	case "kae_start_run":
		seed, _ := args["seed"].(string)
		if seed == "" {
			return "", fmt.Errorf("seed is required")
		}
		cycles := 3
		if c, ok := args["cycles"].(float64); ok {
			cycles = int(c)
		}
		model, _ := args["model"].(string)
		return toolStartRun(seed, cycles, model)

	case "kae_meta_attractors":
		minRuns := 3
		if m, ok := args["min_runs"].(float64); ok {
			minRuns = int(m)
		}
		return toolMetaAttractors(minRuns)

	case "kae_domain_analysis":
		return toolDomainAnalysis()

	// ── Collection management ───────────────────────────────────────────────

	case "qdrant_collection_info":
		name, _ := args["collection_name"].(string)
		return toolCollectionInfo(name)

	case "qdrant_create_collection":
		name, _ := args["collection_name"].(string)
		distance, _ := args["distance"].(string)
		vectorSize := 0
		if vs, ok := args["vector_size"].(float64); ok {
			vectorSize = int(vs)
		}
		onDisk, _ := args["on_disk"].(bool)
		return toolCreateCollection(name, distance, vectorSize, onDisk)

	case "qdrant_delete_collection":
		name, _ := args["collection_name"].(string)
		return toolDeleteCollection(name)

	// ── Point operations ────────────────────────────────────────────────────

	case "qdrant_scroll_points":
		collection, _ := args["collection_name"].(string)
		limit := 10
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}
		offset, _ := args["offset"].(string)
		withPayload := true
		if v, ok := args["with_payload"].(bool); ok {
			withPayload = v
		}
		withVector := false
		if v, ok := args["with_vector"].(bool); ok {
			withVector = v
		}
		filter, _ := args["filter"].(map[string]any)
		return toolScrollPoints(collection, limit, offset, withPayload, withVector, filter)

	case "qdrant_get_point":
		collection, _ := args["collection_name"].(string)
		pointID, _ := args["point_id"].(string)
		withPayload := true
		if v, ok := args["with_payload"].(bool); ok {
			withPayload = v
		}
		withVector := false
		if v, ok := args["with_vector"].(bool); ok {
			withVector = v
		}
		return toolGetPoint(collection, pointID, withPayload, withVector)

	case "qdrant_get_points":
		collection, _ := args["collection_name"].(string)
		pointIDs := stringArray(args["point_ids"])
		withPayload := true
		if v, ok := args["with_payload"].(bool); ok {
			withPayload = v
		}
		withVector := false
		if v, ok := args["with_vector"].(bool); ok {
			withVector = v
		}
		return toolGetPoints(collection, pointIDs, withPayload, withVector)

	case "qdrant_count_points":
		collection, _ := args["collection_name"].(string)
		filter, _ := args["filter"].(map[string]any)
		exact := true
		if v, ok := args["exact"].(bool); ok {
			exact = v
		}
		return toolCountPoints(collection, filter, exact)

	// ── Search ──────────────────────────────────────────────────────────────

	case "qdrant_search":
		collection, _ := args["collection_name"].(string)
		queryVector := float32Array(args["query_vector"])
		limit := 10
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}
		filter, _ := args["filter"].(map[string]any)
		var scoreThreshold float32
		if st, ok := args["score_threshold"].(float64); ok {
			scoreThreshold = float32(st)
		}
		withPayload := true
		if v, ok := args["with_payload"].(bool); ok {
			withPayload = v
		}
		withVector := false
		if v, ok := args["with_vector"].(bool); ok {
			withVector = v
		}
		return toolSearch(collection, queryVector, limit, filter, scoreThreshold, withPayload, withVector)

	case "qdrant_recommend":
		collection, _ := args["collection_name"].(string)
		positiveIDs := stringArray(args["positive_ids"])
		negativeIDs := stringArray(args["negative_ids"])
		limit := 10
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}
		filter, _ := args["filter"].(map[string]any)
		withPayload := true
		if v, ok := args["with_payload"].(bool); ok {
			withPayload = v
		}
		return toolRecommend(collection, positiveIDs, negativeIDs, limit, filter, withPayload)

	// ── Payload management ──────────────────────────────────────────────────

	case "qdrant_set_payload":
		collection, _ := args["collection_name"].(string)
		pointIDs := stringArray(args["point_ids"])
		payload, _ := args["payload"].(map[string]any)
		return toolSetPayload(collection, pointIDs, payload)

	case "qdrant_delete_payload":
		collection, _ := args["collection_name"].(string)
		pointIDs := stringArray(args["point_ids"])
		keys := stringArray(args["keys"])
		return toolDeletePayload(collection, pointIDs, keys)

	case "qdrant_clear_payload":
		collection, _ := args["collection_name"].(string)
		pointIDs := stringArray(args["point_ids"])
		return toolClearPayload(collection, pointIDs)

	// ── Advanced querying ───────────────────────────────────────────────────

	case "qdrant_query_points":
		collection, _ := args["collection_name"].(string)
		filter, _ := args["filter"].(map[string]any)
		limit := 10
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}
		offset, _ := args["offset"].(string)
		withPayload := true
		if v, ok := args["with_payload"].(bool); ok {
			withPayload = v
		}
		withVector := false
		if v, ok := args["with_vector"].(bool); ok {
			withVector = v
		}
		orderBy, _ := args["order_by"].(string)
		return toolQueryPoints(collection, filter, limit, offset, withPayload, withVector, orderBy)

	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

func errResponse(id any, code int, msg string) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: msg},
	}
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	encoder := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			encoder.Encode(errResponse(nil, -32700, "parse error"))
			continue
		}

		resp := handleRequest(req)
		// Don't send response for notifications
		if req.Method == "notifications/initialized" {
			continue
		}
		encoder.Encode(resp)
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func strVal(m map[string]any, k string) string {
	if v, ok := m[k]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
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

func normalizeLabel(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	return s
}
