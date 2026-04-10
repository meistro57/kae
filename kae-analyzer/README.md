# KAE Analyzer

**Go CLI tool for analyzing Knowledge Archaeology Engine data in Qdrant**

Directly integrates with your `kae-qdrant` MCP server to query real concept graphs, anomalies, and convergence patterns.

## Features

- 📊 **List runs** - View all KAE runs with stats (23+ runs available)
- 🔍 **Analyze runs** - Deep dive into concept distributions  
- 🔗 **Compare runs** - Find convergent/divergent concepts
- ⚡ **Find anomalies** - High-weight cross-domain insights
- 🔎 **Search concepts** - Find specific ideas across runs
- 📈 **Convergence analysis** - Track how seeds evolve
- 📊 **Global stats** - Aggregate analysis across all runs
- 📤 **Export** - JSON output for further processing

## Installation

```bash
cd ~/kae/kae-analyzer

# Install dependencies
go mod tidy

# Build
go build -o kae-analyzer

# Or install globally
go install
```

## Quick Start

```bash
# See all 23 runs in Qdrant
./kae-analyzer runs

# Analyze your first pseudopsychology run
./kae-analyzer analyze --run-id run_1775826869

# Compare the 3 divergence runs
./kae-analyzer compare --runs run_1775826869,run_1775829660,run_1775831260

# Find high-weight anomalies
./kae-analyzer anomalies --min-weight 4.0
```

## Usage

### List All Runs

```bash
./kae-analyzer runs
```

Shows table of all runs with:
- Node counts (concepts discovered)
- Anomaly counts (cross-domain insights)
- Max weight (strongest concept)
- Anomaly percentage

**Example Output:**
```
+----------------+-------+-----------+------------+-----------+
| Run ID         | Nodes | Anomalies | Max Weight | Anomaly % |
+----------------+-------+-----------+------------+-----------+
| run_1775826869 | 75    | 22        | 7.70       | 29.3%     |
| run_1775829660 | 96    | 23        | 7.28       | 24.0%     |
| run_1775831260 | 96    | 24        | 6.20       | 25.0%     |
+----------------+-------+-----------+------------+-----------+
Total runs: 23
```

### Analyze Specific Run

```bash
./kae-analyzer analyze --run-id run_1775826869 --top 50
```

Shows:
- 🔝 Top concepts by weight
- ⚡ Anomaly distribution
- 📊 Cycle progression (how concepts evolved)
- 🏷️ Domain classification (ingested vs anomaly)

**Example Output:**
```
=== Analysis: run_1775826869 ===

🔝 Top Concepts by Weight:
 1. Pseudo-psychology (7.70) ⚠️
 2. Potential Function (4.90) ⚠️
 3. Quantum synchronization (4.30)
 4. Gradient Flow (4.00)

⚡ Anomalies: 22 / 75 (29.3%)

Top Anomalies:
 1. Quantum dynamical computation (weight: 2.00)
 2. Markov blankets in dissipatively coupled quantum systems (weight: 2.00)
```

### Compare Multiple Runs

```bash
./kae-analyzer compare --runs run_1775826869,run_1775829660,run_1775831260
```

Finds:
- 🔗 Convergent concepts (appear in 2+ runs)
- 🌿 Unique concepts per run
- Overlap percentage

**Example Output:**
```
=== Run Comparison ===

Comparing 3 runs:
  - run_1775826869 (75 concepts)
  - run_1775829660 (96 concepts)
  - run_1775831260 (96 concepts)

🔗 Convergent Concepts: 8 / 267 (3.0%)

Top Convergent Concepts:
 1. Pseudo-psychology (in 3/3 runs)
 2. Gradient Flow (in 2/3 runs)

🌿 Unique Concepts per Run:
  run_1775826869: 67 unique
  run_1775829660: 88 unique
  run_1775831260: 88 unique
```

This confirms **99.7% divergence** - same seed leads to completely different conceptual territories!

### Find High-Weight Anomalies

```bash
./kae-analyzer anomalies --min-weight 4.0 --limit 10
```

Shows top anomalies across all runs above weight threshold.

