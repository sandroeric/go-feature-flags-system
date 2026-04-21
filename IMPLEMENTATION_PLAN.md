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

- [ ] Define control-plane models:
  - [ ] `Flag`
  - [ ] `Variant`
  - [ ] `Rule`
  - [ ] `Context`
- [ ] Include required fields:
  - [ ] flag key
  - [ ] enabled state
  - [ ] default variant
  - [ ] variants
  - [ ] rollout weights
  - [ ] rules
  - [ ] version
- [ ] Add validation rules:
  - [ ] flag key is required
  - [ ] default variant exists
  - [ ] variant names are unique
  - [ ] weights are non-negative
  - [ ] weights sum to expected total, likely `100`
  - [ ] rule operators are supported
  - [ ] rule variants exist
  - [ ] rule priorities are valid
- [ ] Add unit tests for valid and invalid configs.
- [ ] Keep validation out of the hot evaluation path.

## Phase 3: Compiled Evaluation Engine

Goal: Build the fast data-plane evaluator described in the document.

- [ ] Define runtime models separate from DB/control models:
  - [ ] `CompiledFlag`
  - [ ] `CompiledRule`
  - [ ] `WeightedVariant`
- [ ] Implement compiler:
  - [ ] `Flag -> CompiledFlag`
- [ ] Compile rules into executable match logic.
- [ ] Support initial operators:
  - [ ] `eq`
  - [ ] `in`
- [ ] Sort rules by priority during compilation.
- [ ] Precompute total rollout weight.
- [ ] Implement deterministic bucketing:
  - [ ] hash `user_id`
  - [ ] hash `flag_key`
  - [ ] avoid string concatenation where practical
- [ ] Implement weighted variant selection.
- [ ] Implement evaluator:
  - [ ] disabled flag returns default
  - [ ] matching rule returns rule variant
  - [ ] missing user ID returns default
  - [ ] rollout fallback uses stable hash
- [ ] Add unit tests for:
  - [ ] disabled flags
  - [ ] default behavior
  - [ ] rule matching
  - [ ] rule priority
  - [ ] deterministic bucketing
  - [ ] weighted rollout
  - [ ] missing user ID
- [ ] Add benchmark tests with `go test -bench=. -benchmem`.
- [ ] Target near-zero allocations in evaluator benchmarks.

## Phase 4: Immutable In-Memory Store

Goal: Add the lock-free, read-optimized data plane store.

- [ ] Define immutable `Store`:
  - [ ] `map[string]*CompiledFlag`
- [ ] Add atomic holder:
  - [ ] `atomic.Value` storing `*Store`
- [ ] Implement store load:
  - [ ] `Current() *Store`
- [ ] Implement atomic replacement:
  - [ ] `Swap(newStore *Store)`
- [ ] Ensure live stores are never mutated after publication.
- [ ] Add lookup method:
  - [ ] `GetFlag(key string) (*CompiledFlag, bool)`
- [ ] Add service method:
  - [ ] `Evaluate(flagKey string, ctx Context) variant`
- [ ] Add concurrency tests with `go test -race`.
- [ ] Test that reads continue while swaps happen.
- [ ] Add benchmark for:
  - [ ] atomic load
  - [ ] map lookup
  - [ ] evaluation

## Phase 5: PostgreSQL Persistence

Goal: Implement the normalized control-plane database model.

- [ ] Add database migrations.
- [ ] Create tables:
  - [ ] `flags`
  - [ ] `variants`
  - [ ] `rules`
- [ ] Include versioning on flags.
- [ ] Include timestamps:
  - [ ] `created_at`
  - [ ] `updated_at`
- [ ] Add repository methods:
  - [ ] create flag
  - [ ] update flag
  - [ ] delete flag
  - [ ] list flags
  - [ ] get flag by key
  - [ ] load all flags for data-plane compilation
- [ ] Increment version on every config change.
- [ ] Wrap multi-table flag writes in transactions.
- [ ] Add repository integration tests.
- [ ] Add seed data for local development.

## Phase 6: Admin Control Plane APIs

Goal: Expose CRUD APIs for managing flags.

- [ ] Implement:
  - [ ] `POST /flags`
  - [ ] `PUT /flags/:key`
  - [ ] `GET /flags`
  - [ ] `GET /flags/:key`
  - [ ] `DELETE /flags/:key`
