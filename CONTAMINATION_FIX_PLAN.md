# KAE Domain Contamination: Root Cause & Fix

**Date:** April 17, 2026  
**Issue:** Domain tags contaminated across 60% of ingested chunks  
**Root Cause:** Single `Topic` field serving dual purpose

---

## THE SMOKING GUN

**File:** `internal/agent/engine.go`  
**Lines:** 242-245, 257-259, 271-274 (and similar patterns)

```go
// Wikipedia ingestion
for i, c := range chunks {
    sc[i] = ingestion.SourceChunk{
        Text: c, 
        Source: result.URL, 
        Topic: topic  // ← PROBLEM: 'topic' is the cycle's exploration focus
    }
}

// arXiv ingestion  
for _, p := range papers {
    for _, c := range ingestion.PaperToChunks(p) {
        sc = append(sc, ingestion.SourceChunk{
            Text: c, 
            Source: p.URL, 
            Topic: topic  // ← Same issue
        })
    }
}
```

**What's happening:**
1. `topic` variable = current cycle's exploration theme (e.g., "The 1984 revision of the Lateran Treaty")
2. Every chunk ingested during that cycle gets `Topic: topic` 
3. Result: Marcus Aurelius text tagged as "Lateran Treaty," medical papers as "Pope," etc.

**The field is overloaded:**
- **Intended purpose:** Track which concept/theme this chunk relates to (chunk-level semantic classification)
- **Actual usage:** Store the run's current exploration topic (run-level metadata)

---

## DATA STRUCTURE

**Current:** `internal/ingestion/types.go`

```go
type SourceChunk struct {
    Text   string
    Source string // URL or title
    Topic  string // concept this relates to ← AMBIGUOUS
}
```

**Storage:** `internal/store/qdrant.go`

```go
type Chunk struct {
    ID     string
    Text   string
    Source string
    Topic  string  // ← Gets stored in Qdrant payload
    RunID  string
    Vector []float32
}
```

---

## THE FIX

### Step 1: Separate Concerns

**New structure:**

```go
type SourceChunk struct {
    Text          string
    Source        string   // URL or title
    RunTopic      string   // What the RUN is exploring (metadata)
    SemanticDomain string  // What THIS CHUNK is actually about (content)
    DomainConfidence float64 // 0.0-1.0
}
```

**Storage update:**

```go
type Chunk struct {
    ID               string
    Text             string
    Source           string
    RunTopic         string   // Exploration theme
    SemanticDomain   string   // Chunk content classification
    DomainConfidence float64
    RunID            string
    Vector           []float32
}
```

### Step 2: Add Semantic Domain Classifier

**New file:** `internal/ingestion/classifier.go`

```go
package ingestion

import (
    "github.com/meistro57/kae/internal/llm"
)

// ClassifyDomain determines the semantic domain of a text chunk
func ClassifyDomain(text, source string, llm llm.Provider) (domain string, confidence float64) {
    prompt := fmt.Sprintf(`Classify the semantic domain of this text chunk.

SOURCE: %s
TEXT: %s

Return ONLY a JSON object with these fields:
{
  "domain": "primary subject area (e.g., 'Roman History', 'Medical Research', 'Hermetic Philosophy')",
  "confidence": 0.95  // 0.0-1.0 score
}

Do not explain. Output ONLY valid JSON.`, source, text)

    response := llm.Generate(prompt, 100)
    
    // Parse JSON response
    var result struct {
        Domain     string  `json:"domain"`
        Confidence float64 `json:"confidence"`
    }
    
    if err := json.Unmarshal([]byte(response), &result); err != nil {
        // Fallback: extract from source URL if LLM fails
        return inferDomainFromSource(source), 0.5
    }
    
    return result.Domain, result.Confidence
}

func inferDomainFromSource(source string) string {
    // Heuristics for common sources
    if strings.Contains(source, "pubmed") {
        return "Medical Research"
    }
    if strings.Contains(source, "arxiv.org") {
        return "Academic Research"
    }
    if strings.Contains(source, "wikipedia") {
        return "Encyclopedia"
    }
    return "Unknown"
}
```

