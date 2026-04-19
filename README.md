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

## Ecosystem

```
KAE  (Knowledge Archaeology Engine)
 └── ingests broad human knowledge (Wikipedia, arXiv, Gutenberg, web)
 └── builds a knowledge graph — nodes, edges, anomalies
 └── embeds and deposits into Qdrant → kae_chunks (text) + kae_nodes (graph)

KAE LENS
 └── event-driven: reads kae_chunks, fires when new points appear
 └── adaptive density assessment → variable search width
 └── LLM reasoning (DeepSeek R1 / Gemini Flash via OpenRouter)
 └── writes findings back to Qdrant → kae_lens_findings
 └── live dashboard: TUI (Bubbletea) + Web (SSE, port 8080)

KAE ANALYZER
 └── CLI for post-run inspection: runs, anomalies, convergence, search, export

KAE FORENSICS
 └── scans any Qdrant collection for data quality anomalies
 └── detects missing payload fields and zero-magnitude (un-embedded) vectors
 └── repairs in place: re-embeds via OpenAI and upserts corrected vectors
 └── dry-run by default; --repair to apply fixes

KAE MCP SERVER
 └── exposes KAE + Qdrant to any MCP-compatible AI assistant
```

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

KAE supports five backends via a unified `provider:model` syntax:

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

# Optional — CORE open-access full-text papers (core.ac.uk/services/api)
# Without this key CORE is silently skipped; all other sources still run
CORE_API_KEY=your_core_key_here
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

# Auto-restart on stagnation — saves report then starts a fresh run automatically
go run . --auto-restart
go run . --auto-restart --seed "consciousness"   # keep the same seed each restart
go run . --auto-restart --headless               # headless + auto-restart (good for overnight runs)

# Auto-branch on high model controversy
go run . --ensemble --models "..." --branch-threshold 0.7 --max-branches 4

# Cross-run meta-analysis — find "convergent heresies" across past runs
go run . --analyze --min-runs 3

# Tier 2: show attractor concepts (emerged in 3+ independent runs)
go run . --attractors --attractor-min-runs 3

# Tier 2: domain bridge/moat analysis from persistent meta-graph
go run . --domain-analysis

# Tier 2: skip updating the meta-graph for this run
go run . --no-meta-graph --seed "quick test"

# Tier 2: citation crawl — automatically fires when a high-anomaly concept is detected
# Fetches suppressed lineages from Semantic Scholar and expands their citation chains
go run . --citation-threshold 0.6   # only crawl when anomaly score >= 0.6 (default 0.5)
go run . --no-cite-crawl            # disable citation crawl entirely

# Limit cycles
go run . --cycles 50

# Resume from previous graph snapshot
go run . --resume-graph graph_snapshot.json --cycles 25

# Save current graph snapshot on exit
go run . --save-graph graph_snapshot.json

# Search across all previous runs (default: isolated to current run)
go run . --shared

# Headless mode (no TUI — for scripts and MCP)
go run . --headless --seed "consciousness" --cycles 5

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

## KAE Lens

KAE Lens is an autonomous post-processing layer that fires when KAE deposits new knowledge into Qdrant. It reasons over the full topology of the ingested graph and surfaces connections, contradictions, clusters, and anomalies that KAE never explicitly made. For anomalies and contradictions, it runs a second focused LLM pass to produce a **data-grounded correction** from the actual source evidence.

```bash
cd kae-lens

# Start Qdrant (if not already running)
make qdrant-up

# Configure — Lens picks up your existing KAE .env keys automatically:
# OPENROUTER_API_KEY (required), OPENAI_API_KEY (optional, falls back to OpenRouter)

# Build and run
make build
make run-lens
# TUI in terminal + web dashboard at http://localhost:8080
```

### Finding Types

| Type | Correction pass | Meaning |
|---|---|---|
| `connection` | — | Unexpected cross-domain semantic link |
| `contradiction` | yes | Conflicting claims between knowledge nodes |
| `cluster` | — | Emergent concept group KAE never tagged |
| `anomaly` | yes | Outlier breaking mainstream consensus |

When a correction is produced it is stored on the finding and shown in the TUI trace panel (`enter` to expand).

Every finding also carries a **source URL map** (`point_id → URL`) built from the HTTP(S) source URLs of the chunks that were in scope. Source links appear in the TUI SOURCES block, as clickable links in the web dashboard, and URLs in HTML reports are auto-linkified.

### Adaptive Density

Lens adjusts its search width to local vector density so sparse regions get wide nets and dense regions get tight focused ones:

| Density | Nearby Points | Width | Threshold |
|---|---|---|---|
| very_sparse | 0 | 50 | 0.60 |
| sparse | 1–10 | 40 | 0.60 |
| medium | 11–50 | 20 | 0.65 |
| dense | 51–200 | 12 | 0.70 |
| very_dense | 200+ | 6 | 0.70 |

