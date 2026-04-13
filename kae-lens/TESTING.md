# KAE Lens Health Check & Testing Guide

## Quick Start

To verify KAE Lens is functioning properly, run these commands in order:

```bash
cd ~/kae/kae-lens

# 1. Run health check (checks all dependencies and connections)
make healthcheck

# 2. Run unit tests (validates core logic)
make test

# 3. Run component integration test (validates Qdrant + LLM connectivity)
make component-test
```

## What Each Test Does

### `make healthcheck`
**Duration:** ~10 seconds  
**Requirements:** Qdrant running on ports 6333 (REST) and 6334 (gRPC)

Checks:
- ✅ Go installation and version
- ✅ Qdrant REST API connectivity (port 6333)
- ✅ Qdrant gRPC connectivity (port 6334)
- ✅ Environment variables (OPENROUTER_API_KEY, OPENAI_API_KEY)
- ✅ Configuration files (config/lens.yaml, parent .env)
- ✅ Binary build status
- ✅ Go build compilation
- ✅ Unit test suite
- ✅ Qdrant collection schemas (kae_chunks, kae_lens_findings)
- ✅ Unprocessed point count
- ✅ Web dashboard port availability

**Exit codes:**
- `0` = All systems operational
- `1` = Critical issues found (see output)

### `make test`
**Duration:** <1 second  
**Requirements:** None (pure unit tests)

Validates:
- ✅ Density classification buckets (very_sparse → very_dense)
- ✅ Search width calculation
- ✅ Score threshold interpolation
- ✅ Medium profile threshold computation

These tests run WITHOUT any external dependencies.

### `make component-test`
**Duration:** ~5-10 seconds  
**Requirements:** Qdrant running + API keys configured

Real integration test that:
- ✅ Loads config/lens.yaml
- ✅ Connects to Qdrant (both collections)
- ✅ Tests density calculator with real config values
- ✅ Makes actual LLM API call (FastChat test)
- ✅ Makes actual embedding API call (if OpenAI key present)
- ✅ Scrolls unprocessed points from kae_chunks
- ✅ Displays collection stats and point previews

**Note:** This test makes REAL API calls and will consume tokens (minimal, ~100 tokens total)

## Common Issues & Fixes

### ❌ Qdrant not reachable
```bash
# Check if Qdrant is running
docker ps | grep qdrant

# If not running, start it
make qdrant-up

# Or manually:
docker run -d --name kae-qdrant \
  -p 6333:6333 -p 6334:6334 \
  -v $(pwd)/qdrant_storage:/qdrant/storage:z \
  qdrant/qdrant:v1.17.1
```

### ❌ API keys not found
```bash
# Check parent .env exists
ls -la ../.env

# If missing, create it
cd ..
echo "OPENROUTER_API_KEY=your_key_here" > .env
echo "OPENAI_API_KEY=your_key_here" >> .env
```

### ❌ Binary out of date
```bash
# Rebuild
make build

# Or rebuild just Lens
go build -o bin/lens ./cmd/lens
```

### ❌ No points in kae_chunks
This means KAE hasn't ingested any knowledge yet. This is expected if you haven't run KAE.

```bash
# In a separate terminal, run KAE
cd ~/kae
./kae --seed "consciousness" --cycles 3 --headless
```

Once KAE completes, Lens will automatically detect the new points and begin processing them.

## Expected Output

### Healthy System
```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
🔍 KAE LENS HEALTH CHECK
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📦 Checking Go installation...
✅ Go installed: go1.24.0

🗄️  Checking Qdrant connectivity...
✅ Qdrant REST API (port 6333) is reachable
   Collections found:
   - kae_chunks (8472 points)
   - kae_nodes (1243 points)
   - kae_lens_findings (156 points)
✅ Qdrant gRPC (port 6334) is listening

🔑 Checking environment configuration...
✅ Found .env in parent directory (KAE root)
✅ OPENROUTER_API_KEY is set
✅ OPENAI_API_KEY is set
✅ config/lens.yaml exists
   Knowledge collection: kae_chunks
   Findings collection: kae_lens_findings
   Poll interval: 30s
   Batch size: 10

🔨 Checking binary status...
✅ Binary is up to date

🏗️  Testing Go build...
✅ Go build successful

🧪 Running tests...
✅ All tests passed (5 test cases)

🗂️  Checking Qdrant collection schemas...
✅ kae_chunks collection exists
   Total points: 8472
   Unprocessed: 234
✅ kae_lens_findings collection exists
   Total findings: 156
   By type:
     connection: 78
     cluster: 42
     contradiction: 24
     anomaly: 12

🌐 Checking web dashboard...
⚠️  Web dashboard port 8080 is available (Lens not running)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
📊 HEALTH CHECK SUMMARY
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

✅ ALL CRITICAL SYSTEMS OPERATIONAL

Ready to run:
  make run-lens     # Start Lens with TUI + web dashboard
  make run-kae      # Start KAE ingestion (separate terminal)

Web dashboard will be at http://localhost:8080
```

## Debugging

### View Lens logs
```bash
# If running with TUI, logs are suppressed
# Run without TUI to see full logs:
make run-lens-notui
```

### View Qdrant dashboard
```bash
# Open in browser
open http://localhost:6333/dashboard

# Or check collections via curl
curl http://localhost:6333/collections | jq
```

### Manual Qdrant queries
```bash
# Count unprocessed points
curl -s http://localhost:6333/collections/kae_chunks/points/scroll \
  -H "Content-Type: application/json" \
  -d '{
    "limit": 1000,
    "filter": {
      "must_not": [{"key": "lens_processed", "match": {"value": true}}]
    },
    "with_payload": false
  }' | jq '.result.points | length'

# View findings by type
curl -s http://localhost:6333/collections/kae_lens_findings/points/scroll \
  -H "Content-Type: application/json" \
  -d '{"limit": 1000, "with_payload": true}' \
  | jq -r '.result.points[]?.payload.type.string_value' \
  | sort | uniq -c | sort -rn
```

## Success Criteria

KAE Lens is functioning properly when:

1. ✅ `make healthcheck` exits with code 0
2. ✅ `make test` shows all tests passing
3. ✅ `make component-test` successfully connects to Qdrant and LLM
4. ✅ Qdrant has both collections (kae_chunks and kae_lens_findings)
5. ✅ Running `make run-lens` shows TUI with live stats
6. ✅ Web dashboard is accessible at http://localhost:8080
7. ✅ New findings appear in kae_lens_findings as KAE ingests data

If all criteria are met, Lens is ready for production use. 🚀
