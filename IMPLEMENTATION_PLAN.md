# Implementation Plan

This plan is based on the architecture described in `documentation.md`: a Go feature flag system split into a flexible control plane and a fast, deterministic data plane.

## Phase 1: Project Foundation

Goal: Create the base Go service and architecture boundaries.

- [x] Initialize Go module.
- [x] Create basic project layout:
  - [x] `cmd/server`
  - [x] `internal/domain`
  - [x] `internal/eval`
  - [x] `internal/store`
  - [x] `internal/control`
  - [x] `internal/api`
  - [x] `internal/sync`
  - [x] `internal/db`
- [x] Add configuration loading for database URL, port, and sync interval.
- [x] Add basic HTTP server.
- [x] Add health endpoint:
  - [x] `GET /health`
- [x] Add structured error response format.
- [x] Add initial README explaining control plane vs data plane.
- [x] Add Makefile or task commands:
  - [x] `make test`
  - [x] `make run`
  - [x] `make bench`

## Phase 1.5: Dockerized Local Development

Goal: Make the service and its dependencies runnable in a consistent local environment.

- [x] Add `Dockerfile` for the Go API service.
- [x] Add `.dockerignore`.
- [x] Add `docker-compose.yml`.
- [x] Add PostgreSQL service.
- [x] Configure API service environment:
  - [x] `HTTP_ADDR=:8080`
  - [x] `DATABASE_URL=postgres://...`
  - [x] `SYNC_INTERVAL=5s`
- [x] Expose API port:
  - [x] `8080:8080`
- [x] Expose PostgreSQL port:
  - [x] `5432:5432`
- [x] Add container health check for `/health`.
- [x] Add persistent Postgres volume.
- [x] Add Makefile targets:
  - [x] `make docker-build`
  - [x] `make docker-up`
  - [x] `make docker-down`
  - [x] `make docker-logs`
- [x] Document Docker usage in README.

## Phase 2: Domain Model and Validation

Goal: Implement the source-of-truth models used by the control plane.

- [x] Define control-plane models:
  - [x] `Flag`
  - [x] `Variant`
  - [x] `Rule`
  - [x] `Context`
- [x] Include required fields:
  - [x] flag key
  - [x] enabled state
  - [x] default variant
  - [x] variants
  - [x] rollout weights
  - [x] rules
  - [x] version
- [x] Add validation rules:
  - [x] flag key is required
  - [x] default variant exists
  - [x] variant names are unique
  - [x] weights are non-negative
  - [x] weights sum to expected total, likely `100`
  - [x] rule operators are supported
  - [x] rule variants exist
  - [x] rule priorities are valid
- [x] Add unit tests for valid and invalid configs.
- [x] Keep validation out of the hot evaluation path.

## Phase 3: Compiled Evaluation Engine

Goal: Build the fast data-plane evaluator described in the document.

- [x] Define runtime models separate from DB/control models:
  - [x] `CompiledFlag`
  - [x] `CompiledRule`
  - [x] `WeightedVariant`
- [x] Implement compiler:
  - [x] `Flag -> CompiledFlag`
- [x] Compile rules into executable match logic.
- [x] Support initial operators:
  - [x] `eq`
  - [x] `in`
- [x] Sort rules by priority during compilation.
- [x] Precompute total rollout weight.
- [x] Implement deterministic bucketing:
  - [x] hash `user_id`
  - [x] hash `flag_key`
  - [x] avoid string concatenation where practical
- [x] Implement weighted variant selection.
- [x] Implement evaluator:
  - [x] disabled flag returns default
  - [x] matching rule returns rule variant
  - [x] missing user ID returns default
  - [x] rollout fallback uses stable hash
- [x] Add unit tests for:
  - [x] disabled flags
  - [x] default behavior
  - [x] rule matching
  - [x] rule priority
  - [x] deterministic bucketing
  - [x] weighted rollout
  - [x] missing user ID
- [x] Add benchmark tests with `go test -bench=. -benchmem`.
- [x] Target near-zero allocations in evaluator benchmarks.

## Phase 4: Immutable In-Memory Store

Goal: Add the lock-free, read-optimized data plane store.

- [x] Define immutable `Store`:
  - [x] `map[string]*CompiledFlag`
- [x] Add atomic holder:
  - [x] `atomic.Value` storing `*Store`
- [x] Implement store load:
  - [x] `Current() *Store`
- [x] Implement atomic replacement:
  - [x] `Swap(newStore *Store)`
- [x] Ensure live stores are never mutated after publication.
- [x] Add lookup method:
  - [x] `GetFlag(key string) (*CompiledFlag, bool)`
- [x] Add service method:
  - [x] `Evaluate(flagKey string, ctx Context) variant`
- [x] Add concurrency tests with `go test -race`.
- [x] Test that reads continue while swaps happen.
- [x] Add benchmark for:
  - [x] atomic load
  - [x] map lookup
  - [x] evaluation
- [x] Add store generation/version metadata for future sync work.

## Phase 5: PostgreSQL Persistence

Goal: Implement the normalized control-plane database model.

- [x] Add database migrations.
- [x] Create tables:
  - [x] `flags`
  - [x] `variants`
  - [x] `rules`
- [x] Include versioning on flags.
- [x] Include timestamps:
  - [x] `created_at`
  - [x] `updated_at`
- [x] Add repository methods:
  - [x] create flag
  - [x] update flag
  - [x] delete flag
  - [x] list flags
  - [x] get flag by key
  - [x] load all flags for data-plane compilation