**Use Cases:**
- Find strongest cross-domain bridges
- Identify confabulation candidates (audit high-weight edges)
- Discover recurring anomaly patterns

### Search for Concepts

```bash
./kae-analyzer search --query "pseudo-psychology"
./kae-analyzer search --query "quantum"
```

Finds all concepts matching the query (case-insensitive) with:
- Weight, cycle, run ID
- Anomaly flag

### Convergence Analysis

```bash
./kae-analyzer convergence --seed pseudopsychology
```

Analyzes how runs with the same seed converge/diverge:
- Convergence rate (% overlap)
- Concepts appearing in multiple runs
- Run-specific trajectories

**Perfect for your 6-run pseudopsychology experiment!**

### Global Statistics

```bash
./kae-analyzer stats
```

Shows aggregate stats:
- Total runs, concepts, anomalies
- Global max weight
- Average concepts per run

### Export to JSON

```bash
./kae-analyzer export --output kae_analysis.json
```

Exports full analysis data for:
- Custom visualization (feed to D3.js, Plotly)
- TensorBoard preparation
- Further statistical analysis

## Architecture

```
kae-analyzer (Go CLI)
       ↓
   exec.Command (spawns MCP server)
       ↓
   kae-qdrant MCP Server (Node.js)
       ↓
   Qdrant Vector DB
       ↓
   Real KAE run data (23+ runs, 1,636 concept nodes)
```

The Go app:
1. Spawns your MCP server via `npx`
2. Passes tool calls via environment variables
3. Parses markdown output with regex
4. Returns structured data

## MCP Inspector

You can also test your MCP server visually:

```bash
npx @modelcontextprotocol/inspector node /home/mark/.local/share/claude/mcp/kae-qdrant/build/index.js
```

Opens a web UI to:
- Call tools interactively
- See raw responses
- Debug tool parameters

Perfect for verifying the markdown format before parsing!

## Real Data Examples

Your current Qdrant database contains:

**Collections:**
- `kae_chunks`: 7,596 source passages
- `kae_nodes`: 1,636 concept nodes
- `marks_gpt_history`: 2,228 conversations
- `qmu_forum`: 315 forum posts

**Sample Runs:**
- `run_1775826869`: 75 nodes, 22 anomalies, max weight 7.70
- `run_1775829660`: 96 nodes, 23 anomalies, max weight 7.28
- `run_1775831260`: 96 nodes, 24 anomalies, max weight 6.20
- `run_1775844576`: 40 nodes, 8 anomalies, max weight **15.82** 🔥

## Advanced Usage

### Analyze Your 6 Pseudopsychology Runs

```bash
# Find all pseudopsychology run IDs
./kae-analyzer runs | grep pseudo

# Compare them
./kae-analyzer compare --runs run_XXX,run_YYY,run_ZZZ,run_AAA,run_BBB,run_CCC

# Convergence analysis
./kae-analyzer convergence --seed pseudopsychology
```

### Find Confabulation Candidates

```bash
# High-weight nodes might be confabulated
./kae-analyzer anomalies --min-weight 10.0

# Then verify sources:
# Use kae-qdrant:qdrant_search_chunks to audit passages
```

### Export for TensorBoard

```bash
# Export all data
./kae-analyzer export --output tb_data.json

# Use with future TensorBoard integration
# (will show 3D concept embedding clusters)
```

## Development

Current status: **Fully functional with real Qdrant integration** ✅

Future enhancements:
- [ ] Vector similarity search (find related concepts)
- [ ] TensorBoard projector export (3D embedding viz)
- [ ] Graph visualization output (Graphviz/DOT)
- [ ] Watch mode for live updates
- [ ] Markdown report generation
- [ ] Statistical correlation analysis

## Built for KAE by Meistro 🏍️

Part of the Knowledge Archaeology Engine visualization toolchain:
- **kae_live_parser.py** - Parse markdown reports → JSON
- **kae-genealogy-viewer.html** - Live timeline visualization
- **kae-analyzer** - CLI analysis tool (you are here)
- **kae-qdrant MCP server** - Vector database bridge
