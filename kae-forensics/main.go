package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	collectionName  = "kae_study"
	targetAddr      = "localhost:6334"
	embeddingModel  = "text-embedding-3-small"
	embeddingDims   = 384
	batchSize       = 50
)

func main() {
	dryRun := true
	if len(os.Args) > 1 && os.Args[1] == "--repair" {
		dryRun = false
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" && !dryRun {
		log.Fatal("OPENAI_API_KEY not set")
	}

	conn, err := grpc.Dial(targetAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to connect to gRPC: %v", err)
	}
	defer conn.Close()

	pointsClient := qdrant.NewPointsClient(conn)
	ctx := context.Background()

	fmt.Printf("Forensic Scan + Repair: %s (Dry Run: %v)\n", collectionName, dryRun)
	fmt.Println("---------------------------------------------------")

	var offset *qdrant.PointId
	totalScanned, anomalies, repaired := 0, 0, 0

	// Collect weak-vector points for batched repair
	type pendingPoint struct {
		id   *qdrant.PointId
		text string
	}
	var pending []pendingPoint

	flush := func() {
		if len(pending) == 0 {
			return
		}
		texts := make([]string, len(pending))
		for i, p := range pending {
			texts[i] = p.text
		}
		vecs, err := embedBatch(apiKey, texts)
		if err != nil {
			fmt.Printf("   embed error: %v\n", err)
			pending = pending[:0]
			return
		}
		for i, p := range pending {
			_, err := pointsClient.UpdateVectors(ctx, &qdrant.UpdatePointVectors{
				CollectionName: collectionName,
				Points: []*qdrant.PointVectors{
					{
						Id:      p.id,
						Vectors: qdrant.NewVectors(vecs[i]...),
					},
				},
			})
			if err != nil {
				fmt.Printf("   update failed for %s: %v\n", p.id.String(), err)
			} else {
				repaired++
			}
		}
		pending = pending[:0]
	}

	for {
		res, err := pointsClient.Scroll(ctx, &qdrant.ScrollPoints{
			CollectionName: collectionName,
			WithPayload:    qdrant.NewWithPayload(true),
			WithVectors:    qdrant.NewWithVectors(true),
			Limit:          qdrant.PtrOf(uint32(100)),
			Offset:         offset,
		})
		if err != nil {
			log.Fatalf("scroll failed: %v", err)
		}
		if len(res.Result) == 0 {
			break
		}

		for _, p := range res.Result {
			totalScanned++

			// Check vector magnitude via new DenseVector path (GetData() is deprecated in v1.13+)
			mag := float32(0)
			if p.Vectors != nil {
				if vec := p.Vectors.GetVector(); vec != nil {
					if d := vec.GetDense(); d != nil {
						for _, v := range d.Data {
							mag += v * v
						}
					}
				}
			}

			if mag >= 0.01 {
				continue // vector is healthy
			}

			anomalies++

			// Extract text from payload
			text := ""
			if doc, ok := p.Payload["document"]; ok {
				text = doc.GetStringValue()
			}
			if text == "" {
				if src, ok := p.Payload["source_material"]; ok {
					text = src.GetStringValue()
				}
			}

			if text == "" {
				fmt.Printf("  [%s] skipped — no embeddable text in payload\n", p.Id.String())
				continue
			}

			fmt.Printf("  [%s] weak vector — queued for re-embed\n", p.Id.String())

			if !dryRun {
				pending = append(pending, pendingPoint{id: p.Id, text: text})
				if len(pending) >= batchSize {
					flush()
				}
			}
		}

		offset = res.NextPageOffset
		if offset == nil {
			break
		}
	}

	if !dryRun {
		flush()
	}

	fmt.Println("---------------------------------------------------")
	fmt.Printf("Done. Scanned: %d | Anomalies: %d | Repaired: %d\n", totalScanned, anomalies, repaired)
	if dryRun && anomalies > 0 {
		fmt.Println("Run with --repair to fix them.")
	}
}

func embedBatch(apiKey string, texts []string) ([][]float32, error) {
	body, _ := json.Marshal(map[string]any{
		"model":      embeddingModel,
		"input":      texts,
		"dimensions": embeddingDims,
	})
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
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
			Index     int       `json:"index"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	// OpenAI returns results ordered by index field, not input order
	vecs := make([][]float32, len(texts))
	for _, d := range result.Data {
		vecs[d.Index] = d.Embedding
	}
	return vecs, nil
}
