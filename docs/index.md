---
layout: default
title: Orderbook
description: Client-delivered simulated exchange backend — matching engine, scale paths (single node to horizontal), Go & Rust, Postgres, observability
---

![Go](https://img.shields.io/badge/Go-API-00ADD8?logo=go&logoColor=white)
![Rust](https://img.shields.io/badge/Rust-matcher-000000?logo=rust&logoColor=white)
![Postgres](https://img.shields.io/badge/Postgres-data-4169E1?logo=postgresql&logoColor=white)
![Docker](https://img.shields.io/badge/Compose-stack-2496ED?logo=docker&logoColor=white)

## What this is

This backend was **built for a client** as a **simulated exchange** stack: **price–time** matching, REST + WebSocket market data, durable orders and trades in **Postgres**, and full **observability**. It is **not** a licensed venue or custody product—the scope was **matching, APIs, persistence, and ops hooks** so the system could be **exercised and extended** responsibly.

**A match engine is not CRUD.** Stateless APIs scale by adding replicas; the **order book** is **mutable in-memory state** next to **durable** truth in the database. **Scaling** that shape means deciding **what** to partition, **what** to replay, and **what** to fan out—whether you run **one node** today or **many** tomorrow.

---

<div class="ob-scale-wrap" markdown="0">
<p class="ob-scale-intro"><strong>Same codebase</strong> — from <em>one deployment</em> to <em>scaled-out</em> tiers (API replicas, matcher partitions, DB replicas). The animation is decorative; the table below is the real story.</p>
<div class="ob-scale-visual">
  <div class="ob-scale-card ob-scale-card--single">
    <h4>Single-node path</h4>
    <p>One API process, in-process Go matcher or Rust on the side, one Postgres. Vertical scale: CPU, RAM, disk.</p>
    <div class="ob-bars ob-bars--single"><div class="ob-bar" aria-hidden="true"></div></div>
  </div>
  <div class="ob-scale-flow"><span aria-hidden="true">→</span></div>
  <div class="ob-scale-card ob-scale-card--scale">
    <h4>At scale (next tiers)</h4>
    <p>Stateless API behind LB · matcher <strong>sharded by symbol</strong> · Postgres replicas / partitioning · shared bus for WS.</p>
    <div class="ob-bars ob-bars--scale">
      <div class="ob-bar" aria-hidden="true"></div>
      <div class="ob-bar" aria-hidden="true"></div>
      <div class="ob-bar" aria-hidden="true"></div>
      <div class="ob-bar" aria-hidden="true"></div>
      <div class="ob-bar" aria-hidden="true"></div>
      <div class="ob-bar" aria-hidden="true"></div>
      <div class="ob-bar" aria-hidden="true"></div>
      <div class="ob-bar" aria-hidden="true"></div>
    </div>
  </div>
</div>
<p class="ob-scale-caption">Bars animate throughput “shape”: one pillar vs many parallel workers — not a benchmark, just a visual metaphor.</p>
</div>

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

**Delivery scope:** This repo **implements** the vertical / single-node path and **documents** the horizontal path (routing, replay, WS fan-out) for when the client grows past one box. [**SCALING.md**](SCALING.md) has the diagrams; [**TRADEOFFS.md**](TRADEOFFS.md) records **why** we didn’t ship async-everything, auth, or multi-pod WebSockets in v1.

---

## What you can verify in the code

| Signal | Where it shows up |
|--------|-------------------|
| **Correctness under failure** | Matcher error → **HTTP 502**, DB **transaction rolled back** (integration tests) |
| **Scale literacy** | Stateless vs **stateful** matcher · **partitioning** story · WS **limits** named |
| **Operational hooks** | `/metrics`, Grafana, **`cmd/stress`**, rate-limit behavior documented |
| **Explicit tradeoffs** | Ledger below—decisions are **named**, not hidden |

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

### Next implementation steps (when the client is ready)

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
