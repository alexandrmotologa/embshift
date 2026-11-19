# Architecture and Design Invariants

## System Overview
embshift is a vector database migration proxy and shadow re-embedding orchestrator. It sits between application services and vector storage engines, managing the transition between incompatible vector spaces without search interruption or dropped writes.

The engine executes in Go as a single binary with an embedded React 19 management interface. Memory consumption stays below 20MB of resident RAM during sustained operation.

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
|  |  * Phased Canary traffic splitting (0% -> 100%)       |  |
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
|  3. Dead-Letter Queue (DLQ) Inspector & Poison-Pill Retry   |
|  4. Prometheus Metrics Exporter (HTTP :8080/metrics)        |
|  5. Non-Repudiable Migration Audit Manifest (audit.json)    |
+-------------------------------------------------------------+
```

## Core Modules

### 1. Dual-Writing Router and Idempotency Buffer
The router intercepts `/v1/vectors/upsert` and `/v1/vectors/delete`.
When a write arrives:
1. The engine checks if embedding vectors exist. If the client submitted raw text without vectors, the primary embedder generates them.
2. The engine writes synchronously to the active primary store. If the write fails, the client receives an error immediately.
3. Upon primary commit success, the engine stores the record into a persistent SQLite queue in Write-Ahead Logging (WAL) mode.
4. A background goroutine drains the queue in batches, generates embeddings using the secondary embedding model, and commits the records to the secondary store.

### 2. Rate-Limited Backfill Orchestrator
The backfill process iterates through the legacy vector store in cursor-based batches:
- Reads document chunks from the legacy store.
- Evaluates document IDs against SQLite checkpoint records to avoid re-embedding previously completed items.
- Regulates token throughput with a token bucket algorithm enforcing requests per minute (RPM) and tokens per minute (TPM).
- Tracks dollar expenditures against configured budget limits.
- Upserts target records into the target collection.

### 3. Shadow Query Evaluator & Semantic Drift Scoring
The search router mirrors incoming queries to the target store asynchronously:
- Queries against the active primary return immediately to the caller.
- Mirrored queries execute in the background with the target embedding model.
- The engine calculates Jaccard similarity and NDCG (Normalized Discounted Cumulative Gain) at rank K between legacy and target results.
- Queries can be filtered in the web console by semantic drift severity (High Drift < 35%, Moderate, Strong Overlap).
- Metrics are recorded in SQLite and streamed to the React 19 studio via Server-Sent Events.

### 4. Phased Canary Traffic Splitting
Prior to committing full atomic cutover, platform engineers can enable Canary routing:
- A configurable percentage of read traffic (for example, 10% or 25%) routes directly to the target collection.
- Target results return directly to the caller with the `is_canary: true` attribute.
- If target queries encounter error rates above the configured threshold (default 5%), the engine executes an automated abort, resetting canary traffic to 0% and returning 100% of read traffic to the legacy store.

### 5. Dead-Letter Queue (DLQ) & Poison-Pill Isolation
If a document repeatedly fails embedding (due to context window overflow, invalid encoding, or provider rejection):
- After exceeding `max_retry_attempts` (default 3), the record is sequestered into the `dead_letter_events` table in SQLite.
- Background backfill and replication continue processing subsequent items without obstruction.
- Operators can inspect failed document IDs, read error stack traces, and manually trigger re-embedding or record purging via the Migration Studio.

### 6. Prometheus Observability Pipeline
The engine exposes standard Prometheus metrics at `GET /metrics`:
- Replication lag in milliseconds.
- Pending replication events in SQLite.
- Count of poison-pill documents in DLQ.
- Total backfilled document count.
- Cumulative tokens consumed and dollar spend.
- Rolling average Jaccard rank similarity.
- Active canary traffic weight ratio.

### 7. Atomic Cutover and Reversible Rollback
Cutover switches live traffic routing to the target collection:
- Preconditions require 100% corpus parity, an empty replication queue, and replication lag below the configured threshold.
- The pointer swap executes atomically in Go memory via a synchronization lock.
- Reverse dual-writing begins automatically. Writes to the target store replicate backward to the legacy store.
- If retrieval regressions occur post-cutover, an operator can trigger rollback with a single API call or button click.
- An immutable audit manifest (`audit.json`) containing migration statistics and a SHA-256 verification hash is written to disk.
