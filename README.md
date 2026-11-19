# embshift

Zero-downtime vector database migration proxy, dual-write engine, and shadow re-embedding orchestrator with an embedded React 19 cockpit.

```
       +-------------------------------------------------------------+
       |               Client Application / RAG Ingestion            |
       |       (Configured with Vector Endpoint: http://embshift)    |
       +-------------------------------------------------------------+
                                      |
                      Write / Query / Delete Operations
                                      v
       +-------------------------------------------------------------+
       |                     embshift Core Engine                    |
       |                    (High-Performance Go CLI)                |
       |                                                             |
       |  +---------------------+        +------------------------+  |
       |  | Dual-Write Router   |------->| Asynchronous Shadow    |  |
       |  | & Idempotency Gate  |        | Event Queue (SQLite)   |  |
       |  +---------------------+        +------------------------+  |
       |              |                               |              |
       |       Sync Write / Query                     v              |
       |              |                  +------------------------+  |
       |              |                  | Controlled Rate-Limited|  |
       |              |                  | Backfill Orchestrator  |  |
       |              |                  +------------------------+  |
       |              v                               |              |
       |       +---------------+                      v              |
       |       | Legacy Store  |               +---------------+     |
       |       | (Ada-002)     |               | Target Store  |     |
       |       | 1536 Dims     |               | (Voyage/3072d)|     |
       |       +---------------+               +---------------+     |
       |              |                               |              |
       |              +---------------+---------------+              |
       |                              |                              |
       |                              v                              |
       |  +-------------------------------------------------------+  |
       |  |          Shadow Query Comparator & Quality Gate       |  |
       |  |  * Mirrors configurable fraction of live queries      |  |
       |  |  * Computes rank correlation & retrieval parity       |  |
       |  +-------------------------------------------------------+  |
       |                              |                              |
       |         Parity = 100% && Quality Gate Passed                |
       |                              v                              |
       |  +-------------------------------------------------------+  |
       |  |              Atomic Traffic Cutover Switch            |  |
       |  |  Target becomes Primary; Legacy safely buffered       |  |
       |  +-------------------------------------------------------+  |
       |                              |                              |
       +------------------------------|------------------------------+
                                      |
                                      v
       +-------------------------------------------------------------+
       |                  Control and Inspection Surfaces            |
       |  1. Embedded React 19 Migration Studio (HTTP :8787)         |
       |  2. Real-Time Backfill Progress & Dual-Write Lag Gauge      |
       |  3. Non-Repudiable Migration Audit Manifest (audit.json)    |
       +-------------------------------------------------------------+
```

## Problem Statement

When upgrading foundation embedding models in production RAG systems (for example, migrating from 1536-dimensional OpenAI Ada-002 to 3072-dimensional text-embedding-3-large or Voyage-3), engineering teams face fundamental obstacles:
1. Mathematical incompatibility. Vectors from different models cannot be queried in the same vector space. Cosine similarity between an Ada-002 query and a text-embedding-3 vector is undefined.
2. Search downtime. Shutting down the legacy index during backfill stops semantic search and retrieval workflows.
3. In-flight drift. New writes and deletes occurring during hours of batch re-embedding cause state divergence between indices.
4. Unverified retrieval quality. Teams lack empirical proof that the target index maintains or improves retrieval accuracy on live production query distributions before cutover.

embshift resolves these issues by acting as a transparent proxy that coordinates dual-writing, manages rate-limited backfills with token budget caps, runs live shadow query evaluations, and executes atomic cutovers with instant rollback support.

## Core Capabilities

- Transparent Dual-Writing Proxy: Commits writes and deletes synchronously to the active primary store while asynchronously staging updates to the target store via an embedded SQLite WAL queue.
- Rate-Limited Backfill Orchestrator: Traverses legacy collections with cursor pagination, pacing embedding API requests through a token bucket rate limiter with dollar budget caps.
- Production Shadow Query Evaluation: Mirrors a configurable fraction of live search traffic to the target store, computing Jaccard similarity and NDCG without adding latency to client responses.
- Atomic Zero-Downtime Cutover: Executes an in-memory pointer swap to reroute production traffic once corpus parity reaches 100% and quality gates pass.
- Reverse Dual-Writing Rollback: Automatically replicates updates backward from target to legacy post-cutover, allowing an immediate one-click rollback if anomalies arise.
- Embedded React 19 Cockpit: Delivers a real-time web interface over Server-Sent Events (SSE) inside a single Go binary consuming under 20MB of RAM.

## Mathematical Formulation

### 1. Document Parity Ratio (rho_parity)
Let D_legacy be the set of document IDs in the legacy store and D_target the set of re-embedded documents in the target store:

