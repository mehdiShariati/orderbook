# Stress & scale testing

This is **not** `go test` (see [TESTING.md](TESTING.md)). Stress here means **sustained load** against a running API to observe throughput, latency tails, and resource limits.

## Prerequisites

1. **Running stack** — e.g. `docker compose up --build` (or Postgres + API only for a narrower test).
2. **Rate limit** — With the default Compose file, the API uses Redis and `RATE_LIMIT_PER_MIN=600` (**10 req/s** per IP). A stress run will mostly see **HTTP 429** unless you raise the limit.

**Option A — merge stress override (recommended for local runs):**

```bash
docker compose -f docker-compose.yml -f docker-compose.stress.yml up -d
```

This loads `docker-compose.stress.yml`, which sets `RATE_LIMIT_PER_MIN` to a high value for the `api` service.

**Option B — edit** `docker-compose.yml` **temporarily** under `api.environment`.

**Option C — no Redis** — If the API process has no `REDIS_URL`, rate limiting is disabled (middleware no-ops).

3. **Observability (optional)** — Prometheus on `:9091` and Grafana on `:3000` (default Compose) for `match_latency_ms`, `http_request_duration_seconds`, `db_query_latency_seconds`, and counters.

## Built-in stress CLI (`cmd/stress`)

A small Go driver that **POSTs** `/orders` with unique `user_id`s for the duration you choose.

```bash
go run -buildvcs=false ./cmd/stress -url http://localhost:8080 -c 64 -z 60s
```

### Flags

| Flag | Default | Meaning |
|------|---------|---------|
| `-url` | `http://localhost:8080` | API base URL (no trailing slash) |
| `-c` | `32` | Concurrent workers when `-rate=0` |
| `-z` | `30s` | How long to run |
| `-rate` | `0` | Target **requests/sec** (total). `0` = each worker sends as fast as possible |
| `-symbol` | `STRESS-USD` | Order symbol (all orders share it; book grows) |
| `-timeout` | `10s` | Per-request HTTP client timeout |

### Modes

- **Flood** (`-rate 0`): maximize throughput; useful for finding saturation and watching queue/CPU/DB.
- **Steady QPS** (`-rate 500`): hold a fixed aggregate rate; useful for repeatable latency percentiles and comparing configs.

### Example sessions

**Warm up Postgres + API, then flood for 2 minutes:**

```bash
docker compose -f docker-compose.yml -f docker-compose.stress.yml up -d
# wait for healthy
go run -buildvcs=false ./cmd/stress -url http://localhost:8080 -c 80 -z 2m
```

**Fixed 2000 req/s (if the stack can keep up):**

```bash
go run -buildvcs=false ./cmd/stress -url http://localhost:8080 -rate 2000 -c 100 -z 60s
```

The tool prints **2xx count**, **errors**, **429 count** (if any), **approximate RPS**, and **latency min / p50 / p95 / p99 / max** for completed HTTP responses (including non-2xx with a body). Transport failures are counted separately and are **not** in the latency histogram.

### Interpreting output

- Many **429** → rate limit; use the Compose override or raise `RATE_LIMIT_PER_MIN`.
- **High p99** with flat p50 → tail latency (DB, GC, lock contention); check Prometheus and DB metrics.
- **Errors** without 429 → validation, 502 matcher failures, or upstream timeouts (`-timeout`).

## Scaling load beyond one machine

`cmd/stress` is single-process. To push **aggregate** QPS from multiple clients:

- Run the same command on several hosts against the **same** API URL, or
- Use external generators ([`hey`](https://github.com/rakyll/hey), [Vegeta](https://github.com/tsenart/vegeta), k6) with a **POST** body like:

```json
{"user_id":"load-1","symbol":"STRESS-USD","side":"buy","type":"limit","price":"100","quantity":"0.001"}
```

Vary `user_id` or use a counter so rows are distinct. **Each client IP** may hit the Redis rate limit independently unless you whitelist or raise limits.

## What to watch in Prometheus / Grafana

| Metric | Notes |
|--------|--------|
| `http_request_duration_seconds` | End-to-end API latency by route |
| `match_latency_ms` | Matcher path only (`result` label: ok / error) |
| `db_query_latency_seconds` | Postgres pressure |
| `orders_received_total` / `trades_executed_total` | Throughput sanity |

## Relating to [benchmarks.md](benchmarks.md)

- **benchmarks.md** — how to compare **Go vs Rust** matcher and report numbers fairly.
- **This doc** — how to **drive load** safely (rate limits, flags, multi-client) and read stress output.

## Production-style scale (not implemented here)

Horizontal scaling of **stateless API** replicas is straightforward; **matching** state and **WebSocket** fan-out need shared infrastructure (partitioning, NATS/Redis pub/sub) — see [TRADEOFFS.md](TRADEOFFS.md) and [ORDERBOOK.md](ORDERBOOK.md).
