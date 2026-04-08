# 🧠 Knowledge Archaeology Engine (KAE)

> *An autonomous agent that ingests human knowledge, follows contradictions without flinching, and builds a model of what the data actually points to — not what consensus says.*

---

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
- Qdrant running locally (optional — for vector memory)

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

# Set your OpenRouter key
export OPENROUTER_API_KEY=your_key_here

# Optional: point at your Qdrant instance
export QDRANT_URL=http://localhost:6333

# Download dependencies
go mod tidy

# Run
go run .
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

# Use a different thinking model
go run . --model "anthropic/claude-opus-4"

# Use a different fast/bulk model
go run . --fast "google/gemini-flash-1.5"

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
├── internal/
│   ├── config/
│   │   └── config.go            # Config loader (env vars)
│   ├── llm/
│   │   └── client.go            # OpenRouter streaming client + R1 think-block parser
│   ├── graph/
│   │   └── graph.go             # Thread-safe knowledge graph (nodes, edges, anomalies)
│   ├── agent/
│   │   └── engine.go            # Core agent loop (seed→ingest→think→connect→report)
│   ├── ingestion/
│   │   └── wiki.go              # Wikipedia ingestion + text chunking
│   └── ui/
│       └── app.go               # Bubbletea TUI — 3-panel cinematic layout
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

## Roadmap

- [x] Core agent loop
- [x] OpenRouter streaming with R1 think-block parser
- [x] Thread-safe knowledge graph
- [x] Bubbletea TUI
- [x] Wikipedia ingestion
- [ ] Wire Wikipedia into live ingest phase
- [ ] arxiv paper ingestion
- [ ] Project Gutenberg ancient texts
- [ ] Qdrant vector memory (semantic search across sessions)
- [ ] Graph persistence (resume runs)
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

*KAE v0.1 — Built in WSL2 | Go | OpenRouter | Qdrant | Pure curiosity*
