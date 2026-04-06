---
layout: default
title: Orderbook
description: Simulated exchange backend — documentation home
---

![Go](https://img.shields.io/badge/Go-API-00ADD8?logo=go&logoColor=white)
![Rust](https://img.shields.io/badge/Rust-matcher-000000?logo=rust&logoColor=white)
![Postgres](https://img.shields.io/badge/Postgres-data-4169E1?logo=postgresql&logoColor=white)
![Docker](https://img.shields.io/badge/Compose-stack-2496ED?logo=docker&logoColor=white)

**Simulated exchange backend** — limit & market orders, **price–time** matching, REST + WebSocket market data, durable orders and trades, Prometheus metrics.

---

## At a glance

| | |
|--|--|
| **Matching** | Go in-process **or** Rust HTTP service (`MATCHER_URL`) — same `Engine` abstraction |
| **Persistence** | PostgreSQL — orders, trades, append-only audit `events` |
| **Platform** | Redis (rate limits), NATS (events), Prometheus + Grafana, Docker Compose |
| **Quality** | `go test`, optional Postgres integration tests, `cmd/stress` for load |

---

## For reviewers

Read these first — they cover **architecture**, **scaling**, and **tradeoffs**, not only features.

| Doc | What you get |
|-----|----------------|
| [**Architecture & API**](ORDERBOOK.md) | Diagrams (open the `.md` on GitHub for Mermaid), HTTP reference, data model |
| [**Scaling**](SCALING.md) | Order book basics, vertical vs horizontal diagrams, how this repo maps |
| [**Tradeoffs**](TRADEOFFS.md) | Go vs Rust matcher, NATS design, Redis, WebSocket limits |

---

## All documentation

| Doc | Topic |
|-----|--------|
| [ORDERBOOK.md](ORDERBOOK.md) | Full technical write-up |
| [TRADEOFFS.md](TRADEOFFS.md) | Design decisions |
| [SCALING.md](SCALING.md) | Scale & order-book context |
| [TESTING.md](TESTING.md) | Unit & integration tests |
| [stress.md](stress.md) | Stress CLI, Prometheus |
| [benchmarks.md](benchmarks.md) | Go vs Rust matcher methodology |
| [PRD.md](PRD.md) | Original requirements |
| [README.md](README.md) | Doc index |

---

## Source code

{% if site.github %}
<a href="{{ site.github.repository_url }}" class="btn">View repository on GitHub</a>
{% else %}
Clone this repository from GitHub to run the API and stack locally.
{% endif %}

The project overview **README** lives at the **repository root** (outside this `docs/` folder).
