# LaunchDarkly-Inspired Feature Flags

This project is a Go implementation of a feature flag system with two distinct planes:

- Control plane: flexible, validated, database-backed flag management.
- Data plane: fast, deterministic, in-memory flag evaluation.

The core architectural goal is to keep all database access, validation, parsing, and configuration management out of the hot path. Runtime evaluation should eventually be a pure in-memory operation built around compiled rules, deterministic bucketing, and atomic store swaps.

## Current Status

Phase 1 is the project foundation:

- Go module initialized.
- Basic package layout created.
- Configuration loading added.
- HTTP server added.
- Health endpoint added.
- Structured JSON error responses added.
- Make targets added for running, testing, formatting, and benchmarking.

## Project Layout

```text
cmd/server        HTTP server entrypoint
internal/api      HTTP routing, handlers, and response helpers
internal/config   environment-based configuration
internal/control  control-plane services
internal/db       PostgreSQL persistence
internal/domain   source-of-truth flag models
internal/eval     compiled data-plane evaluator
internal/store    immutable in-memory store
internal/sync     polling and push-based refresh loops
```

## Configuration

The server reads configuration from environment variables:

| Variable | Default | Description |
| --- | --- | --- |
| `HTTP_ADDR` | `:8080` | Address used by the HTTP server. |
| `PORT` | empty | Used as `:<PORT>` when `HTTP_ADDR` is not set. |
| `DATABASE_URL` | empty | PostgreSQL connection string for later phases. |
| `SYNC_INTERVAL` | `5s` | Data-plane refresh interval for later phases. |

## Run

```bash
make run
```

Then check:

```bash
curl http://localhost:8080/health
```

Expected response:

```json
{
  "service": "launchdarkly",
  "started_at": "2026-04-21T00:00:00Z",
  "status": "ok"
}
```

## Test

```bash
make test
```

## Benchmark

```bash
make bench
```

Benchmarks become more meaningful once the evaluation engine exists in Phase 3.
