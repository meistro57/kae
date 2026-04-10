# 🧠 Knowledge Archaeology Engine (KAE)

> *An autonomous agent that ingests human knowledge, follows contradictions without flinching, and builds a model of what the data actually points to — not what consensus says.*

---
<img width="2409" height="1762" alt="image" src="https://github.com/user-attachments/assets/a485885d-a69c-4e39-a592-303a963bc9bf" />


## What It Does

KAE is a self-directing CLI agent that:

1. **Chooses its own starting point** — no human bias injected at seed (unless you want to)
2. **Ingests knowledge** from Wikipedia, arxiv, Project Gutenberg, and the open web
3. **Thinks visibly** — DeepSeek R1's `<think>` blocks stream live to your terminal in real time
4. **Builds a knowledge graph** — concepts as nodes, relationships as edges, weighted by evidence
5. **Flags anomalies** — where mainstream consensus goes silent, contradicts itself, or suspiciously avoids a thread
6. **Generates a living report** — builds as it runs, saves automatically on exit

The hypothesis: if you feed it everything and let it run unbiased, it arrives at the same place the outliers, mystics, and fringe researchers already are. But this time with receipts.

---

## Requirements

- Go 1.22+
- An [OpenRouter](https://openrouter.ai) API key
- Docker (optional — for Qdrant vector memory via `setup.sh`)

```bash
# Install Go on WSL2/Ubuntu
sudo apt install golang-go

# Verify
go version
```

---

## Setup

```bash
# Clone or copy the project
cd kae

# Run the setup script — installs Go deps, builds binary, starts Qdrant v1.17.1 via Docker
./setup.sh

# Copy the generated .env and fill in your keys
# OPENROUTER_API_KEY is required; the rest are optional
```

`.env` reference:

```env
# Required
OPENROUTER_API_KEY=your_key_here

# Optional — Qdrant vector memory (setup.sh starts this automatically)
QDRANT_URL=http://localhost:6333

# Optional — real semantic embeddings via any OpenAI-compatible endpoint
# Without these, KAE falls back to feature hashing (fast, no API needed)
EMBEDDINGS_URL=https://api.openai.com
EMBEDDINGS_KEY=your_openai_key_here
EMBEDDINGS_MODEL=text-embedding-3-small
```

---

## Usage

```bash
# Fully autonomous — agent picks its own seed
go run .

# Seed it yourself
go run . --seed "observer effect"

# Limit cycles
go run . --cycles 50

# Resume from previous graph snapshot
go run . --resume-graph graph_snapshot.json --cycles 25

# Save current graph snapshot on exit
go run . --save-graph graph_snapshot.json

# Use a different thinking model
go run . --model "anthropic/claude-opus-4"

# Use a different fast/bulk model
go run . --fast "google/gemini-flash-1.5"

# Search across all previous runs (default: isolated to current run)
go run . --shared

# Debug mode (tail -f debug.log in a second terminal)
go run . --debug

# Build a binary
go build -o kae .
./kae --seed "consciousness"
```

---

## Terminal UI

```
╔══════════════════════════════════════════════════════════════════╗
║  🧠 KNOWLEDGE ARCHAEOLOGY ENGINE  ▸ THINKING   focus: observer  ║
║  nodes: 247   edges: 891   anomalies: 34   cycle: 12            ║
╠═════════════════════════╦════════════════════════════════════════╣
║  💭 THINKING            ║  🔗 EMERGENT CONCEPTS                 ║
║                         ║                                        ║
║  The observer effect    ║  1. consciousness                      ║
║  implies that the act   ║  2. quantum_field                      ║
║  of measurement itself  ║  3. [ANOMALY] observer_effect          ║
║  collapses the wave     ║  4. vedic_akasha                       ║
║  function. But physics  ║  5. zero_point_field                   ║
║  refuses to define      ╠════════════════════════════════════════╣
║  what an "observer"     ║  📄 LIVE REPORT                       ║
║  actually is...         ║                                        ║
╠═════════════════════════╣  ## Cycle 12 — 14:32:07               ║
║  ⚡ OUTPUT              ║  Nodes: 247 | Edges: 891               ║
║                         ║                                        ║
║  CONNECTIONS: quantum   ║  Emergent concepts:                    ║
║  field | vedic_akasha   ║  - consciousness (weight: 18.4)        ║
║  | zero_point_field     ║  - observer_effect ⚠ (weight: 14.2)   ║
║  ANOMALY: mainstream    ║  - quantum_field (weight: 11.8)        ║
║  physics avoids...      ║                                        ║
║  NEXT: akashic field    ║                                        ║
╚═════════════════════════╩════════════════════════════════════════╝
  q / ctrl+c — quit gracefully  |  report saves automatically
```

**Panels:**
- **💭 THINKING** — R1's raw `<think>` reasoning, streamed live in blue
- **⚡ OUTPUT** — The agent's structured conclusions and connections
- **🔗 EMERGENT CONCEPTS** — Top-weighted nodes in the knowledge graph, updated each cycle
- **📄 LIVE REPORT** — The growing synthesis document, builds automatically

---

## Project Structure

```
kae/
├── main.go                      # Entry point, CLI flags
├── go.mod                       # Dependencies
├── setup.sh                     # Start Qdrant (v1.17.1) + build binary
├── internal/
│   ├── config/
│   │   └── config.go            # Config loader (env vars + .env)
│   ├── llm/
│   │   └── client.go            # OpenRouter streaming client + R1 think-block parser
│   ├── graph/
│   │   └── graph.go             # Thread-safe knowledge graph (nodes, edges, anomalies)
│   ├── embeddings/
│   │   └── embedder.go          # Embedder interface: APIEmbedder (OpenAI-compat) or HashEmbedder fallback
│   ├── store/
│   │   └── qdrant.go            # Qdrant REST client — batch upsert, retry, payload indexes, hnsw_ef
│   ├── agent/
│   │   └── engine.go            # Core agent loop + batch sync queue for Qdrant
│   ├── ingestion/
│   │   └── wiki.go              # Wikipedia / arxiv / Gutenberg ingestion
│   └── ui/
│       └── app.go               # Bubbletea TUI — 4-panel layout
```

---

## How The Agent Loop Works

```
Phase 0  SEED       Agent chooses its own entry concept (or uses --seed)
Phase 1  INGEST     Pulls sources on current topic, chunks and stores them
Phase 2  THINK      DeepSeek R1 reasons visibly — you watch it think
Phase 3  CONNECT    Extracts connections, adds nodes/edges to knowledge graph
Phase 4  ANOMALY    Scans for where consensus goes silent or contradicts itself
Phase 5  PLAN       Decides what thread to pull next
Phase 6  REPORT     Updates the live markdown report
         └──────────► LOOP back to Phase 1 with new focus
```

Runs until:
- Graph stabilizes (diminishing new connections)
- `--cycles` limit reached
- You hit `q` or `ctrl+c` (graceful save)

---

## Models

KAE uses two models via OpenRouter:

| Role | Default | Purpose |
|------|---------|---------|
| **Brain** | `deepseek/deepseek-r1` | Deep reasoning, visible `<think>` blocks, connection-making |
| **Fast** | `google/gemini-flash-1.5-8b` | Bulk ingestion summarizing, cheap passes |

You can swap either via CLI flags. The brain model is what you *watch think*. The fast model is the workhorse that processes raw text cheaply so R1 can focus on synthesis.

---

## Vector Memory (Qdrant)

KAE uses Qdrant as optional persistent vector memory. When running, every concept node is embedded and stored — future cycles retrieve semantically similar nodes from previous sessions to ground the reasoning.

| Setting | Detail |
|---------|--------|
| Version | `qdrant/qdrant:v1.17.1` (pinned) |
| Collection | `kae_nodes` |
| Distance | Cosine |
| Payload indexes | `domain`, `label` (keyword, created before HNSW builds) |
| Batch size | 64 points per upsert request |
| Retry | 3 attempts, 100ms/300ms backoff |
| `hnsw_ef` | `max(k×4, 64)` at query time |
| Embedding fallback | Feature hashing (128-dim, no API needed) |
| Embedding (configured) | Any OpenAI-compatible endpoint — default `text-embedding-3-small` (1536-dim) |
| Memory isolation | Each run searches only its own chunks by default — use `--shared` to search across all runs |

Qdrant is fully optional. If unavailable, the agent runs entirely in-memory with no degradation to the core loop.

---

## Roadmap

- [x] Core agent loop
- [x] OpenRouter streaming with R1 think-block parser
- [x] Thread-safe knowledge graph
- [x] Bubbletea TUI
- [x] Wikipedia ingestion wired into live ingest phase
- [x] arxiv paper ingestion
- [x] Project Gutenberg ancient texts
- [x] Qdrant vector memory — batch upsert, retry, payload indexes, configurable semantic embeddings
- [x] Run-isolated memory (each run scoped by `run_id`, `--shared` to cross-search)
- [x] Think-block capture — R1 reasoning written to live report each cycle (filtered of meta-instructions)
- [x] Graph persistence (save/load JSON snapshots with `--save-graph` / `--resume-graph`)
- [ ] Final report export to markdown/HTML
- [ ] Anomaly scoring algorithm
- [ ] Multi-source contradiction detection

---

## The Hypothesis

> If you ingest enough human knowledge with no agenda,  
> follow contradictions instead of avoiding them,  
> and let an unbiased reasoner connect the dots —  
>  
> The emergent model looks nothing like the textbook.  
> But it looks exactly like what the outliers figured out  
> working alone, across centuries, in every culture.  
>  
> That's the report we're building.

---

*KAE v0.3 — Built in WSL2 | Go | OpenRouter | Qdrant v1.17.1 | Pure curiosity*
