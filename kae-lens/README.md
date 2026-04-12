# KAE Lens

> *KAE ingests reality. Lens focuses it.*

KAE Lens is an autonomous post-processing intelligence layer that sits on top of the Knowledge Archaeology Engine (KAE). It fires when KAE deposits new knowledge into Qdrant, reasons over the full topology of the ingested knowledge graph, and surfaces connections, contradictions, higher-order clusters, and anomalies that KAE never explicitly made.

---

## Ecosystem

```
KAE  (Knowledge Archaeology Engine)
 └── ingests broad human knowledge (Wikipedia, arXiv, etc.)
 └── chunks, embeds, deposits into Qdrant → kae_knowledge

KAE LENS
 └── event-driven: fires when new KAE data lands in Qdrant
 └── adaptive density assessment → variable search width
 └── LLM reasoning (DeepSeek R1 / Gemini Flash via OpenRouter)
 └── writes findings back to Qdrant → kae_lens_findings
 └── live dashboard: TUI (Bubbletea) + Web (SSE, port 8080)
```

---

## Quick Start

### 1. Start Qdrant
```bash
make qdrant-up
# Dashboard: http://localhost:6333/dashboard
```

### 2. Configure
Edit `config/lens.yaml` — or set environment variables:
```bash
export LENS_OPENROUTER_API_KEY="your-key"
export LENS_OPENAI_API_KEY="your-key"        # for embeddings
export LENS_QDRANT_API_KEY=""                # blank for local
```

### 3. Build
```bash
make build
```

### 4. Run Lens
```bash
make run-lens
# TUI launches in terminal
# Web dashboard: http://localhost:8080
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
Watcher polls kae_knowledge (every 30s)
  └── finds points where lens_processed == false
  └── marks them processed (optimistic)
  └── dispatches batch to Reasoner

Reasoner (per point in batch)
  └── DensityCalculator: probe local vector density
       └── sparse region  → width=50, threshold=0.60
       └── medium region  → width=20, threshold=0.70
       └── dense region   → width=6,  threshold=0.80
  └── QueryNeighbors: adaptive Qdrant similarity search
  └── Synthesizer: build prompt → call LLM → parse findings JSON
  └── Writer: embed findings → upsert to kae_lens_findings
  └── Emit events → TUI channel + SSE broker
```

### Finding Types

| Type | Icon | Meaning |
|---|---|---|
| `connection` | 🔗 | Unexpected cross-domain semantic link |
| `contradiction` | ⚡ | Conflicting claims between knowledge nodes |
| `cluster` | 🌀 | Emergent concept group KAE never tagged |
| `anomaly` | 🔴 | Outlier breaking mainstream consensus |

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
kae/
├── cmd/
│   ├── kae/          ← KAE ingestion binary
│   └── lens/         ← KAE Lens binary (this project)
├── internal/
│   ├── config/       ← shared config loader
│   ├── qdrantclient/ ← shared Qdrant gRPC client
│   ├── llm/          ← shared OpenRouter + OpenAI client
│   ├── graph/        ← shared event types
│   └── lens/
│       ├── watcher.go       ← event trigger
│       ├── density.go       ← adaptive net-width
│       ├── reasoner.go      ← core agent loop
│       ├── synthesizer.go   ← LLM reasoning
│       ├── writer.go        ← writes findings to Qdrant
│       ├── tui/             ← Bubbletea terminal dashboard
│       └── web/             ← HTTP + SSE web dashboard
├── collections/      ← shared Qdrant payload schemas
├── config/
│   └── lens.yaml     ← configuration
└── Makefile
```

---

## Qdrant Collections

**`kae_knowledge`** — written by KAE, read by Lens
```json
{
  "title": "Quantum entanglement",
  "content": "...",
  "domain": "physics",
  "source": "wikipedia",
  "ingested_at": 1712345678,
  "lens_processed": false,
  "anomaly_score": 0.0
}
```

**`kae_lens_findings`** — written by Lens
```json
{
  "type": "connection",
  "confidence": 0.87,
  "source_point_ids": ["uuid1", "uuid2"],
  "domains": ["physics", "philosophy"],
  "summary": "Quantum entanglement and the Vedic Akasha field share...",
  "reasoning_trace": "Step 1: anchor is quantum entanglement...",
  "batch_id": "20260410-142305.123",
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
| `watcher.poll_interval_seconds` | 30 | How often to check for new KAE points |
| `watcher.batch_size` | 10 | Points per reasoning batch |
| `llm.reasoning_model` | deepseek/deepseek-r1 | Model for deep reasoning |
| `llm.fast_model` | google/gemini-flash-1.5 | Model for lighter batches |
| `llm.min_confidence` | 0.65 | Minimum finding confidence threshold |
| `web.port` | 8080 | Web dashboard port |

---

*Part of the KAE Ecosystem*
