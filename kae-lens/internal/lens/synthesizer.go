package lens

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/meistro/kae/collections"
	"github.com/meistro/kae/internal/config"
	"github.com/meistro/kae/internal/llm"
	qdrantclient "github.com/meistro/kae/internal/qdrantclient"
	"github.com/qdrant/go-client/qdrant"
)

// Synthesizer calls the LLM to reason over an anchor point and its neighbors,
// producing structured findings.
type Synthesizer struct {
	llm *llm.Client
	cfg *config.LensConfig
}

// NewSynthesizer creates a Synthesizer.
func NewSynthesizer(llmClient *llm.Client, cfg *config.LensConfig) *Synthesizer {
	return &Synthesizer{llm: llmClient, cfg: cfg}
}

// rawFinding is the JSON structure the LLM returns per finding.
type rawFinding struct {
	Type           string   `json:"type"`
	Confidence     float64  `json:"confidence"`
	SourcePointIDs []string `json:"source_point_ids"`
	Domains        []string `json:"domains"`
	Summary        string   `json:"summary"`
	ReasoningTrace string   `json:"reasoning_trace"`
}

// neighborSummary is a condensed neighbor for the prompt.
type neighborSummary struct {
	ID      string
	Title   string
	Domain  string
	Content string
	Score   float32
	URL     string // HTTP(S) source URL, empty if not available
}

