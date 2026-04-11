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

<img width="873" height="1563" alt="image" src="https://github.com/user-attachments/assets/764c012e-8af9-4c54-9e65-c2f47f951eaf" />

The hypothesis: if you feed it everything and let it run unbiased, it arrives at the same place the outliers, mystics, and fringe researchers already are. But this time with receipts.

---

## Requirements

- Go 1.22+
- At least one LLM provider API key (see table below)
- Docker (optional — for Qdrant vector memory via `setup.sh`)

```bash
# Install Go on WSL2/Ubuntu
sudo apt install golang-go

# Verify
go version
```

---

## Supported LLM Providers

KAE now supports five backends via a unified `provider:model` syntax:

| Provider | Prefix | Key env var |
|---|---|---|
| [OpenRouter](https://openrouter.ai) | `openrouter:` (default, bare names also work) | `OPENROUTER_API_KEY` |
| [Anthropic](https://console.anthropic.com) | `anthropic:` | `ANTHROPIC_API_KEY` |
| [OpenAI](https://platform.openai.com) | `openai:` | `OPENAI_API_KEY` |
| [Google Gemini](https://aistudio.google.com) | `gemini:` | `GEMINI_API_KEY` |
| [Ollama](https://ollama.ai) (local) | `ollama:` | `OLLAMA_URL` (optional, defaults to localhost:11434) |

---

## Setup

```bash
# Clone or copy the project
cd kae

# Run the setup script — installs Go deps, builds binary, starts Qdrant v1.17.1 via Docker
./setup.sh

# Copy the generated .env and fill in your keys
```

`.env` reference:

```env
# At least one provider key is required
OPENROUTER_API_KEY=your_key_here
ANTHROPIC_API_KEY=your_key_here
OPENAI_API_KEY=your_key_here
GEMINI_API_KEY=your_key_here

# Local Ollama — defaults to http://localhost:11434
OLLAMA_URL=http://localhost:11434

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

# Use any provider:model
go run . --model "anthropic:claude-opus-4-6"
go run . --model "openai:gpt-4o"
go run . --model "gemini:gemini-2.5-flash"
go run . --model "ollama:llama3.1"

# Ensemble mode — fan out to multiple providers, measure disagreement
go run . --ensemble --models "anthropic:claude-opus-4-6,openai:gpt-4o,gemini:gemini-2.5-flash"

# Auto-stop when the graph stagnates (no new nodes for N cycles)
go run . --novelty-threshold 0.05 --stagnation-window 5

# Auto-branch on high model controversy
go run . --ensemble --models "..." --branch-threshold 0.7 --max-branches 4

# Cross-run meta-analysis — find "convergent heresies" across past runs
go run . --analyze --min-runs 3

# Limit cycles
go run . --cycles 50

# Resume from previous graph snapshot
go run . --resume-graph graph_snapshot.json --cycles 25

# Save current graph snapshot on exit
go run . --save-graph graph_snapshot.json

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
│   │   └── config.go            # Config loader (env vars + .env) — all provider keys
│   ├── llm/
│   │   ├── provider.go          # Provider interface (Stream, ModelName) + Chunk/Message types
│   │   ├── factory.go           # NewProvider("provider:model", keys) — routes to backend
│   │   ├── client.go            # OpenRouter streaming client (satisfies Provider)
│   │   ├── anthropic.go         # Native Anthropic API — SSE streaming, adaptive thinking
│   │   ├── openai.go            # Native OpenAI API
│   │   ├── gemini.go            # Google Gemini API — SSE, thought parts
│   │   ├── ollama.go            # Local Ollama — NDJSON streaming
│   │   └── compat.go            # Shared OpenAI-compatible SSE helper
│   ├── ensemble/
│   │   └── ensemble.go          # Fan-out to N providers; controversy scoring; dissenter detection
│   ├── runcontrol/
│   │   └── controller.go        # Novelty decay tracking; auto-stop; branch triggering
│   ├── anomaly/
│   │   ├── cluster.go           # Cosine-similarity clustering of Qdrant anomaly nodes
│   │   └── reporter.go          # Markdown report generator for meta-analysis
│   ├── graph/
│   │   └── graph.go             # Thread-safe knowledge graph (nodes, edges, anomalies)
│   ├── embeddings/
│   │   └── embedder.go          # APIEmbedder (OpenAI-compat) or HashEmbedder fallback
│   ├── store/
│   │   ├── qdrant.go            # Qdrant REST client — upsert, search, collections
│   │   └── scroll.go            # Scroll API — FetchAnomalyNodes for meta-analysis
│   ├── agent/
│   │   └── engine.go            # Core agent loop — ensemble, run controller, provider routing
│   ├── ingestion/
│   │   ├── wiki.go              # Wikipedia ingestion
│   │   ├── arxiv.go             # arxiv paper ingestion
│   │   └── gutenberg.go         # Project Gutenberg — gutendex API + formats map
│   └── ui/
│       └── app.go               # Bubbletea TUI — 4-panel layout
```

---

## How The Agent Loop Works

```
Phase 0  SEED           Agent chooses its own entry concept (or uses --seed)
Phase 1  INGEST         Pulls sources on current topic (Wikipedia, arxiv, Gutenberg)
Phase 2  EMBED          Embeds chunks and stores them in Qdrant
Phase 3  SEARCH         Retrieves semantically similar passages from vector memory
Phase 4  THINK          Single model reasons visibly — you watch it think
         OR
Phase 4  ENSEMBLE       N models reason in parallel; controversy score computed
Phase 5  CONNECT        Extracts connections, adds nodes/edges to knowledge graph
Phase 6  SCORE          Contradiction scoring per topic
Phase 7  ANOMALY        Scans for where consensus goes silent or contradicts itself
Phase 8  REPORT         Updates the live markdown report
         └──────────────► Novelty check → LOOP or STOP
```

Runs until:
- Graph novelty drops below `--novelty-threshold` for `--stagnation-window` cycles
- `--cycles` limit reached
- You hit `q` or `ctrl+c` (graceful save)

---

## Models

KAE uses two model roles, each configurable with `provider:model` syntax:

| Role | Default | Purpose |
|---|---|---|
| **Brain** (`--model`) | `deepseek/deepseek-r1` | Deep reasoning, visible `<think>` blocks, connection-making |
| **Fast** (`--fast`) | `google/gemini-2.5-flash` | Bulk passes, seed selection |

Examples:

```bash
# OpenRouter (default — bare name works)
--model "deepseek/deepseek-r1"

# Anthropic native API with adaptive thinking
--model "anthropic:claude-opus-4-6"

# Local Ollama
--model "ollama:llama3.1"
```

In **ensemble mode** (`--ensemble`), the brain role is replaced by N providers running in parallel. Each provider independently reasons over the same context; a controversy score is computed from concept-overlap disagreement (Jaccard). Topics with controversy > `--branch-threshold` are flagged as anomalies and can auto-trigger focus branches.

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

### Tier 0 — Foundation (complete)
- [x] Core agent loop
- [x] OpenRouter streaming with R1 think-block parser
- [x] Thread-safe knowledge graph
- [x] Bubbletea TUI
- [x] Wikipedia, arxiv, Project Gutenberg ingestion
- [x] Qdrant vector memory with run isolation
- [x] Graph persistence (save/load JSON snapshots)
- [x] Markdown + HTML report export

### Tier 1 — Core Engine Enhancements (complete)
- [x] **Multi-provider support** — Anthropic, OpenAI, Gemini, Ollama, OpenRouter via unified `provider:model` syntax
- [x] **Multi-model ensemble reasoning** — parallel fan-out, controversy scoring, dissenter detection
- [x] **Novelty decay detection** — auto-stop when graph stagnates; configurable threshold + window
- [x] **Auto-branching** — high ensemble controversy triggers focus branch
- [x] **Anomaly clustering** — cosine-similarity grouping of anomaly nodes across runs
- [x] **Cross-run meta-analysis** (`--analyze`) — finds "convergent heresies" (anomalies that appear independently across multiple runs)
- [x] **Gutenberg URL fix** — uses gutendex formats map instead of hardcoded URL patterns

### Tier 2+ — Coming Next
- [ ] Persistent meta-graph across runs
- [ ] Active learning / adaptive ingestion
- [ ] Self-improvement feedback loop
- [ ] Extended visualization

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

*KAE v0.4 — Built in WSL2 | Go | OpenRouter · Anthropic · OpenAI · Gemini · Ollama | Qdrant v1.17.1 | Pure curiosity*
