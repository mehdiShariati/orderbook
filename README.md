# Orderbook

**Simulated exchange backend:** limit and market orders, price–time matching, REST + WebSocket market data, Postgres persistence, optional event streaming. Matching runs **in Go** by default or behind **`MATCHER_URL`** as a **Rust** HTTP service—same API, swappable engine.

This repo is a **portfolio-quality systems project**: real persistence, observability, and documentation—not a production exchange (no custody, no regulatory stack).

---

## If you’re reviewing this in a hurry

| Priority | Where to look |
|----------|----------------|
| **Architecture & API** | [docs/ORDERBOOK.md](docs/ORDERBOOK.md) — diagrams, endpoints, data model |
| **Order book + vertical / horizontal scale** | [docs/SCALING.md](docs/SCALING.md) — what a book is, Mermaid diagrams, how this repo maps |
| **Why things are built this way** | [docs/TRADEOFFS.md](docs/TRADEOFFS.md) — Go vs Rust matcher, NATS, Redis, scaling limits |
| **Correctness** | `go test ./...` · [docs/TESTING.md](docs/TESTING.md) — unit + optional Postgres integration tests |
| **Load / stress** | [cmd/stress](cmd/stress) · [docs/stress.md](docs/stress.md) — CLI, rate limits, Prometheus |
| **Requirements trace** | [docs/PRD.md](docs/PRD.md) |

---

## Stack

| Layer | Pieces |
|--------|--------|
| **API** | Go (chi), idempotent `POST /orders`, idempotency key, health checks |
| **Matching** | Go in-process **or** Rust matcher over HTTP (`internal/matching`, `matcher/`) |
| **Data** | PostgreSQL (orders, trades, audit `events`) |
| **Cross-cutting** | Redis (rate limit), NATS (domain events), Prometheus + Grafana (`/metrics`) |
| **Real-time** | WebSocket `/ws/market` |

---

## What’s implemented

- REST: create / get / cancel orders, order book depth, trades by symbol.
- Matching: partial fills, limit + market orders, in-memory book with **price–time** priority.
- **Transactional** create path: match failure on the remote engine rolls back the DB write (see tests).
- **Stress driver** (`cmd/stress`) for sustained `POST /orders` with latency percentiles.
- **Docs:** architecture, tradeoffs, **[scaling & order-book narrative](docs/SCALING.md)**, testing strategy, stress methodology, benchmarks notes.

---

## Architecture

```mermaid
flowchart LR
    C[clients] --> API[Go API]
    API --> PG[(Postgres)]
    API --> RD[(Redis)]
    API --> NT[NATS]
    API -. optional .-> M[Rust matcher]
    PR[Prometheus] -->|scrape| API
    GF[Grafana] --> PR
```

```mermaid
flowchart TB
    api[api] --> pg[(postgres)]
    api --> rd[(redis)]
    api --> n[nats]
    api --> m[matcher]
    prom[prometheus] --> api
    g[grafana] --> prom
```

More detail (sequences, ER, operational notes): [docs/ORDERBOOK.md](docs/ORDERBOOK.md).

---

## Run

**Full stack (Compose):**

```bash
docker compose up --build
```

**Stress / load testing** — default `RATE_LIMIT_PER_MIN` is low; use the override or you will see HTTP 429:

```bash
docker compose -f docker-compose.yml -f docker-compose.stress.yml up --build -d
```

| Service | URL (default) |
|---------|-----------------|
| API | http://localhost:8080 |
| Metrics | http://localhost:8080/metrics |
| Prometheus | http://localhost:9091 |
| Grafana | http://localhost:3000 (`admin` / `admin`) |
| Rust matcher (when enabled) | http://localhost:9090 |

**API only (local Postgres):**

```bash
go run -buildvcs=false ./cmd/api
```

Omit `MATCHER_URL` to use the **Go** matcher. `-buildvcs=false` avoids VCS errors when the tree isn’t a git checkout.

---

## Environment (API)

| Variable | Role |
|----------|------|
| `DATABASE_URL` | Postgres DSN |
| `MATCHER_URL` | Rust matcher base URL; **unset** = Go matcher in-process |
| `NATS_URL` | e.g. `nats://nats:4222` |
| `REDIS_URL` | Per-IP rate limiting |
| `HTTP_ADDR` | Listen address (default `:8080`) |
| `RATE_LIMIT_PER_MIN` | Requests per IP per minute (see [stress.md](docs/stress.md) before load tests) |

---

## HTTP surface

`POST /orders` · `GET` / `DELETE /orders/{id}` · `GET /book/{symbol}` · `GET /trades/{symbol}` · `GET /ws/market?symbol=…` · `/health/live` · `/health/ready`

Validation and response shapes: [docs/ORDERBOOK.md](docs/ORDERBOOK.md).

---

## Tests

```bash
go test ./... -count=1
```

Integration tests (Postgres + HTTP + matcher failure rollback) run when **`TEST_DATABASE_URL`** is set — [docs/TESTING.md](docs/TESTING.md).

---

## Stress & benchmarks

```bash
go run -buildvcs=false ./cmd/stress -url http://localhost:8080 -c 64 -z 60s
```

Steady throughput example:

```bash
go run -buildvcs=false ./cmd/stress -url http://localhost:8080 -rate 1500 -c 120 -z 2m
```

| Topic | Doc |
|-------|-----|
| Flags, rate limits, Prometheus, multi-client load | [docs/stress.md](docs/stress.md) |
| Comparing Go vs Rust matcher fairly | [docs/benchmarks.md](docs/benchmarks.md) |

---

## Documentation index

| Doc | Contents |
|-----|----------|
| [ORDERBOOK.md](docs/ORDERBOOK.md) | Architecture, API, diagrams |
| [SCALING.md](docs/SCALING.md) | Order book primer, vertical vs horizontal scaling (Mermaid), growth path |
| [TRADEOFFS.md](docs/TRADEOFFS.md) | Design decisions and downsides |
| [TESTING.md](docs/TESTING.md) | Unit/integration tests (`go test`) |
| [stress.md](docs/stress.md) | Stress CLI and scale testing |
| [benchmarks.md](docs/benchmarks.md) | Matcher comparison methodology |
| [PRD.md](docs/PRD.md) | Original requirements |

---

## License

MIT — see [LICENSE](LICENSE).
