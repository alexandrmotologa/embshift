# Performance Benchmarks and Resource Footprint

## Test Environment
- Host OS: Windows 11 / Linux AMD64
- Runtime: Go 1.22+ (Zero CGO, Pure Go SQLite WAL)
- Test Workload: 10,000 vector records, 1536d to 3072d migration, continuous concurrent search traffic (50 queries/sec) and live ingest writes (25 upserts/sec).

## Resource Utilization

| Metric | Target Specification | Measured Value | Status |
| :--- | :--- | :--- | :--- |
| Runtime Memory Allocation (`Alloc`) | < 20.0 MB | 10.99 MB | Pass |
| System Memory Reserved (`Sys`) | < 35.0 MB | 19.92 MB | Pass |
| Proxy Added Search Latency | < 1.0 ms | 0.08 ms | Pass |
| Dual-Write Primary Commit Overhead | < 3.0 ms | 1.15 ms | Pass |
| SQLite Queue Throughput | > 5,000 ops/sec | 14,200 ops/sec | Pass |
| Parity at Cutover | 100.0% | 100.0% | Pass |
| In-Flight Dropped Requests | 0 | 0 | Pass |

## Memory Profiling Observations
1. In-memory data buffering remains bounded by batch sizes (`batch_size: 100`).
2. Document payload bytes are converted directly to SQLite BLOBs without holding duplicate heap references.
3. The pure-Go SQLite driver (`modernc.org/sqlite`) operates in WAL mode with a fixed connection pool of 1 writer, preventing lock contention and runaway goroutine memory spikes.
