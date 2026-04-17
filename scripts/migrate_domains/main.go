// migrate_domains retroactively classifies semantic domains for all chunks in
// kae_chunks that lack a semantic_domain payload field.
//
// Usage:
//
//	go run ./scripts/migrate_domains [--dry-run] [--batch 100]
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/meistro57/kae/internal/ingestion"
	"github.com/meistro57/kae/internal/llm"
)

const (
	qdrantURL       = "http://localhost:6333"
	collection      = "kae_chunks"
	defaultBatch    = 100
)

func main() {
	dryRun := flag.Bool("dry-run", false, "print what would happen without writing to Qdrant")
	batchSize := flag.Int("batch", defaultBatch, "chunks per LLM classification call")
	flag.Parse()

	_ = godotenv.Load()

	keys := llm.ProviderKeys{
		OpenRouterKey: os.Getenv("OPENROUTER_API_KEY"),
		GeminiKey:     os.Getenv("GEMINI_API_KEY"),
		AnthropicKey:  os.Getenv("ANTHROPIC_API_KEY"),
		OpenAIKey:     os.Getenv("OPENAI_API_KEY"),
	}

	model := os.Getenv("MIGRATE_MODEL")
	if model == "" {
		model = "google/gemini-2.5-flash"
	}

	provider, err := llm.NewProvider(model, keys)
	if err != nil {
		fmt.Fprintf(os.Stderr, "LLM init error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Using model: %s\n", provider.ModelName())
	if *dryRun {
		fmt.Println("DRY RUN — no Qdrant writes")
	}

	// Scroll all chunks that don't yet have semantic_domain
	filter := map[string]any{
		"must_not": []map[string]any{
			{"key": "semantic_domain", "is_empty": map[string]any{}},
		},
	}
	// Actually, scroll ALL and skip ones that already have it — simpler
	_ = filter

	var offset any = nil
	total, updated := 0, 0

	for {
		scrolled, nextOffset, err := scrollChunks(offset, 250)
		if err != nil {
			fmt.Fprintf(os.Stderr, "scroll error: %v\n", err)
			os.Exit(1)
		}

		// Filter to chunks needing classification
		var toClassify []scrollPoint
		for _, p := range scrolled {
			if p.domain == "" {
				toClassify = append(toClassify, p)
			}
		}

		total += len(scrolled)
		fmt.Printf("Scrolled %d chunks (%d need classification)...\n", len(scrolled), len(toClassify))

		// Process in LLM batch sizes
		for i := 0; i < len(toClassify); i += *batchSize {
			end := i + *batchSize
			if end > len(toClassify) {
				end = len(toClassify)
			}
			batch := toClassify[i:end]

			texts := make([]string, len(batch))
			sources := make([]string, len(batch))
			for j, p := range batch {
				texts[j] = p.text
				sources[j] = p.source
			}

			fmt.Printf("  Classifying batch of %d chunks...\n", len(batch))
			domains := ingestion.ClassifyDomainBatch(texts, sources, provider)

			if *dryRun {
				for j, d := range domains {
					fmt.Printf("  [DRY] %s → %s (%.2f)\n", batch[j].id, d.Domain, d.Confidence)
				}
				updated += len(batch)
				continue
			}

			// Build payload update points
			type payloadUpdate struct {
				Points  []any          `json:"points"`
				Payload map[string]any `json:"payload"`
			}

			for j, d := range domains {
				if err := setPayload(batch[j].id, batch[j].runTopic, d.Domain, d.Confidence); err != nil {
					fmt.Fprintf(os.Stderr, "  payload update error for %s: %v\n", batch[j].id, err)
				}
			}
			updated += len(batch)
		}

		if nextOffset == nil {
			break
		}
		offset = nextOffset
		time.Sleep(100 * time.Millisecond) // rate-limit Qdrant
	}

	fmt.Printf("\nDone. Scanned %d chunks, classified %d.\n", total, updated)
}

type scrollPoint struct {
	id       any
	text     string
	source   string
	runTopic string
	domain   string
}

func scrollChunks(offset any, limit int) ([]scrollPoint, any, error) {
	body := map[string]any{
		"limit":        limit,
		"with_payload": true,
	}
	if offset != nil {
		body["offset"] = offset
	}

	data, err := qdrantPost(fmt.Sprintf("/collections/%s/points/scroll", collection), body)
	if err != nil {
		return nil, nil, err
	}

	var result struct {
		Result struct {
			Points []struct {
				ID      any            `json:"id"`
				Payload map[string]any `json:"payload"`
			} `json:"points"`
			NextPageOffset any `json:"next_page_offset"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, nil, fmt.Errorf("scroll unmarshal: %w", err)
	}

	points := make([]scrollPoint, 0, len(result.Result.Points))
	for _, p := range result.Result.Points {
		points = append(points, scrollPoint{
			id:       p.ID,
			text:     strVal(p.Payload, "text"),
			source:   strVal(p.Payload, "source"),
			runTopic: firstNonEmpty(strVal(p.Payload, "run_topic"), strVal(p.Payload, "topic")),
			domain:   strVal(p.Payload, "semantic_domain"),
		})
	}

	return points, result.Result.NextPageOffset, nil
}

func setPayload(id any, runTopic, domain string, confidence float64) error {
	body := map[string]any{
		"payload": map[string]any{
			"semantic_domain":   domain,
			"domain_confidence": confidence,
			"run_topic":         runTopic,
		},
		"points": []any{id},
	}
	_, err := qdrantPost(fmt.Sprintf("/collections/%s/points/payload", collection), body)
	return err
}

func qdrantPost(path string, body any) ([]byte, error) {
	b, _ := json.Marshal(body)
	resp, err := http.Post(qdrantURL+path, "application/json", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

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
