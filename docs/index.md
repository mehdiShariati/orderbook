---
layout: default
title: Orderbook
description: Systems portfolio — order book matching, scale story (vertical vs horizontal), transactional API, Go & Rust, observability, explicit tradeoffs
---

![Go](https://img.shields.io/badge/Go-API-00ADD8?logo=go&logoColor=white)
![Rust](https://img.shields.io/badge/Rust-matcher-000000?logo=rust&logoColor=white)
![Postgres](https://img.shields.io/badge/Postgres-data-4169E1?logo=postgresql&logoColor=white)
![Docker](https://img.shields.io/badge/Compose-stack-2496ED?logo=docker&logoColor=white)

## What this is

**A match engine is not CRUD.** You can scale **stateless** APIs by adding replicas; the **order book** is **mutable, in-memory state** sitting next to **durable** orders and trades in Postgres. **Scaling** that shape means knowing **what** you partition, **what** you replay, and **what** you fan out over a bus—**before** you buy more CPUs.

This repo is a **portfolio-grade systems slice**: working **price–time** matching (Go or optional **Rust** HTTP service), **transactional** order flow (**rollback** if matching fails), **NATS** events, **Redis** limits, **Prometheus** metrics, **stress tooling**, and docs that **name tradeoffs** instead of hiding them.

---

## How scale works here — first glance

| Layer | Role | Scale **up** (vertical) | Scale **out** (horizontal) |
|-------|------|-------------------------|----------------------------|
| **HTTP API** | REST + WS entry | Bigger machine, tune pools | **Stateless replicas** behind a load balancer |
| **Matcher** | In-memory book | Same box: Go; or **Rust** on another core/host | **Partition by `symbol`** · multiple matcher processes · **replay** after restart |
| **Postgres** | Source of truth for orders/trades | Faster disk, pool tuning, query work | Read replicas for reads · partitioning when tables get hot |
| **Market data** | WS + NATS today | Bigger pipe | **Shared pub/sub** so every API pod sees the same stream |

```text
  traffic ──► [ API ] ──► can replicate (no session affinity for REST)
               │
               ├──► matcher (STATE) ──► shard / replay / isolate CPU
               │
               └──► Postgres (TRUTH) ──► replicas · partition · tune
```

**The honest gap:** this repo **implements** the vertical path and **documents** the horizontal one (routing, replay, WS fan-out). That’s the same **story** you’d walk through in a system-design interview—**with code** behind the API and matcher.

[**SCALING.md**](SCALING.md) has the diagrams (vertical vs horizontal). [**TRADEOFFS.md**](TRADEOFFS.md) has the **why we didn’t** async-everything / auth / multi-pod WS yet.

---

## What a reviewer can verify quickly

| Signal | Where it shows up |
|--------|-------------------|
| **Correctness under failure** | Matcher error → **HTTP 502**, DB **transaction rolled back** (integration tests) |
| **Scale literacy** | Stateless vs **stateful** matcher · **partitioning** story · WS **limits** named |
| **Operational sense** | `/metrics`, Grafana, **`cmd/stress`**, rate-limit behavior documented |
| **Judgment** | Tradeoff **ledger** below—not every shortcut is an accident |

---

## Roadmap — build · scale · trade off

```text
         BUILD — what ships?
        /              \
   SCALE — grow        TRADE OFF — what we skip on purpose
```

| Vertex | Question | Start here |
|--------|----------|------------|
| **Build** | How do orders become trades end-to-end? | [**ORDERBOOK.md**](ORDERBOOK.md) |
| **Scale** | Where is the bottleneck? What does “horizontal” mean for a **book**? | [**SCALING.md**](SCALING.md) |
| **Trade off** | What did we optimize for vs defer? | [**TRADEOFFS.md**](TRADEOFFS.md) |

### Tradeoff ledger (snapshot)

| Decision | Gain | Cost |
|----------|------|------|
| Synchronous match, then NATS + audit | One clear request path | Not a fully async matcher consumer pipeline |
| Optional Rust matcher over HTTP | Isolate CPU; scale matcher tier separately | Network hop; must **fail the request** if match fails (DB rolls back) |
| Redis rate limit fail-open | Stay available if Redis blips | Weaker abuse protection during that window |
| WebSocket hub in-process | Simple broadcast | **Extra design** for many API replicas (NATS bridge, etc.) |

### Next implementation steps (pick one)

1. **Matcher replay** — rebuild the book from Postgres or an event log after restart.  
2. **Symbol routing** — deterministic **hash(`symbol`) → matcher** instance.  
3. **WebSocket at scale** — events through **NATS/Redis**; every API instance pushes to its sockets.  
4. **Read path** — cache top-of-book or read replicas for hot `GET /book`.  
5. **Chaos / failure tests** — document behavior when Postgres or matcher is unhealthy.

More detail: [**SCALING.md**](SCALING.md) (*Growth path*).

---

## Stack snapshot

| | |
|--|--|
| **Matching** | Go in-process **or** Rust via `MATCHER_URL` — same `Engine` abstraction |
| **Persistence** | PostgreSQL — orders, trades, audit `events` |
| **Platform** | Redis, NATS, Prometheus + Grafana, Docker Compose |
| **Quality** | `go test`, optional Postgres integration tests, `cmd/stress` |

---

## All documentation

| Doc | Topic |
|-----|--------|
| [ORDERBOOK.md](ORDERBOOK.md) | Architecture, API, diagrams |
| [SCALING.md](SCALING.md) | Order book + vertical / horizontal scale |
| [TRADEOFFS.md](TRADEOFFS.md) | Design decisions |
| [TESTING.md](TESTING.md) | Tests |
| [stress.md](stress.md) | Stress CLI, load |
| [benchmarks.md](benchmarks.md) | Go vs Rust methodology |
| [PRD.md](PRD.md) | Requirements |
| [README.md](README.md) | Doc index |

---

## Source code

{% if site.github %}
<a href="{{ site.github.repository_url }}" class="btn">View repository on GitHub</a>
{% else %}
Clone this repository from GitHub to run the API and stack locally.
{% endif %}

Root project **README** lives in the repository (outside `docs/`).
