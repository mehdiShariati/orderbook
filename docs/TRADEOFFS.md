# Tradeoffs

Design choices here are deliberate. This doc is for reviewers who want **why**, not a feature list.

For **order-book fundamentals** and **vertical vs horizontal scaling** (with diagrams), see [SCALING.md](SCALING.md).

## Matching: Go in-process vs Rust HTTP

| Choice | Upside | Downside |
|--------|--------|----------|
| **Go default** (`MATCHER_URL` unset) | One process, one failure domain, no network hop, easy to debug | CPU-bound matching competes with HTTP/DB in the same binary |
| **Rust service** | Isolates matcher CPU, can scale/restart independently | Extra hop (JSON), two sources of truth if the API commits DB state without a successful matcher response — mitigated by **failing the HTTP request and rolling back the transaction** when `Submit` returns an error |

**Why both exist:** Demonstrates an `Engine` interface and a real HTTP client (`internal/matching/rustclient`) without forcing Rust for local dev.

## NATS: PRD-style async vs “hybrid”

A fully event-driven pipeline (persist → publish → matcher consumes NATS) is great for throughput and backpressure, but it adds operational surface: ordering guarantees, poison messages, replay, idempotent consumers.

This repo **matches synchronously** (Go function or HTTP to Rust), then **publishes** to NATS and writes audit rows **after** a successful match. That trades some theoretical throughput for **simpler reasoning**: the book state and DB updates stay aligned in one request path.

## Redis rate limit: fail-open

If Redis is unavailable, `INCR` errors fall through and the request is **not** blocked. That favors availability over strict abuse prevention — appropriate for a demo; production would often fail-closed or use a second line of defense.

## No authentication

`user_id` is client-supplied text. There is no JWT, API keys, or mTLS. Adding auth would be the first step before any “real” deployment; omitting it keeps the codebase focused on matching and IO.

## Postgres as source of orders/trades; matcher is ephemeral

The book lives in memory (Go or Rust). **Postgres** holds orders and trades for history and idempotency. On matcher restart, the in-memory book is empty until replay is implemented (out of scope here). **DB remains the audit trail**; GET by id is consistent even if the book is cold.

## Go 1.18 and `replace` in `go.mod`

The module pins older `nats.go` / `x/crypto` / `x/sys` via `replace` so builds work on the declared toolchain without pulling incompatible transitive upgrades. **Tradeoff:** you do not get the latest dependency tree “for free”; bumping Go or NATS needs a conscious pass.

## Observability: Prometheus first

Metrics are pull-based (`/metrics`). No in-app OTLP export is required to run the stack; OTel Collector is optional in Compose. That keeps the default path small while still allowing dashboards.

## WebSocket hub in-process

`/ws/market` broadcasts from the same API process. Horizontal scaling of **only** the API means WebSocket clients on instance A do not see instance B’s broadcasts unless you add a shared pub/sub layer (e.g. NATS or Redis) — not implemented; called out in [ORDERBOOK.md](ORDERBOOK.md).

---

When you disagree with a row above, that’s a good interview talking point: what you would change first for **your** SLOs.
