# Targeted Rust Migration Design

**Date:** 2026-05-04
**Status:** Approved
**Topic:** Migrating high-performance services (Policy, DLP) from Go to Rust.

## 1. Goal
Improve the performance and latency of Open-Guard's critical path by migrating the Policy Evaluation and Data Loss Prevention (DLP) services to Rust, while maintaining the existing Go ecosystem for other services.

## 2. Architecture: Strangler Fig Pattern
-   **Coexistence:** Rust and Go services will coexist in the same cluster.
-   **Communication:** All services (Rust and Go) will continue to communicate via mTLS.
-   **Data Consistency:** Both implementations will use the same PostgreSQL database, respecting the existing Row-Level Security (RLS) policies.

## 3. Tech Stack
-   **Language:** Rust 1.75+
-   **Web Framework:** [Axum](https://github.com/tokio-rs/axum) (High-performance, async-first).
-   **Database Layer:** [SQLx](https://github.com/launchbadge/sqlx) (Compile-time checked SQL, Postgres).
-   **mTLS/TLS:** `rustls` with `tokio-rustls`.
-   **Telemetry:** `tracing` for logs and `metrics` for Prometheus.

## 4. Component Design

### 4.1 `rust/shared` (Common Logic)
A shared crate to avoid duplication between Rust services:
-   **`auth`**: JWT validation and mTLS certificate loading.
-   **`db`**: SQLx pool initialization with RLS transaction support.
-   **`telemetry`**: Standardized logging and metrics wrappers.

### 4.2 `rust/services/policy` (Target 1)
-   **Purpose:** Evaluate security policies against incoming requests.
-   **Logic:** Ported from `services/policy/pkg/engine`.
-   **Performance Gain:** Faster rule evaluation and reduced memory overhead.

### 4.3 `rust/services/dlp` (Target 2)
-   **Purpose:** Scan content for sensitive data (PII, Secrets).
-   **Logic:** Ported from `services/dlp/pkg/scanner`.
-   **Performance Gain:** SIMD-accelerated pattern matching and efficient buffer management.

## 5. Migration Strategy
1.  Initialize Rust workspace.
2.  Implement `rust/shared` logic (mTLS and RLS are blockers).
3.  Rewrite `policy` service in Rust.
4.  Run parallel tests (Go vs Rust results).
5.  Swap service in `docker-compose.yml`.
6.  Repeat for `dlp` service.

## 6. Verification Plan
-   **mTLS Handshake:** Verify Rust services can connect to the Go-based control plane.
-   **RLS Check:** Ensure Rust queries fail if `org_id` is not set in the session.
-   **Latency Benchmark:** Compare Go vs Rust latency under load (k6).
-   **Acceptance Tests:** Run `make test-acceptance` with the new Rust services.
