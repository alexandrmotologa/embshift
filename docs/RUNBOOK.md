# Production Migration Runbook

This guide covers operational procedures for executing vector database migrations using embshift.

## Prerequisites
1. Network access to legacy and target vector stores (e.g. Qdrant, pgvector, or Pinecone).
2. API credentials for embedding providers (OpenAI, Voyage AI, or self-hosted Ollama).
3. embshift binary or Docker container running within the ingestion path.

## Operational Lifecycle

### Step 1: Deploy Configuration
Create a configuration file (`embshift.yaml`) specifying legacy and target stores, dimension sizes, rate limits, and budget caps:

```yaml
proxy:
  port: 8080
  studio_port: 8787

legacy:
  type: "qdrant"
  endpoint: "http://qdrant-prod:6333"
  collection: "kb-ada-002"
  dimensions: 1536

target:
  type: "qdrant"
  endpoint: "http://qdrant-target:6333"
  collection: "kb-3-large"
  dimensions: 3072

backfill:
  batch_size: 100
  tokens_per_minute: 1000000
  max_budget_usd: 100.00
```

### Step 2: Route Client Traffic to embshift
Configure ingestion and search clients to point their vector database endpoints to the embshift proxy:
- URL: `http://<embshift-host>:8080`
- Writes commit to the legacy store synchronously while queuing for shadow replication.
- Searches route directly to the legacy store without latency penalties.

### Step 3: Launch Backfill
Open the Migration Studio at `http://<embshift-host>:8787` or issue a POST request:

```bash
curl -X POST http://localhost:8787/api/backfill/start
```

Monitor progress in the web console:
- Parity Ratio: tracks percentage of corpus migrated.
- Dual-Write Lag: verify replication queue drains continuously.
- Spend Meter: verify spend remains within budget limits.

### Step 4: Validate Shadow Evaluation
Check the Shadow Query View in the studio console.
- Verify Jaccard similarity meets minimum expectations across production queries.
- Inspect side-by-side results for semantic consistency.

### Step 5: Execute Atomic Cutover
When corpus parity reaches 100% and replication lag drops to zero:
1. Click [Execute Atomic Cutover] in the Migration Studio, or send:

```bash
curl -X POST http://localhost:8787/api/cutover
```

2. Live search and ingestion traffic now route to the target store.
3. embshift writes an audit manifest to `audit.json`.
4. Keep reverse replication active for 24 to 72 hours.

### Step 6: Rollback Procedure (Emergency Only)
If unexpected retrieval anomalies occur post-cutover:

```bash
curl -X POST http://localhost:8787/api/rollback
```

Because reverse dual-writing was active during the post-cutover window, the legacy collection contains all updates. Live traffic returns to the legacy collection immediately with zero data loss.