Lens findings are themselves embedded and stored in `kae_lens_findings` — a future pass can run Lens against its own findings, building third-order knowledge structures.

---

## KAE Analyzer

A standalone CLI for inspecting KAE runs stored in Qdrant.

```bash
cd kae-analyzer
go build -o kae-analyzer .

kae-analyzer runs                                    # List all runs
kae-analyzer analyze --run-id run_1775826869        # Analyze a specific run
kae-analyzer compare --runs run_123,run_456          # Compare runs for convergence
kae-analyzer anomalies --min-weight 4.0             # Find high-weight anomalies
kae-analyzer search --query "pseudo-psychology"      # Search concepts
kae-analyzer convergence --seed pseudopsychology     # Analyze convergence patterns
kae-analyzer stats                                   # Overall statistics
kae-analyzer export                                  # Export analysis to JSON
```

---

## KAE Forensics

A data quality tool for auditing and repairing Qdrant collections — catches points with missing payload fields or zero-magnitude vectors (never embedded) and fixes them in place.

```bash
cd kae-forensics
go build -o kae-forensics .

./kae-forensics           # dry-run: scan and report anomalies
./kae-forensics --repair  # re-embed and upsert corrected vectors
```

Checks performed:
- **Weak vector** — magnitude < 0.01 (un-embedded or corrupted); repaired by re-embedding the `document` payload via OpenAI `text-embedding-3-small`
- **Missing `source_material`** — payload field absent; flagged for review

Requires `OPENAI_API_KEY` when running with `--repair`. The collection name and gRPC address are configured at the top of `main.go`.

---

## KAE MCP Server

Exposes KAE and Qdrant to any MCP-compatible AI assistant (Claude, Cursor, etc.).

```bash
cd mcp
go build -o kae-mcp .
./kae-mcp
```

**Available tools:**

| Tool | Description |
|---|---|
| `qdrant_collections` | List all Qdrant collections with vector counts |
| `qdrant_list_runs` | List all KAE runs with node and anomaly counts |
| `qdrant_top_nodes` | Get highest-weight emergent concept nodes, optionally filtered by run |
| `qdrant_search_chunks` | Keyword search over ingested source passages |
| `qdrant_compare_runs` | Compare runs for independently converging concepts |
| `kae_start_run` | Start a new KAE run in headless mode, returns the report |
| `kae_meta_attractors` | Show attractor concepts from the persistent meta-graph (Tier 2) |
| `kae_domain_analysis` | Show domain bridges and moats from the meta-graph (Tier 2) |

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
├── kae-lens/                    # Autonomous post-processing intelligence layer
│   ├── cmd/lens/main.go         # Lens binary entry point
│   ├── config/lens.yaml         # Configuration
│   ├── internal/
│   │   ├── lens/
│   │   │   ├── watcher.go       # Polls Qdrant for unprocessed KAE points
│   │   │   ├── density.go       # Adaptive search width by local vector density
│   │   │   ├── reasoner.go      # Core agent loop
│   │   │   ├── synthesizer.go   # LLM reasoning → findings JSON
│   │   │   ├── writer.go        # Embeds and upserts findings to kae_lens_findings
│   │   │   ├── tui/             # Bubbletea terminal dashboard
│   │   │   └── web/             # HTTP + SSE web dashboard (port 8080)
│   │   ├── llm/                 # OpenRouter + OpenAI client
│   │   └── qdrantclient/        # Qdrant gRPC client helpers
│   └── collections/             # Qdrant payload schemas
├── kae-analyzer/                # Post-run analysis CLI
│   └── main.go                  # runs, analyze, compare, anomalies, search, convergence, export
├── kae-forensics/               # Data quality auditor and repair tool
│   └── main.go                  # Scans for weak vectors / missing fields; --repair re-embeds in place
└── mcp/                         # MCP server for AI assistant integration
    └── main.go                  # JSON-RPC over stdio — 8 tools
```

---

## How The Agent Loop Works

```
Phase 0  SEED           Agent chooses its own entry concept (or uses --seed)
Phase 1  INGEST         Pulls sources on current topic (Wikipedia, arXiv, Gutenberg,
                        Semantic Scholar, OpenAlex, CORE*, PubMed)  *requires CORE_API_KEY
Phase 2  EMBED          Embeds chunks and stores them in Qdrant
Phase 3  SEARCH         Retrieves semantically similar passages from vector memory
Phase 4  THINK          Single model reasons visibly — you watch it think
         OR
Phase 4  ENSEMBLE       N models reason in parallel; controversy score computed
Phase 5  CONNECT        Extracts connections, adds nodes/edges to knowledge graph
Phase 6  SCORE          Contradiction scoring per topic
Phase 7  ANOMALY        Scans for where consensus goes silent or contradicts itself
         └──────────────► If anomaly score ≥ threshold: background CITATION CRAWL
                          (Semantic Scholar suppressed lineages + citation chain BFS)
                          Results queued and picked up by next cycle's INGEST phase
