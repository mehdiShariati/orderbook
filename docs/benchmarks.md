# Benchmarks

For **how to run sustained load** (CLI, rate limits, scaling clients), see [stress.md](stress.md). This page focuses on **what to compare** and how to report results fairly.

## Go matcher vs Rust

1. **Go (in-process):** don’t set `MATCHER_URL`; matching runs inside the API process.
2. **Rust:** run the `matcher` service (Docker or `cargo run` in `matcher/`), set `MATCHER_URL=http://localhost:9090` on the API.

Rust adds HTTP + JSON overhead, so wall-clock latency isn’t a fair comparison to Go unless you separate network time or profile the matcher binary alone.

## What to run

With `docker compose up`, use the repo’s [`cmd/stress`](../cmd/stress) driver or point `hey`, `vegeta`, or `k6` at `POST /orders` — see [stress.md](stress.md) for flags and rate-limit gotchas.

Then open Prometheus (`:9091` in the default compose) and watch:

- `match_latency_ms`
- `http_request_duration_seconds`
- `orders_received_total` / `trades_executed_total`

Grafana (`:3000`, admin/admin) has a starter dashboard if you wired provisioning.

## Reporting

If you’re writing this up: note machine, Go version, Rust version, concurrency, and paste or screenshot a Grafana panel. Raw numbers without context aren’t worth much.
