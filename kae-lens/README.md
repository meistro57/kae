# KAE Lens

> *KAE ingests reality. Lens focuses it.*

KAE Lens is an autonomous post-processing intelligence layer that sits on top of the Knowledge Archaeology Engine (KAE). It fires when KAE deposits new knowledge into Qdrant, reasons over the full topology of the ingested knowledge graph, and surfaces connections, contradictions, higher-order clusters, and anomalies that KAE never explicitly made.

---

## Ecosystem

```
KAE  (Knowledge Archaeology Engine)
 └── ingests broad human knowledge (Wikipedia, arXiv, etc.)
 └── chunks, embeds, deposits into Qdrant → kae_chunks + kae_nodes

KAE LENS
 └── event-driven: reads kae_chunks, fires when new points appear
 └── adaptive density assessment → variable search width
 └── LLM reasoning (DeepSeek R1 / Gemini Flash via OpenRouter)
 └── writes findings back to Qdrant → kae_lens_findings
 └── live dashboard: TUI (Bubbletea) + Web (SSE, port 8080)
```

---

## Quick Start

### 1. Start Qdrant (both REST and gRPC ports required)
```bash
make qdrant-up
# REST dashboard: http://localhost:6333/dashboard
# gRPC (required by Lens): localhost:6334
```

### 2. Configure
Lens picks up the main KAE `.env` keys automatically — no separate config needed if you already have KAE set up:
```bash
# Already in your KAE .env — Lens reads these as fallback:
OPENROUTER_API_KEY=your-key    # for LLM reasoning
OPENAI_API_KEY=your-key        # for embeddings (optional — falls back to OpenRouter)

# Lens-specific overrides (take priority if set):
LENS_OPENROUTER_API_KEY=your-key
LENS_OPENAI_API_KEY=your-key
LENS_QDRANT_API_KEY=           # blank for local Qdrant
```

### 3. Build
```bash
make build
```

### 4. Run Lens
```bash
make run-lens          # TUI + web dashboard at http://localhost:8080
make run-lens-notui    # headless, plain logs (good for scripting)
```

### 5. Run KAE (ingestion — separate terminal)
```bash
make run-kae
# Lens will detect new points within poll_interval_seconds
```

---

## How It Works

### The Agent Loop

```
Watcher polls kae_chunks (every 30s)
  └── finds points where lens_processed is absent or false
  └── marks them processed optimistically (before reasoning starts)
  └── dispatches batch to Reasoner

Reasoner (per point in batch)
  └── DensityCalculator: probe local vector density
       └── very_sparse → width=50, threshold=0.60
       └── sparse      → width=35, threshold=0.60
       └── medium      → width=20, threshold=0.70
       └── dense       → width=12, threshold=0.80
       └── very_dense  → width=6,  threshold=0.80
  └── QueryNeighbors: adaptive Qdrant similarity search
  └── Synthesizer: build prompt → call LLM → parse findings JSON
  └── Writer: embed findings → upsert to kae_lens_findings
  └── Emit events → TUI channel + SSE broker
```

### Finding Types

| Type | Meaning |
|---|---|
| `connection` | Unexpected cross-domain semantic link |
| `contradiction` | Conflicting claims between knowledge nodes |
| `cluster` | Emergent concept group KAE never tagged |
| `anomaly` | Outlier breaking mainstream consensus |

### Adaptive Density

The search width and score threshold adapt to local point density:

| Density | Nearby Points | Width | Threshold |
|---|---|---|---|
| very_sparse | 0 | 50 | 0.60 |
| sparse | 1–10 | 35 | 0.60 |
| medium | 11–50 | 20 | 0.70 |
| dense | 51–200 | 12 | 0.80 |
| very_dense | 200+ | 6 | 0.80 |

---

## Repository Structure

```
kae-lens/
├── cmd/
│   └── lens/         ← Lens binary entry point
├── internal/
│   ├── config/       ← config loader (YAML + env var overrides)
│   ├── qdrantclient/ ← Qdrant gRPC client (handles UUID + numeric IDs)
│   ├── llm/          ← OpenRouter chat + embedding client
│   ├── graph/        ← shared event types (FindingEvent, BatchEvent)
│   └── lens/
│       ├── watcher.go       ← polls for unprocessed points
│       ├── density.go       ← adaptive search width by vector density
│       ├── reasoner.go      ← core agent loop + schema mapping
│       ├── synthesizer.go   ← LLM prompt construction + response parsing
│       ├── writer.go        ← embeds findings → upserts to kae_lens_findings
│       ├── tui/             ← Bubbletea terminal dashboard
│       └── web/             ← HTTP + SSE web dashboard (port 8080)
├── collections/      ← Qdrant payload schemas (KnowledgePoint, LensFinding)
├── config/
│   └── lens.yaml     ← configuration (all fields with defaults)
└── Makefile
```

---

## Qdrant Collections

**`kae_chunks`** — written by KAE, read by Lens
```json
{
  "source": "The Kybalion - Three Initiates",
  "text": "...",
  "topic": "Non-duality",
  "run_id": "run_1775921843",
  "lens_processed": false
}
```
> Lens maps `source` → title, `text` → content, `topic` → domain.
> Points without a `lens_processed` field are treated as unprocessed.

**`kae_lens_findings`** — written by Lens
```json
{
  "type": "connection",
  "confidence": 0.87,
  "source_point_ids": ["270359567535248", "271459079163459"],
  "domains": ["physics", "philosophy"],
  "summary": "Quantum entanglement and the Vedic Akasha field share...",
  "reasoning_trace": "Step 1: anchor is quantum entanglement...",
  "batch_id": "20260412-185800.500",
  "created_at": 1712345900
}
```

---

## Turtles All The Way Down

Because Lens writes findings back into Qdrant as vectorized points, a future **Pass 2** can run Lens against `kae_lens_findings` itself — finding connections *between findings*, building third-order knowledge structures. Each pass produces higher-order understanding from the same raw data.

---

## Configuration Reference

See `config/lens.yaml` — all fields documented inline. Key settings:

| Field | Default | Description |
|---|---|---|
| `qdrant.knowledge_collection` | `kae_chunks` | Collection Lens reads from |
| `qdrant.findings_collection` | `kae_lens_findings` | Collection Lens writes to |
| `watcher.poll_interval_seconds` | 30 | How often to check for new points |
| `watcher.batch_size` | 10 | Points per reasoning batch |
| `llm.reasoning_model` | `deepseek/deepseek-r1` | Model for deep reasoning |
| `llm.fast_model` | `google/gemini-2.5-flash` | Model for lighter batches |
| `llm.min_confidence` | 0.65 | Minimum finding confidence threshold |
| `web.port` | 8080 | Web dashboard port |

---

*Part of the KAE Ecosystem*