### Step 3: Update Ingestion Pipeline

**File:** `internal/agent/engine.go`

**BEFORE:**
```go
for i, c := range chunks {
    sc[i] = ingestion.SourceChunk{
        Text: c, 
        Source: result.URL, 
        Topic: topic
    }
}
```

**AFTER:**
```go
for i, c := range chunks {
    // Fast domain classification (batch these)
    domain, conf := ingestion.ClassifyDomain(c, result.URL, e.fast)
    
    sc[i] = ingestion.SourceChunk{
        Text:             c,
        Source:           result.URL,
        RunTopic:         topic,  // What the run is exploring
        SemanticDomain:   domain, // What this chunk is about
        DomainConfidence: conf,
    }
}
```

### Step 4: Optimize with Batch Classification

Since you're ingesting 50-200 chunks per cycle, classify in batches:

```go
// New function in classifier.go
func ClassifyDomainBatch(chunks []string, sources []string, llm llm.Provider) []DomainResult {
    prompt := fmt.Sprintf(`Classify semantic domains for these text chunks.

%s

Return ONLY a JSON array with domain classifications:
[
  {"index": 0, "domain": "Roman History", "confidence": 0.95},
  {"index": 1, "domain": "Medical Research", "confidence": 0.88},
  ...
]`, buildChunkList(chunks, sources))

    // Process with fast model
    response := llm.Generate(prompt, 2000)
    
    // Parse and return
    var results []DomainResult
    json.Unmarshal([]byte(response), &results)
    return results
}
```

---

## MIGRATION STRATEGY

### Option A: Full Retroactive Fix (Recommended)

1. **Export all existing chunks** from `kae_chunks`
2. **Run batch classification** on all 4,690 chunks
3. **Update Qdrant payloads** with correct semantic_domain
4. **Preserve run_topic** for historical context

**Script:** `scripts/migrate_domains.go`

```go
func MigrateChunks() {
    // 1. Scroll all chunks
    chunks := qdrant.ScrollAll("kae_chunks")
    
    // 2. Batch classify (100 at a time)
    for batch := range chunks.InBatches(100) {
        domains := ingestion.ClassifyDomainBatch(
            batch.Texts(), 
            batch.Sources(), 
            fastModel,
        )
        
        // 3. Update Qdrant
        for i, chunk := range batch {
            qdrant.UpdatePayload(chunk.ID, map[string]any{
                "run_topic":         chunk.Topic, // Preserve old value
                "semantic_domain":   domains[i].Domain,
                "domain_confidence": domains[i].Confidence,
            })
        }
    }
}
```

### Option B: Clean Slate

1. **Delete contaminated collections**: `kae_chunks`, `kae_nodes`
2. **Keep clean collections**: `gpt_conversations`, `qmu_forum`
3. **Re-run best seeds** with fixed pipeline

---

## COST ANALYSIS

**Batch Classification (Gemini Flash 2.5):**
- 4,690 chunks × ~200 tokens/chunk = 938,000 tokens
- Batch size 100 = 47 API calls
- Cost: ~$0.10 total (Gemini Flash is $0.10/1M tokens)

**Time:**
- 47 calls × 2-3 seconds = ~2-3 minutes total

**Worth it?** Absolutely. Fixes 60% contamination for $0.10.

---

## TESTING PLAN

### 1. Unit Tests

**File:** `internal/ingestion/classifier_test.go`