- [ ] Validate request payloads before persistence.
- [ ] Return validation errors clearly.
- [ ] Increment version on updates.
- [ ] Trigger data-plane refresh after successful writes.
- [ ] Add API tests for:
  - [ ] valid create
  - [ ] invalid weights
  - [ ] invalid rule operator
  - [ ] update version increment
  - [ ] delete behavior
- [ ] Document request/response examples in README.

## Phase 7: Evaluation API

Goal: Add remote evaluation support.

- [ ] Implement:
  - [ ] `POST /evaluate`
- [ ] Support request shape:

```json
{
  "flag_key": "checkout_flow",
  "context": {
    "user_id": "123",
    "country": "BR"
  }
}
```

- [ ] Support response shape:

```json
{
  "variant": "A"
}
```

- [ ] Ensure `/evaluate` uses only the in-memory store.
- [ ] Do not call PostgreSQL from `/evaluate`.
- [ ] Return default or a clear error for unknown flags.
- [ ] Add latency-focused tests.
- [ ] Add benchmark for remote evaluation handler.
- [ ] Add README section explaining local vs remote evaluation.

## Phase 8: Sync Between Control Plane and Data Plane

Goal: Keep the in-memory compiled store fresh.

- [ ] Implement full reload from PostgreSQL.
- [ ] Compile all DB flags into a fresh immutable `Store`.
- [ ] Atomically swap the new store.
- [ ] Add polling sync:
  - [ ] default every `5s`
- [ ] Track last successful refresh time.
- [ ] Log sync failures without breaking evaluation.
- [ ] Keep old store active if reload fails.
- [ ] Add manual refresh hook after admin writes.
- [ ] Add tests for:
  - [ ] successful refresh
  - [ ] failed refresh preserves old store
  - [ ] deleted flags disappear after refresh
  - [ ] updated versions replace old versions

## Phase 9: Real-Time Updates

Goal: Add faster propagation using Postgres `LISTEN/NOTIFY`, while keeping polling as fallback.

- [ ] Add database trigger or application-level `NOTIFY flags_updated`.
- [ ] Implement Postgres listener.
- [ ] On notification, reload and swap store.
- [ ] Add reconnect handling.
- [ ] Keep polling fallback enabled.
- [ ] Avoid duplicate refresh storms with basic debounce.
- [ ] Add integration test or manual verification flow:
  - [ ] update flag
  - [ ] notification fires
  - [ ] store refreshes
  - [ ] evaluation changes without restart
- [ ] Document eventual consistency tradeoff.

## Phase 10: Go SDK / Client Layer

Goal: Support both local and remote evaluation modes.

- [ ] Create SDK package, for example `pkg/client`.
- [ ] Implement remote client:
  - [ ] calls `POST /evaluate`
- [ ] Implement local client:
  - [ ] downloads compiled or raw config
  - [ ] evaluates locally
- [ ] Add client API:

```go
variant := client.Eval("checkout", user)
```

- [ ] Add config sync for local mode.
- [ ] Add fallback behavior when sync fails.
- [ ] Add tests proving local mode does not call remote evaluation during hot path.
- [ ] Document tradeoff:
  - [ ] local evaluation is fastest but eventually consistent
  - [ ] remote evaluation is fresher but has network latency

## Phase 11: Performance, Reliability, and Observability

Goal: Prove the design behaves predictably under load.

- [ ] Add benchmarks:
  - [ ] evaluator only
  - [ ] store lookup + evaluator
  - [ ] HTTP `/evaluate`
- [ ] Track:
  - [ ] `ns/op`
  - [ ] `allocs/op`
  - [ ] `B/op`
- [ ] Add load test script for 10k+ evaluations.
- [ ] Add p50/p95/p99 latency reporting.
- [ ] Add metrics:
  - [ ] evaluation count
  - [ ] unknown flag count
  - [ ] sync success/failure count
  - [ ] current store version or generation
  - [ ] refresh duration
- [ ] Ensure logging does not block hot path.
- [ ] Run race detector.
- [ ] Profile CPU and memory if allocations appear.
- [ ] Add README benchmark results.

## Phase 12: Optional React Admin UI

Goal: Build the product surface mentioned in the document.

- [ ] Create simple React frontend.
- [ ] Add flag list page.
- [ ] Add create/edit flag form.
- [ ] Add variant editor.
- [ ] Add rule editor.
- [ ] Add rollout weight editor.
- [ ] Show flag version.
- [ ] Show enabled/disabled state.
- [ ] Call admin APIs.
- [ ] Add validation feedback in the UI.
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
