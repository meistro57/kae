# kae-mcp — KAE / Qdrant MCP Server

MCP server exposing KAE archaeology data and full Qdrant CRUD to Claude.  
Pure Go stdlib, no external dependencies.

## Build & run

```bash
cd ~/kae/mcp
go build -o kae-mcp .
./kae-mcp          # reads from stdin, writes to stdout (MCP JSON-RPC)
```

Override the Qdrant address:

```bash
QDRANT_URL=http://my-host:6333 ./kae-mcp
```

## Tools reference

### KAE-specific (8 tools)

| Tool | Description |
|------|-------------|
| `qdrant_collections` | List all collections with point counts |
| `qdrant_list_runs` | List KAE runs with node/anomaly counts |
| `qdrant_top_nodes` | Top concepts by weight (optional `run_id` filter) |
| `qdrant_compare_runs` | Cross-run convergence — shared concepts, overlap % |
| `kae_meta_attractors` | Attractor concepts from the persistent meta-graph |
| `kae_domain_analysis` | Cross-domain bridge and moat analysis |
| `kae_start_run` | Launch a headless KAE archaeology run |
| `qdrant_search_chunks` | Keyword search over ingested source passages |

### Collection management (3 tools)

| Tool | Key args | Description |
|------|----------|-------------|
| `qdrant_collection_info` | `collection_name` | Vector size, distance, point count, status |
| `qdrant_create_collection` | `collection_name`, `vector_size`, `distance?`, `on_disk?` | Create a new collection |
| `qdrant_delete_collection` | `collection_name` | **Permanently** delete a collection |

### Point operations (4 tools)

| Tool | Key args | Description |
|------|----------|-------------|
| `qdrant_scroll_points` | `collection_name`, `limit?`, `offset?`, `filter?`, `with_payload?` | Browse with pagination; returns `next_offset` |
| `qdrant_get_point` | `collection_name`, `point_id` | Single point by ID (numeric or UUID) |
| `qdrant_get_points` | `collection_name`, `point_ids[]` | Batch point retrieval |
| `qdrant_count_points` | `collection_name`, `filter?`, `exact?` | Count with optional filter |

### Search (2 tools)

| Tool | Key args | Description |
|------|----------|-------------|
| `qdrant_search` | `collection_name`, `query_vector[]`, `limit?`, `score_threshold?`, `filter?` | Vector similarity search (supply a pre-computed vector) |
| `qdrant_recommend` | `collection_name`, `positive_ids[]`, `negative_ids?`, `limit?`, `filter?` | Find similar to example points |

### Payload management (3 tools)

| Tool | Key args | Description |
|------|----------|-------------|
| `qdrant_set_payload` | `collection_name`, `point_ids[]`, `payload{}` | Merge payload keys into points |
| `qdrant_delete_payload` | `collection_name`, `point_ids[]`, `keys[]` | Remove specific payload keys |
| `qdrant_clear_payload` | `collection_name`, `point_ids[]` | Wipe all payload (vectors preserved) |

### Advanced querying (1 tool)

| Tool | Key args | Description |
|------|----------|-------------|
| `qdrant_query_points` | `collection_name`, `filter?`, `limit?`, `offset?`, `order_by?` | Filter DSL query with pagination and optional client-side sort |

## Qdrant filter DSL

```json
// Match exact value
{"must": [{"key": "run_id", "match": {"value": "run_12345"}}]}

// Range
{"must": [{"key": "weight", "range": {"gte": 3.0}}]}

// Text (substring)
{"must": [{"key": "label", "match": {"text": "consciousness"}}]}

// Combined
{
  "must": [
    {"key": "run_id", "match": {"value": "run_12345"}},
    {"key": "weight", "range": {"gte": 2.0}}
  ],
  "must_not": [
    {"key": "anomaly", "match": {"value": true}}
  ]
}
```

## Pagination

`qdrant_scroll_points` and `qdrant_query_points` return a `next_offset` token when more
results exist. Pass it as `offset` in the next call:

```
Call 1: scroll_points(collection="kae_chunks", limit=10)
        → "Next page offset: `122771824465783980`"

Call 2: scroll_points(collection="kae_chunks", limit=10, offset="122771824465783980")
        → next batch...
```

## Collections

| Collection | ID type | Content |
|---|---|---|
| `kae_chunks` | uint64 | Ingested source passages |
| `kae_nodes` | uint64 | Per-run concept nodes with weight/anomaly |
| `kae_meta_graph` | uint64 | Persistent cross-run concept graph |
| `kae_lens_findings` | UUID | Lens synthesis findings |