```go
func TestClassifyDomain(t *testing.T) {
    tests := []struct{
        text     string
        source   string
        expected string
    }{
        {
            text:     "Marcus Aurelius was Roman Emperor from 161 to 180 AD...",
            source:   "Meditations - Marcus Aurelius",
            expected: "Roman History",
        },
        {
            text:     "RETRO-POPE: A Retrospective, Multicenter, Real-World Study...",
            source:   "https://pubmed.ncbi.nlm.nih.gov/38022829/",
            expected: "Medical Research",
        },
        {
            text:     "The Kybalion is dedicated to Hermes Trismegistus...",
            source:   "The Kybalion - Three Initiates",
            expected: "Hermetic Philosophy",
        },
    }
    
    mockLLM := &MockProvider{}
    for _, tt := range tests {
        domain, _ := ClassifyDomain(tt.text, tt.source, mockLLM)
        if !strings.Contains(domain, tt.expected) {
            t.Errorf("got %s, want %s", domain, tt.expected)
        }
    }
}
```

### 2. Integration Test

Run a small cycle with fixed pipeline and verify:
- No cross-contamination between run_topic and semantic_domain
- Confidence scores reasonable (>0.7 average)
- Domain labels semantic and specific

### 3. Regression Test

Compare run convergence BEFORE and AFTER fix:
- Old contaminated runs: 0.7% overlap
- New clean runs: Expected 5-15% overlap for related seeds

---

## IMPLEMENTATION CHECKLIST

- [ ] Create `internal/ingestion/classifier.go`
- [ ] Add unit tests in `classifier_test.go`
- [ ] Update `SourceChunk` struct in `types.go`
- [ ] Update `Chunk` struct in `store/qdrant.go`
- [ ] Update ingestion calls in `agent/engine.go`
- [ ] Create migration script `scripts/migrate_domains.go`
- [ ] Test on small sample (10 chunks)
- [ ] Run full migration on 4,690 chunks
- [ ] Verify Lens findings decrease from 60% to <10%
- [ ] Re-run best seeds (forward invariance, Vatican sovereignty)
- [ ] Compare convergence metrics
- [ ] Update documentation in README.md

---

## EXPECTED OUTCOMES

**Before Fix:**
- Domain contamination: 60%
- Cross-run convergence: 0.7%
- Lens anomaly findings: ~60% are false positives
- Top concepts: Contaminated (Pope from RETRO-POPE)

**After Fix:**
- Domain contamination: <5%
- Cross-run convergence: 5-15% (for related seeds)
- Lens anomaly findings: ~90% are genuine
- Top concepts: Legitimate emergent patterns

**Meta-Graph Population:**
Once domains are clean, concepts will properly merge across runs, populating the currently-empty `kae_meta_graph` collection.

---

## BONUS: Enhanced Domain Taxonomy

Once basic fix is working, add hierarchical classification:

```go
type DomainClassification struct {
    Primary   string  // "Science"
    Secondary string  // "Physics"
    Tertiary  string  // "Quantum Mechanics"
    Confidence float64
}
```

This enables:
- **Bridge detection**: Concepts connecting "Philosophy" ↔ "Mathematics"
- **Moat detection**: Domains with no connecting concepts
- **Domain evolution tracking**: How concepts migrate across domains over cycles

---

## ALTERNATIVE: Embedding-Based Classification

If LLM classification is too slow, use embedding similarity:

```go
// Create domain prototypes
var domainPrototypes = map[string][]float32{
    "Roman History":      embedder.Embed("Roman Empire history philosophy Stoicism"),
    "Medical Research":   embedder.Embed("clinical study patients treatment outcomes"),
    "Hermetic Philosophy": embedder.Embed("occult mysticism Hermes Trismegistus esoteric"),
    // ... add more
}

func ClassifyByEmbedding(text string, embedder *Embedder) string {
    vec := embedder.Embed(text)
    
    var bestDomain string
    var bestScore float32
    
    for domain, prototype := range domainPrototypes {
        score := cosineSimilarity(vec, prototype)
        if score > bestScore {
            bestScore = score
            bestDomain = domain
        }
    }
    
    return bestDomain
}
```

**Pros:** 100x faster, no API costs  
**Cons:** Fixed taxonomy, needs prototype tuning

---

**Ready to implement?** Start with the classifier unit tests, then run migration on a 10-chunk sample to validate before the full 4,690-chunk batch.
