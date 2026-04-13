# KAE — Claude Code Guide

## Repo structure

Two independent Go modules in one repo:

| Path | Module | Binary | Purpose |
|------|--------|--------|---------|
| `/` | `github.com/meistro57/kae` | `./kae` | KAE ingestion engine (chunking, embedding, Qdrant write) |
| `kae-lens/` | `github.com/meistro/kae` | `./kae-lens/lens` | Lens synthesis agent (reasoning, findings, corrections) |

## Build

```bash
# From kae-lens/ — builds both binaries
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
| `kae_chunks` | uint64 numeric | Knowledge base — KAE writes, Lens reads |
| `kae_lens_findings` | UUID | Lens output findings |

### MCP server

Configured in `~/.claude.json` pointing at `http://localhost:6333`. Restart Claude Code to activate.

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