- [x] Increment version on every config change.
- [x] Wrap multi-table flag writes in transactions.
- [x] Add repository integration tests.
- [x] Add seed data for local development.

## Phase 6: Admin Control Plane APIs

Goal: Expose CRUD APIs for managing flags.

- [x] Implement:
  - [x] `POST /flags`
  - [x] `PUT /flags/:key`
  - [x] `GET /flags`
  - [x] `GET /flags/:key`
  - [x] `DELETE /flags/:key`
- [x] Validate request payloads before persistence.
- [x] Add reusable API-level validation response mapping before admin handlers.
- [x] Return validation errors clearly.
- [x] Increment version on updates.
- [x] Trigger data-plane refresh after successful writes.
- [x] Add API tests for:
  - [x] valid create
  - [x] invalid weights
  - [x] invalid rule operator
  - [x] update version increment
  - [x] delete behavior
- [x] Document request/response examples in README.

## Phase 7: Evaluation API

Goal: Add remote evaluation support.

- [x] Implement:
  - [x] `POST /evaluate`
- [x] Support request shape:

```json
{
  "flag_key": "checkout_flow",
  "context": {
    "user_id": "123",
    "country": "BR"
  }
}
```

- [x] Support response shape:

```json
{
  "variant": "A"
}
```

- [x] Ensure `/evaluate` uses only the in-memory store.
- [x] Do not call PostgreSQL from `/evaluate`.
- [x] Return default or a clear error for unknown flags.
- [x] Add latency-focused tests.
- [x] Add benchmark for remote evaluation handler.
- [x] Add README section explaining local vs remote evaluation.

## Phase 8: Sync Between Control Plane and Data Plane

Goal: Keep the in-memory compiled store fresh.

- [x] Implement full reload from PostgreSQL.
- [x] Compile all DB flags into a fresh immutable `Store`.
- [x] Atomically swap the new store.
- [x] Add polling sync:
  - [x] default every `5s`
- [x] Track last successful refresh time.
- [x] Log sync failures without breaking evaluation.
- [x] Keep old store active if reload fails.
- [x] Add manual refresh hook after admin writes.
- [x] Add tests for:
  - [x] successful refresh
  - [x] failed refresh preserves old store
  - [x] deleted flags disappear after refresh
  - [x] updated versions replace old versions

## Phase 9: Real-Time Updates

Goal: Add faster propagation using Postgres `LISTEN/NOTIFY`, while keeping polling as fallback.

- [x] Add database trigger or application-level `NOTIFY flags_updated`.
- [x] Implement Postgres listener.
- [x] On notification, reload and swap store.
- [x] Add reconnect handling.
- [x] Keep polling fallback enabled.
- [x] Avoid duplicate refresh storms with basic debounce.
- [x] Add integration test or manual verification flow:
  - [x] update flag
  - [x] notification fires
  - [x] store refreshes
  - [x] evaluation changes without restart
- [x] Document eventual consistency tradeoff.

## Phase 10: Go SDK / Client Layer

Goal: Support both local and remote evaluation modes.

- [x] Create SDK package, for example `pkg/client`.
- [x] Implement remote client:
  - [x] calls `POST /evaluate`
- [x] Implement local client:
  - [x] downloads compiled or raw config
  - [x] evaluates locally
- [x] Add client API:

```go
variant := client.Eval("checkout", user)
```

- [x] Add config sync for local mode.
- [x] Add fallback behavior when sync fails.
- [x] Add tests proving local mode does not call remote evaluation during hot path.
- [x] Document tradeoff:
  - [x] local evaluation is fastest but eventually consistent
  - [x] remote evaluation is fresher but has network latency

## Phase 11: Performance, Reliability, and Observability

Goal: Prove the design behaves predictably under load.

- [x] Add benchmarks:
  - [x] evaluator only
  - [x] store lookup + evaluator
  - [x] HTTP `/evaluate`
- [x] Track:
  - [x] `ns/op`
  - [x] `allocs/op`
  - [x] `B/op`
- [x] Add load test script for 10k+ evaluations.
- [x] Add p50/p95/p99 latency reporting.
- [x] Add metrics:
  - [x] evaluation count
  - [x] unknown flag count
  - [x] sync success/failure count
  - [x] current store version or generation
  - [x] refresh duration
- [x] Ensure logging does not block hot path.
- [x] Run race detector.
- [x] Profile CPU and memory if allocations appear.
- [x] Add README benchmark results.

## Phase 12: Optional React Admin UI

Goal: Build the product surface mentioned in the document.

- [ ] Create simple React frontend.
- [x] Add flag list page.
- [x] Add create/edit flag form.
- [x] Add variant editor.
- [x] Add rule editor.
- [x] Add rollout weight editor.
- [x] Show flag version.
- [x] Show enabled/disabled state.
- [x] Call admin APIs.
- [x] Add validation feedback in the UI.
- [ ] Keep this phase optional until backend behavior is solid.

## Recommended Build Order

1. Evaluation engine.
2. Immutable atomic store.
3. PostgreSQL persistence.
4. Admin APIs.
5. Evaluation API.
6. Sync loop.
7. LISTEN/NOTIFY.
8. SDK.
9. Benchmarks and README polish.
10. Optional UI.

This order keeps the heart of the system honest: the data plane becomes fast and deterministic before the slower control-plane features grow around it.
