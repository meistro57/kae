# KAE Agents

Knowledge Archaeology Engine — agent architecture reference.

---

## Overview

KAE runs a single engine agent in a continuous cycle. It uses two LLM clients (a deep-thinking model and a fast bulk model), an in-memory graph, and an optional Qdrant vector store for semantic memory across cycles.

```
seed → [ ingest → think → connect → anomaly scan → report ] × N cycles
```

---

## Models

| Role | Default | Flag | Purpose |
|------|---------|------|---------|
| **brain** | `deepseek/deepseek-r1` | `-model` | Deep reasoning, connection finding, anomaly detection |
| **fast** | `google/gemini-flash-1.5-8b` | `-fast` | Bulk ingestion summaries |

Both route through OpenRouter (`OPENROUTER_API_KEY`).

---

## Phases

Each cycle runs these phases in order:

### 1. SEEDING
Runs once at startup. If no `-seed` flag is given, the brain model is asked to choose a foundational concept that maximally reconciles contradictions in human knowledge. Falls back to `"consciousness"` on empty response.

### 2. INGESTING
Three sources are queried in parallel for the current topic:

1. **Wikipedia** — intro extract; URL stored as a node source
2. **arxiv** — top 3 papers by relevance; abstracts + URLs stored
3. **Project Gutenberg** — up to 2 classical/ancient texts via Gutendex; metadata + excerpt stored

All three are then passed to the fast model, which supplements with what they omit: fringe research, ancient wisdom traditions, cross-domain philosophical implications, and documented anomalies. If all external sources fail, the fast model generates a summary from scratch.

A graph node is created (`domain=ingested`, `weight=1.0`) and asynchronously embedded into Qdrant. All source URLs are stored on the node.

### 3. THINKING
The brain model (R1) receives:
- Current topic and graph summary
- Semantically related concepts already in Qdrant (if available)
- Ingested knowledge text from the current cycle (up to 3000 chars of Wikipedia + model supplement)

R1's `<think>` blocks are streamed live to the UI thinking panel. The model is prompted to return a structured response:

```
CONNECTIONS: <concept1> | <concept2> | <concept3>
ANOMALY: <where consensus fails>
NEXT: <next concept to research>
```

### 4. CONNECTING
The structured response is parsed and applied to the graph:
- Each connection becomes an `inferred` node (weight 0.5)
- Edges are created (`connects_to`, confidence 0.7) — both endpoint node weights increase by the confidence value
- If an anomaly is detected, a special node is created (`[ANOMALY] {topic}`, weight 2.0, `anomaly=true`)
- All new nodes are asynchronously embedded and stored in Qdrant

### 5. ANOMALY SCAN
Queries the graph for all nodes flagged `anomaly=true` and emits the count to the UI.

### 6. REPORT
Generates a markdown summary for the cycle: timestamp, graph stats, and the top 5 nodes by weight (anomaly nodes marked with ⚠). Appended to the live report panel.

---

## Graph

In-memory, thread-safe. No persistence between runs (Qdrant provides cross-run vector memory).

**Nodes** — `{ID, Label, Domain, Sources[], Weight, Anomaly}`

**Edges** — `{From, To, Relation, Confidence}`

**Weight accumulation:**
- Node created: initial weight from domain (`ingested=1.0`, `inferred=0.5`, `anomaly=2.0`)
- Node re-encountered: weight accumulates additively
- Edge added: both endpoint nodes gain `+confidence`

**Domains:**
- `ingested` — directly researched topic
- `inferred` — connection identified by R1
- `anomaly` — consensus gap flagged by R1

---

## Vector Store (Qdrant)

Optional. The engine degrades gracefully if Qdrant is unreachable.

| Detail | Value |
|--------|-------|
| Collection | `kae_nodes` |
| Dimensions | 128 |
| Distance | Cosine |
| Default URL | `http://localhost:6333` (override: `QDRANT_URL`) |

**Embedding method:** local feature hashing — no external API. Each word is hashed to a dimension index and sign, accumulated and L2-normalised. Deterministic, zero cost.

**Usage per cycle:**
- After node upsert → embed + store in Qdrant (async, best-effort)
- Before R1 thinking → query Qdrant for 5 semantically similar existing nodes and inject as context

**UI stats:** header shows `● qdrant  vectors: N` when connected, `○ qdrant offline` otherwise.

---

## Event Flow

```
Engine (goroutine)
  └─ emits Event{Phase, Focus, ThinkChunk, OutputChunk, ReportLine, GraphSnap}
       ↓  buffered channel (256)
  UI Update()
       ↓
  thinkBuf / outputBuf / reportBuf
       ↓
  View() → terminal panels
```

Events are dropped (not blocked) if the channel is full — the UI is never allowed to stall the agent.

---

## Stopping

- **`-cycles N`** — stop after N cycles (0 = run until killed)
- **`q` / `Ctrl+C`** — graceful quit from the UI

---

## Configuration

All config is loaded from environment variables or `.env`:

| Variable | Required | Default | Purpose |
|----------|----------|---------|---------|
| `OPENROUTER_API_KEY` | yes | — | LLM access |
| `QDRANT_URL` | no | `http://localhost:6333` | Vector store |

CLI flags override model and cycle settings at runtime. See `./kae --help`.

---

## Setup

```bash
./setup.sh   # installs deps, builds binary, starts Qdrant in Docker
./kae        # run with defaults
./kae -seed "quantum entanglement" -cycles 10 -debug
```
