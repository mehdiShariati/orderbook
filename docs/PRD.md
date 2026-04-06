# Exchange Order Book Platform — PRD

## 1. Overview

This project is a **containerized exchange backend platform** that simulates the core components of a financial trading system.

It includes:

* A **Go-based API layer**
* A **Rust-based matching engine**
* A **PostgreSQL database** for persistence
* A **Redis cache layer**
* A **message broker (NATS)** for event-driven communication
* A full **observability stack (OpenTelemetry, Prometheus, Grafana)**

The system supports:

* Limit and market orders
* Order matching using price-time priority
* Trade generation
* Market data streaming
* Monitoring and metrics collection

---

## 2. Goals

### Primary Goals

* Build a **low-latency order matching system**
* Demonstrate **Go + Rust hybrid architecture**
* Implement **event-driven system design**
* Showcase **observability and monitoring**
* Demonstrate **reliability and fault tolerance**
* Provide a **clean, production-like system architecture**

### Secondary Goals

* Benchmark Go vs Rust performance
* Demonstrate scalability via symbol partitioning
* Provide clean documentation and developer experience

---

## 3. Non-Goals

* Real financial trading or real money handling
* Full authentication/authorization system
* Regulatory compliance
* High-frequency trading optimizations (nanosecond-level)
* Multi-region deployment

---

## 4. System Architecture

### High-Level Components

* API Gateway (Go)
* Order Service (Go)
* Market Data Service (Go)
* Matching Engine (Rust)
* PostgreSQL (DB)
* Redis (Cache)
* NATS (Message Broker)
* OpenTelemetry Collector
* Prometheus
* Grafana

### Data Flow

1. Client sends order → API Gateway
2. API validates request → Order Service
3. Order Service persists order → PostgreSQL
4. Order Service publishes event → NATS
5. Matching Engine consumes event → processes order
6. Matching Engine emits trades → NATS
7. Market Data Service consumes trades → updates book
8. API serves book/trade data via REST/WebSocket

---

## 5. Functional Requirements

### 5.1 Order Management

System must support:

* Create limit order
* Create market order
* Cancel order
* Retrieve order status

### 5.2 Matching Engine

* Must implement **price-time priority**
* Must support:

  * Partial fills
  * Full fills
  * Remaining quantity tracking
* Must produce trade events

### 5.3 Market Data

* Provide:

  * Top of book (best bid/ask)
  * Full order book depth
  * Recent trades
* Support WebSocket streaming

### 5.4 Persistence

* Orders must be stored in PostgreSQL
* Trades must be stored in PostgreSQL
* Event logs must be stored for auditability

### 5.5 Event System

* All state changes must be event-driven via NATS
* Events:

  * OrderCreated
  * OrderCanceled
  * TradeExecuted
  * BookUpdated

---

## 6. Non-Functional Requirements

### 6.1 Performance

* Order processing latency < 10ms (target)
* Matching latency minimized via Rust engine
* Support high throughput (1000+ orders/sec simulated)

### 6.2 Reliability

* Services must expose:

  * `/health/live`
  * `/health/ready`
* Graceful shutdown required
* Retry logic for transient failures
* Idempotency support for order creation

### 6.3 Scalability

* System must support:

  * Horizontal scaling of API layer
  * Symbol-based partitioning of matching engine
* Stateless services preferred

### 6.4 Observability

* Metrics via Prometheus
* Dashboards via Grafana
* Tracing via OpenTelemetry
* Structured logs (JSON)

---

## 7. Data Model

### Order

* id
* user_id
* symbol
* side (buy/sell)
* type (limit/market)
* price
* quantity
* remaining_quantity
* status
* created_at
* updated_at

### Trade

* id
* symbol
* price
* quantity
* buy_order_id
* sell_order_id
* executed_at

### Event

* id
* type
* payload (JSON)
* created_at

---

## 8. API Design

### POST /orders

Create a new order

### DELETE /orders/{id}

Cancel an order

### GET /orders/{id}

Get order status

### GET /book/{symbol}

Get order book snapshot

### GET /trades/{symbol}

Get recent trades

### WebSocket

* /ws/market
* streams:

  * trades
  * order book updates

---

## 9. Matching Engine Design

### Rules

* Price-time priority
* Buy orders match lowest sell price
* Sell orders match highest buy price

### Data Structures

* BTreeMap (Rust)
* Map + sorted levels (Go version)

### Responsibilities

* Maintain in-memory order book
* Match orders
* Emit trade events
* Maintain sequence numbers

---

## 10. Infrastructure

### Docker

* All services must be containerized
* Multi-stage builds required
* Non-root containers preferred

### Docker Compose (Dev)

Includes:

* all services
* hot reload
* mounted volumes
* seeded DB
* preconfigured Grafana dashboards

### Production Setup

* optimized images
* environment-based configs
* health checks
* resource limits

---

## 11. Observability

### Metrics

* orders_received_total
* trades_executed_total
* match_latency_ms
* http_request_duration
* db_query_latency

### Tracing

Trace:

* API request lifecycle
* DB operations
* matching engine calls

### Dashboards

* API performance
* matching engine metrics
* DB performance
* queue metrics

---

## 12. Reliability Features

* Idempotent order creation
* Retry with backoff
* Dead-letter handling
* Graceful shutdown
* Timeouts
* Backpressure handling

---

## 13. Security (Basic)

* Input validation
* Rate limiting (Redis)
* No auth required in MVP

---

## 14. Development Phases

### Phase 1 — Core MVP (Go only)

* Order book
* Matching logic
* REST API
* PostgreSQL integration

### Phase 2 — Rust Engine

* Rewrite matching engine in Rust
* Integrate via service or FFI
* Benchmark comparison

### Phase 3 — Event-Driven System

* Introduce NATS
* Refactor services to async architecture

### Phase 4 — Observability

* Add OpenTelemetry
* Add Prometheus + Grafana
* Build dashboards

### Phase 5 — Production Readiness

* Docker optimization
* reliability features
* documentation

---

## 15. Success Criteria

* Orders are correctly matched
* System handles concurrent requests
* Metrics are visible in Grafana
* System runs fully via Docker Compose
* Benchmark results documented
* Clean, professional README

---

## 16. Risks

* Over-engineering too early
* Rust integration complexity
* Event consistency issues
* Time spent on infra instead of core logic

---

## 17. Future Enhancements

* Advanced order types (IOC, FOK)
* Event sourcing
* Replay system
* FIX protocol support
* Kubernetes deployment
* Multi-region architecture

---