Phase 8  REPORT         Updates the live markdown + HTML report
         └──────────────► Novelty check → LOOP or STOP
```

Runs until:
- Graph novelty drops below `--novelty-threshold` for `--stagnation-window` cycles → saves report and stops (or restarts if `--auto-restart` is set)
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

## Ingestion Sources

| Source | Key required | Strength |
|---|---|---|
| Wikipedia | none | Broad concept grounding |
| arXiv | none | Cutting-edge preprints (physics, AI, math) |
| Project Gutenberg | none | Ancient philosophy and primary texts |
| Semantic Scholar | none | Academic index with one-sentence tl;dr summaries |
| OpenAlex | none | Massive open index — tags works with broad scientific concepts |
| CORE | `CORE_API_KEY` | World's largest open-access aggregator — full abstract density |
| PubMed | none | Biomedical and neuroscience abstracts |

All sources run in parallel each cycle. CORE is silently skipped if the key is absent.

---

## Vector Memory (Qdrant)

| Setting | Detail |
|---------|--------|
| Version | `qdrant/qdrant:v1.17.1` (pinned) |
| Collections | `kae_chunks` (text chunks), `kae_nodes` (graph), `kae_meta_graph` (cross-run meta-graph), `kae_lens_findings` (Lens findings) |
| Distance | Cosine |
| Payload indexes | `domain`, `label` (keyword, created before HNSW builds) |
| `kae_chunks` payload | `text`, `source`, `run_topic` (exploration theme), `semantic_domain` (content classification), `domain_confidence` (0–1), `run_id` |
| Batch size | 64 points per upsert request |
| Retry | 3 attempts, 100ms/300ms backoff |
| `hnsw_ef` | `max(k×4, 64)` at query time |
| Embedding fallback | Feature hashing (128-dim, no API needed) |
| Embedding (configured) | Any OpenAI-compatible endpoint — default `text-embedding-3-small` (1536-dim) |
| Memory isolation | Each run searches only its own chunks by default — use `--shared` to search across all runs |
| Network access | Binds to `0.0.0.0:6333` — accessible on LAN by default |

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
- [x] **Auto-restart** (`--auto-restart`) — saves report and starts a fresh run on stagnation
- [x] **Auto-branching** — high ensemble controversy triggers focus branch
- [x] **Anomaly clustering** — cosine-similarity grouping of anomaly nodes across runs
- [x] **Cross-run meta-analysis** (`--analyze`) — finds "convergent heresies" (anomalies that appear independently across multiple runs)
- [x] **Headless mode** (`--headless`) — run without TUI for scripting and MCP integration
- [x] **Expanded ingestion** — Semantic Scholar, OpenAlex, CORE, PubMed alongside Wikipedia, arXiv, Gutenberg
- [x] **KAE Analyzer** — standalone CLI for post-run inspection (runs, anomalies, convergence, search, export)
- [x] **KAE MCP Server** — exposes KAE + Qdrant to any MCP-compatible AI assistant
- [x] **KAE Lens** — autonomous post-processing layer; adaptive density reasoning; TUI + web dashboard
- [x] **Lens anomaly correction** — data-grounded second LLM pass resolves anomaly/contradiction findings against source evidence
- [x] **Lens performance tuning** — per-call LLM timeout, relaxed density thresholds, paced batch polling
- [x] **Source paper links** — findings carry a `source_urls` map; TUI, web dashboard, and HTML reports all surface clickable links to the originating papers
- [x] **Domain contamination fix** — `kae_chunks` now stores `run_topic` (what the run was exploring) separately from `semantic_domain` (what the chunk is actually about); each embed batch is LLM-classified via `ClassifyDomainBatch`; migrate existing chunks with `go run ./scripts/migrate_domains [--dry-run]`

### Tier 2 — Knowledge Graph Intelligence (complete)
- [x] **Persistent meta-graph** (`kae_meta_graph`) — cross-run concept aggregation with attractor detection; update runs synchronously after each run and reports merged/new concept counts
- [x] **Citation chain excavation** — BFS over Semantic Scholar citation graph; suppressed lineage detection; wired into score phase — automatically fires on high-anomaly concepts and queues results for the next ingest cycle
- [x] **Domain boundary detection** — bridge concepts (cross-domain connectors) and moats (isolated domain pairs)

### Tier 3+ — Coming Next
- [ ] Active learning / adaptive ingestion
- [ ] Self-improvement feedback loop
- [ ] Lens Pass 2 — reason over findings to build third-order knowledge structures
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

*KAE v1.0 — Built in WSL2 | Go | OpenRouter · Anthropic · OpenAI · Gemini · Ollama | Qdrant v1.17.1 | Pure curiosity*
