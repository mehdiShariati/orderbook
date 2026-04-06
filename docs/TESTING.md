# Testing

## What runs by default

```bash
go test ./...
```

This runs **fast, hermetic** tests: matching engine logic, store helpers, Rust client JSON parsing, etc. No database or Docker is required.

Tests that need Postgres are **skipped** unless `TEST_DATABASE_URL` is set (see below).

## Layout

| Package / file | What it covers |
|----------------|----------------|
| `internal/matching` | Order book: crosses, partial fills, market vs limit, cancel, FIFO at same price, snapshots |
| `internal/matching/rustclient` | HTTP round-trip to a matcher: request/response JSON, error paths |
| `internal/store` | Small helpers (e.g. unique-violation detection) without DB |
| `internal/api` | Integration: real HTTP handler + real Postgres + real Go `Engine` (skipped without DSN) |

## Integration tests (Postgres)

Point at a database you can **truncate** (do not use production):

```bash
set TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5432/orderbook?sslmode=disable
go test ./internal/api/ -v -count=1
```

The suite runs migrations, truncates `orders`, `trades`, and `events`, then exercises `POST /orders` and matcher-failure rollback.

**Docker one-liner** (if you already use Compose):

```bash
docker compose up -d postgres
set TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5432/orderbook?sslmode=disable
go test ./internal/api/ -v -count=1
```

Adjust host/port/user to match your `docker-compose.yml`.

## What is intentionally not automated here

- **Load / stress tests** — not part of `go test`. Use [`cmd/stress`](../cmd/stress) and [stress.md](stress.md); comparison methodology in [benchmarks.md](benchmarks.md).
- **Rust matcher binary** — CI would `cargo test` / `docker build` in `matcher/` separately; the Go client is covered via `httptest`.
- **End-to-end with NATS/Redis** — optional; happy path is API + DB + in-process matcher.

## CI suggestion

```bash
go test ./... -count=1
```

`-race` is ideal on Linux/macOS CI where the race detector is easy to enable. On Windows, `-race` often requires a C toolchain (`gcc`) for `cgo`; if `go test -race` fails to build, run without `-race` locally or use WSL/Linux runners in CI.

Optional Postgres job:

```bash
export TEST_DATABASE_URL=postgres://...
go test ./internal/api/ -v -count=1
```

If you add a pipeline, failing the build when `TEST_DATABASE_URL` is set and integration tests fail is stronger than skipping them silently in PRs.