// Synthesize runs LLM reasoning over an anchor + its neighbors.
// Returns only findings that meet the minimum confidence threshold.
func (s *Synthesizer) Synthesize(
	ctx context.Context,
	batchID string,
	anchor *anchorPoint,
	neighbors []*qdrant.ScoredPoint,
	profile *DensityProfile,
) ([]*collections.LensFinding, error) {

	// Parse neighbors into summaries and build a URL map for all points in scope.
	// allURLs maps point ID → source URL (HTTP(S) only) for the anchor + all neighbors.
	allURLs := make(map[string]string)
	if strings.HasPrefix(anchor.url, "http://") || strings.HasPrefix(anchor.url, "https://") {
		allURLs[anchor.id] = anchor.url
	}

	neighborSummaries := make([]neighborSummary, 0, len(neighbors))
	for _, n := range neighbors {
		payload := qdrantclient.PayloadToMap(n.Payload)
		ns := neighborSummary{
			ID:     qdrantclient.PointIDStr(n.Id),
			Score:  n.Score,
			Title:  stringField(payload, "title"),
			Domain: stringField(payload, "domain"),
		}
		// Truncate content for prompt efficiency
		content := stringField(payload, "content")
		if len(content) > 400 {
			content = content[:400] + "..."
		}
		ns.Content = content

		// Extract source URL (kae_knowledge has 'url'; kae_chunks stores URL in 'source')
		srcURL := stringField(payload, "url")
		if srcURL == "" {
			srcURL = stringField(payload, "source")
		}
		if strings.HasPrefix(srcURL, "http://") || strings.HasPrefix(srcURL, "https://") {
			ns.URL = srcURL
			allURLs[ns.ID] = srcURL
		}

		neighborSummaries = append(neighborSummaries, ns)
	}

	// Choose model based on batch complexity
	useReasoningModel := len(neighbors) >= s.cfg.LLM.FastBatchThreshold

	systemPrompt := s.buildSystemPrompt()
	userPrompt := s.buildUserPrompt(anchor, neighborSummaries, profile)

	log.Printf("[synthesizer] calling LLM for %q | neighbors=%d | model=reasoning=%v",
		anchor.title, len(neighborSummaries), useReasoningModel)

	// Apply per-call timeout so a hung DeepSeek-R1 response doesn't cancel
	// the whole batch context — each point gets its own deadline.
	timeout := time.Duration(s.cfg.LLM.LLMTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second // safe default if not configured
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var (
		resp *llm.ChatResponse
		err  error
	)

	if useReasoningModel {
		resp, err = s.llm.Reason(callCtx, systemPrompt, userPrompt)
	} else {
		resp, err = s.llm.FastChat(callCtx, systemPrompt, userPrompt)
	}
	if err != nil {
		return nil, fmt.Errorf("LLM call: %w", err)
	}

	log.Printf("[synthesizer] LLM response: %d tokens for %q", resp.Tokens, anchor.title)

	// Parse findings from response
	raw, err := s.parseFindings(resp.Content)
	if err != nil {
		log.Printf("[synthesizer] parse error for %q: %v\nraw: %s", anchor.title, err, resp.Content)
		return nil, nil // Non-fatal: log and continue
	}

	// Convert to LensFinding, filtering by confidence
	now := time.Now().Unix()
	findings := make([]*collections.LensFinding, 0, len(raw))
	for _, r := range raw {
		if r.Confidence < s.cfg.LLM.MinConfidence {
			continue
		}
		if !isValidFindingType(r.Type) {
			log.Printf("[synthesizer] unknown finding type %q, skipping", r.Type)
			continue
		}

		// Always include the anchor ID in source IDs
		sourceIDs := r.SourcePointIDs
		if !contains(sourceIDs, anchor.id) {
			sourceIDs = append([]string{anchor.id}, sourceIDs...)
		}

		// Collect unique domains from anchor + finding
		domains := uniqueDomains(append(r.Domains, anchor.domain))

		embeddingText := fmt.Sprintf("[%s] %s", r.Type, r.Summary)

		// Build the subset of URLs that are relevant to this finding's source IDs.
		var sourceURLs map[string]string
		for _, id := range sourceIDs {
			if u, ok := allURLs[id]; ok {
				if sourceURLs == nil {
					sourceURLs = make(map[string]string)
				}
				sourceURLs[id] = u
			}
		}

		findings = append(findings, &collections.LensFinding{
			Type:           collections.FindingType(r.Type),
			Confidence:     r.Confidence,
			SourcePointIDs: sourceIDs,
			SourceURLs:     sourceURLs,
			Domains:        domains,
			Summary:        r.Summary,
			ReasoningTrace: r.ReasoningTrace,
			EmbeddingText:  embeddingText,
			CreatedAt:      now,
			Reviewed:       false,
			BatchID:        batchID,
		})
	}

	log.Printf("[synthesizer] %d findings produced for %q (threshold=%.2f)",
		len(findings), anchor.title, s.cfg.LLM.MinConfidence)

	return findings, nil
}

// buildSystemPrompt returns the system-level instructions for the LLM.
func (s *Synthesizer) buildSystemPrompt() string {
	return `You are KAE Lens — an autonomous knowledge synthesis agent.

Your mission is to find meaningful patterns in a vector knowledge graph that the ingestion 
system (KAE) never explicitly tagged. You reason over an anchor knowledge point and its 
nearest semantic neighbors, then identify:

1. CONNECTIONS  — unexpected cross-domain links (e.g. quantum physics ↔ ancient philosophy)
2. CONTRADICTIONS — conflicting claims between nodes (e.g. two sources disagree on a fact)
3. CLUSTERS — emergent concept groups not yet labeled (e.g. 8 nodes all relate to "observer effect")
4. ANOMALIES — outliers that break mainstream consensus or sit oddly isolated

Rules:
- Think step by step. Show your reasoning chain in reasoning_trace.
- Only report findings with genuine intellectual weight — do not invent connections.
- If nothing significant exists, return an empty array [].
- Output ONLY a valid JSON array. No preamble, no markdown, no explanation outside the JSON.
- Each finding must follow this exact schema:
  {
    "type": "connection|contradiction|cluster|anomaly",
    "confidence": 0.0-1.0,
    "source_point_ids": ["uuid1", "uuid2"],
    "domains": ["domain1", "domain2"],
    "summary": "1-2 sentence plain English description",
    "reasoning_trace": "Step 1: ... Step 2: ... Conclusion: ..."
  }`
}

// buildUserPrompt constructs the per-point reasoning prompt.
func (s *Synthesizer) buildUserPrompt(anchor *anchorPoint, neighbors []neighborSummary, profile *DensityProfile) string {
	var sb strings.Builder

	sb.WriteString("## ANCHOR POINT\n")
	sb.WriteString(fmt.Sprintf("ID: %s\n", anchor.id))
	sb.WriteString(fmt.Sprintf("Title: %s\n", anchor.title))
	sb.WriteString(fmt.Sprintf("Domain: %s\n", anchor.domain))
	if len(anchor.content) > 600 {
		sb.WriteString(fmt.Sprintf("Content: %s...\n", anchor.content[:600]))
	} else {
		sb.WriteString(fmt.Sprintf("Content: %s\n", anchor.content))
	}

	sb.WriteString(fmt.Sprintf("\n## NEIGHBOR POINTS (%d retrieved, density=%s, width=%d)\n",
		len(neighbors), profile.Label, profile.SearchWidth))

	for i, n := range neighbors {
		sb.WriteString(fmt.Sprintf("\n[%d] ID: %s | Score: %.3f | Domain: %s\n", i+1, n.ID, n.Score, n.Domain))
		sb.WriteString(fmt.Sprintf("    Title: %s\n", n.Title))
		if n.Content != "" {
			sb.WriteString(fmt.Sprintf("    Content: %s\n", n.Content))
		}
	}

	sb.WriteString("\n## TASK\n")
	sb.WriteString("Analyze the anchor point in relation to its neighbors.\n")
	sb.WriteString("Identify connections, contradictions, clusters, or anomalies.\n")
	sb.WriteString("Return ONLY a JSON array of findings. Empty array [] if nothing significant.\n")

	return sb.String()
}

// Correct runs a focused second-pass LLM call for an anomaly or contradiction
// finding, producing a data-grounded correction using the anchor and neighbor
// content that were already in scope when the finding was made.
// Returns an empty string (non-fatal) if the call fails.
func (s *Synthesizer) Correct(
	ctx context.Context,
	finding *collections.LensFinding,
	anchor *anchorPoint,
	neighbors []neighborSummary,
) string {
	system := `You are a data correction agent working inside KAE Lens.

An anomaly or contradiction has been flagged in a knowledge graph. Your job is to
use the raw source evidence — and only that evidence — to produce a precise,
data-grounded correction or resolution.

Rules:
- Do not speculate beyond what the source data contains.
- Cite specific source point IDs where they support your correction.
- Write 2-4 sentences in plain English.
- Output only the correction text. No JSON, no preamble, no bullet points.`

	user := s.buildCorrectionPrompt(finding, anchor, neighbors)

	log.Printf("[synthesizer] requesting correction for %q (%s)", anchor.title, finding.Type)

	timeout := time.Duration(s.cfg.LLM.LLMTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp, err := s.llm.FastChat(callCtx, system, user)
	if err != nil {
		log.Printf("[synthesizer] correction LLM error for %q: %v", anchor.title, err)
		return ""
	}

	correction := strings.TrimSpace(resp.Content)
	log.Printf("[synthesizer] correction produced for %q (%d chars)", anchor.title, len(correction))
	return correction
}

// buildCorrectionPrompt constructs the prompt for the correction pass.
func (s *Synthesizer) buildCorrectionPrompt(
	finding *collections.LensFinding,
	anchor *anchorPoint,
	neighbors []neighborSummary,
) string {
	var sb strings.Builder

	sb.WriteString("## FINDING\n")
	sb.WriteString(fmt.Sprintf("Type: %s\n", finding.Type))
	sb.WriteString(fmt.Sprintf("Summary: %s\n", finding.Summary))
	if finding.ReasoningTrace != "" {
		sb.WriteString(fmt.Sprintf("Reasoning: %s\n", finding.ReasoningTrace))
	}

	sb.WriteString("\n## SOURCE DATA\n")
	sb.WriteString(fmt.Sprintf("\nAnchor [%s] %s — %s\n", anchor.id, anchor.title, anchor.domain))
	content := anchor.content
	if len(content) > 500 {
		content = content[:500] + "..."
	}
	if content != "" {
		sb.WriteString(fmt.Sprintf("%s\n", content))
	}

	for _, n := range neighbors {
		sb.WriteString(fmt.Sprintf("\nNeighbor [%s] %s — %s\n", n.ID, n.Title, n.Domain))
		if n.Content != "" {
			sb.WriteString(fmt.Sprintf("%s\n", n.Content))
		}
	}

	sb.WriteString("\n## TASK\n")
	sb.WriteString("Based solely on the source data above, write a correction that:\n")
	sb.WriteString("1. States what the evidence actually supports\n")
	sb.WriteString("2. Identifies where the anomaly or contradiction diverges from that evidence\n")
	sb.WriteString("3. Proposes what the corrected understanding should be\n")
	sb.WriteString("\nCite source point IDs inline (e.g. [abc-123]). 2-4 sentences only.\n")

	return sb.String()
}

// parseFindings extracts the JSON array from the LLM response.
func (s *Synthesizer) parseFindings(response string) ([]rawFinding, error) {
	response = strings.TrimSpace(response)

	// Strip markdown code fences if present
	if strings.HasPrefix(response, "```") {
		lines := strings.Split(response, "\n")
		var inner []string
		for _, line := range lines {
			if strings.HasPrefix(line, "```") {
				continue
			}
			inner = append(inner, line)
		}
		response = strings.Join(inner, "\n")
	}

	// Find the JSON array
	start := strings.Index(response, "[")
	end := strings.LastIndex(response, "]")
	if start == -1 || end == -1 || end <= start {
		return nil, fmt.Errorf("no JSON array found in response")
	}
	jsonStr := response[start : end+1]

	var findings []rawFinding
	if err := json.Unmarshal([]byte(jsonStr), &findings); err != nil {
		return nil, fmt.Errorf("unmarshaling findings JSON: %w", err)
	}

	return findings, nil
}

// --- helpers ---

func stringField(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

func isValidFindingType(t string) bool {
	switch collections.FindingType(t) {
	case collections.FindingConnection,
		collections.FindingContradiction,
		collections.FindingCluster,
		collections.FindingAnomaly:
		return true
	}
	return false
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

func uniqueDomains(domains []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(domains))
	for _, d := range domains {
		if d == "" {
			continue
		}
		if _, ok := seen[d]; !ok {
			seen[d] = struct{}{}
			result = append(result, d)
		}
	}
	return result
}