$$\rho_{\text{parity}} = \frac{|\mathcal{D}_{\text{legacy}} \cap \mathcal{D}_{\text{target}}|}{|\mathcal{D}_{\text{legacy}}|}$$

A migration is eligible for cutover only when rho_parity = 1.0 and replication lag is zero (Delta t_lag = 0).

### 2. Shadow Rank Overlap (Jaccard@K)
For mirrored production query q, let L_legacy(q) and L_target(q) represent the top-K retrieved document IDs:

$$\text{Jaccard@K}(q) = \frac{|L_{\text{legacy}}(q) \cap L_{\text{target}}(q)|}{|L_{\text{legacy}}(q) \cup L_{\text{target}}(q)|}$$

Rolling average rank correlation verifies retrieval parity across live user queries.

## Quickstart

### 1. Run the Self-Contained Demo Simulation
Clone the repository and run the built-in demo. This mode starts an in-memory legacy store with 100 documents, runs live traffic, and opens the Migration Studio without requiring external database dependencies:

```bash
git clone https://github.com/alexandrmotologa/embshift.git
cd embshift
go run ./cmd/embshift demo
```

Open your browser to `http://localhost:8787` to monitor backfill progress, replication lag, and execute the atomic cutover.

### 2. Run the Verification Test Suite
Execute unit tests across the queue, proxy, backfill orchestrator, shadow evaluator, and REST server:

```bash
go test -v ./...
```

Run the end-to-end simulation script verifying zero dropped requests under concurrent write load:

```bash
go run ./examples/simulation.go
```

### 3. Build Single Binary
Compile the embedded React 19 frontend and Go binary:

```bash
# Build frontend and binary
make build

# Start production proxy with custom configuration
./embshift.exe start --config examples/embshift.yaml
```

## Configuration Reference

Configuration files use YAML syntax (`embshift.yaml`):

```yaml
proxy:
  host: "0.0.0.0"
  port: 8080
  studio_port: 8787

legacy:
  type: "qdrant" # "qdrant", "pgvector", "pinecone", "mock"
  endpoint: "http://localhost:6333"
  api_key: ""
  collection: "production-v1"
  dimensions: 1536
  model: "text-embedding-ada-002"

target:
  type: "qdrant"
  endpoint: "http://localhost:6334"
  api_key: ""
  collection: "production-v2"
  dimensions: 3072
  model: "text-embedding-3-large"

embedder:
  legacy:
    type: "openai"
    endpoint: "https://api.openai.com/v1"
    api_key: "sk-..."
    model: "text-embedding-ada-002"
    dimensions: 1536
  target:
    type: "openai"
    endpoint: "https://api.openai.com/v1"
    api_key: "sk-..."
    model: "text-embedding-3-large"
    dimensions: 3072
    price_per_million_tokens_usd: 0.13

backfill:
  batch_size: 100
  concurrency: 4
  requests_per_minute: 3000
  tokens_per_minute: 1000000
  max_budget_usd: 50.00
  text_field: "text"

shadow:
  sample_ratio: 0.25 # Mirror 25% of live production search requests
  top_k: 10
  min_jaccard_threshold: 0.40

cutover:
  require_parity_ratio: 1.0
  max_replication_lag_ms: 100.0
  require_zero_pending_queue: true
  enable_reverse_replication: true

storage:
  sqlite_path: "embshift.db"
```

## API Specification

### Data Plane (Vector Ingestion & Search)
- `POST /v1/vectors/upsert`: Ingest vectors or raw text records. Synchronously commits to primary and enqueues to secondary.
- `POST /v1/vectors/query`: Execute vector or text search. Returns primary results immediately and asynchronously mirrors queries for shadow quality scoring.
- `POST /v1/vectors/delete`: Delete vectors by ID across collections.

### Management Plane
- `GET /api/status`: Retrieve real-time telemetry (parity, lag, spend, state).
- `POST /api/backfill/start`: Initiate background backfill.
- `POST /api/backfill/pause`: Pause background backfill.
- `POST /api/backfill/resume`: Resume background backfill.
- `POST /api/cutover`: Execute atomic traffic switchover and generate audit manifest.
- `POST /api/rollback`: Revert primary routing to legacy collection.
- `GET /api/audit`: Retrieve cryptographic audit manifest.
- `GET /api/events`: Server-Sent Events (SSE) telemetry stream.

## Documentation

- [Architecture Invariants](file:///B:/workgit/embshift/docs/ARCHITECTURE.md)
- [Production Runbook](file:///B:/workgit/embshift/docs/RUNBOOK.md)
- [Performance Benchmarks](file:///B:/workgit/embshift/docs/BENCHMARKS.md)

## License

MIT License.
