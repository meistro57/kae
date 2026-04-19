# KAE — Claude Code Guide

## Repo structure

Four independent Go modules in one repo:

| Path | Module | Binary | Purpose |
|------|--------|--------|---------|
| `/` | `github.com/meistro57/kae` | `./kae` | KAE ingestion engine (chunking, embedding, Qdrant write) |
| `kae-lens/` | `github.com/meistro/kae` | `./kae-lens/lens` | Lens synthesis agent (reasoning, findings, corrections) |
| `mcp/` | standalone | `./mcp/kae_mcp` | MCP server exposing KAE/Qdrant tools to Claude |
| `kae-analyzer/` | standalone | `./kae-analyzer/kae-analyzer` | CLI for post-run inspection |
| `kae-forensics/` | standalone | `./kae-forensics/kae-forensics` | Data quality auditor — scans for weak vectors, repairs via re-embed |

## Build

```bash
# From kae-lens/ — builds all four binaries (kae, lens, kae_mcp, kae-analyzer)
make build

# Individual
go build -o kae .                          # root
cd kae-lens && go build -o lens ./cmd/lens # lens
```

## Run

```bash
# Lens — daemon with TUI (logs → lens.log)
cd kae-lens && ./lens

# Lens — daemon, plain stdout logs
./lens --no-tui

# Lens — drain queue once then exit
./lens --once

# Lens — clear all processed flags and reprocess everything
./lens --reprocess
```

Config lives at `kae-lens/config/lens.yaml`.

## Qdrant

- **HTTP (REST/MCP)**: `localhost:6333`
- **gRPC (Go client)**: `localhost:6334`
- Docker: `make qdrant-up` / `make qdrant-down` from `kae-lens/`

### Collections

| Collection | ID type | Purpose |
|---|---|---|
| `kae_chunks` | uint64 numeric | Knowledge base — KAE writes, Lens reads. Payload: `text`, `source`, `run_topic`, `semantic_domain`, `domain_confidence`, `run_id` |
| `kae_nodes` | uint64 numeric | Per-run concept nodes with weights and anomaly flags |
| `kae_meta_graph` | uint64 numeric | Persistent cross-run concept graph (Tier 2) |
| `kae_lens_findings` | UUID | Lens output findings |

### MCP servers

Two MCP servers are configured in `~/.claude.json`:
- **`qdrant`** — generic Qdrant find/store (via `mcp-server-qdrant` uvx package)
- **`kae`** — custom KAE tools: `qdrant_collections`, `qdrant_list_runs`, `qdrant_top_nodes`, `qdrant_search_chunks`, `qdrant_compare_runs`, `kae_start_run`, `kae_meta_attractors`, `kae_domain_analysis`

Restart Claude Code after changing MCP config.

## KAE CLI (Tier 2 flags)

```bash
# After a run, view attractor concepts (appeared in 3+ independent runs)
./kae --attractors --attractor-min-runs 3

# Domain bridge/moat analysis from meta-graph
./kae --domain-analysis

# Skip meta-graph update for this run
./kae --no-meta-graph --seed "test topic"
```

The meta-graph (`kae_meta_graph`) is updated automatically after every run unless `--no-meta-graph` is set.

## Key invariants

**Always use `qdrantclient.PointIDStr(id)` — never `id.GetUuid()` — when extracting IDs from Qdrant points.**  
`kae_chunks` uses uint64 numeric IDs; `GetUuid()` returns `""` for them, which breaks source citation in synthesis and correction prompts.

**Correction chunks** written back to `kae_chunks` carry `lens_correction: true` and `lens_processed: true`. They are permanently excluded from re-processing — `ClearProcessedFlags` skips them.

**Optimistic mark-processed**: Lens marks points `lens_processed=true` *before* reasoning starts to prevent duplicate batches. Double-processing is harmless; missed processing is not.

## Tests

```bash
cd kae-lens && go test ./internal/lens/... -v
```

## Lens pipeline (brief)

```
Watcher (polls kae_chunks) 
  → Reasoner.ProcessBatch
    → DensityCalculator.Assess   (adaptive search width + threshold)
    → QueryNeighbors             (Qdrant vector search)
    → Synthesizer.Synthesize     (LLM → findings JSON)
    → Synthesizer.Correct        (anomaly/contradiction only → correction text)
    → Writer.Write               (→ kae_lens_findings)
    → Writer.WriteCorrectionChunks (→ kae_chunks with lens_correction=true)
```
