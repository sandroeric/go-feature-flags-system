# LaunchDarkly-Inspired Feature Flags

This project is a Go implementation of a feature flag system with two distinct planes:

- Control plane: flexible, validated, database-backed flag management.
- Data plane: fast, deterministic, in-memory flag evaluation.

The core architectural goal is to keep all database access, validation, parsing, and configuration management out of the hot path. Runtime evaluation should eventually be a pure in-memory operation built around compiled rules, deterministic bucketing, and atomic store swaps.

## Current Status

Phase 1 is the project foundation, and Phase 1.5 adds Dockerized local development:

- Go module initialized.
- Basic package layout created.
- Configuration loading added.
- HTTP server added.
- Health endpoint added.
- Structured JSON error responses added.
- Make targets added for running, testing, formatting, and benchmarking.
- Dockerfile added for the API service.
- Docker Compose added for the API and PostgreSQL.
- Control-plane domain models and validation added.
- Compiled evaluation engine added with deterministic bucketing.
- API validation errors are mapped to a structured response shape.
- Immutable in-memory store and atomic swaps added.
- PostgreSQL migrations and repository added.

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
| `SYNC_INTERVAL` | `5s` | Polling fallback interval for data-plane refreshes. |

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
  "flag_count": "0",
  "service": "launchdarkly",
  "started_at": "2026-04-21T00:00:00Z",
  "store_generation": "0",
  "store_version": "0",
  "status": "ok"
}
```

## Docker

Run the API and PostgreSQL together:

```bash
make docker-up
```

Check the API:

```bash
curl http://localhost:8080/health
```

Follow API logs:

```bash
make docker-logs
```

Stop the local environment:

```bash
make docker-down
```

The Compose environment starts:

- API on `localhost:8080`
- PostgreSQL on `localhost:5432`

The API receives this database URL inside the Compose network:

```text
postgres://launchdarkly:launchdarkly@postgres:5432/launchdarkly?sslmode=disable
```

PostgreSQL data is stored in a named Docker volume called `launchdarkly_postgres-data`.

## Database

When `DATABASE_URL` is set, the server opens PostgreSQL and applies embedded migrations at startup. Docker Compose provides the expected local database URL automatically.

The persistence layer stores flags in normalized tables:

- `flags`
- `variants`
- `rules`

Run database integration tests against the Compose PostgreSQL instance:

```bash
make docker-up
make test-integration
```

## Evaluation Example

```go
flag := domain.Flag{
	Key:     "checkout",
	Enabled: true,
	Default: "off",
	Variants: []domain.Variant{
		{Name: "off", Weight: 50},
		{Name: "on", Weight: 50},
	},
	Rules: []domain.Rule{
		{
			Attribute: "country",
			Operator:  domain.OperatorEq,
			Values:    []string{"BR"},
			Variant:   "on",
			Priority:  1,
		},
	},
	Version: 1,
}

compiled, err := eval.CompileFlag(flag)
if err != nil {
	// handle validation/compile error
}

variant := eval.Evaluate(compiled, &domain.Context{
	UserID:  "123",
	Country: "BR",
})
```

## Admin Control Plane APIs

Phase 6 exposes CRUD APIs for managing flags. The control plane is suitable for admin operations and accepts full flag configurations with validation before persistence.

All endpoints return structured JSON error responses with validation details when applicable.

### Create Flag

```http
POST /flags
Content-Type: application/json

{
  "key": "checkout_flow",
  "enabled": true,
  "default": "control",
  "variants": [
    {"name": "control", "weight": 50},
    {"name": "treatment", "weight": 50}
  ],
  "rules": [
    {
      "attribute": "country",
      "operator": "eq",
      "values": ["BR"],
      "variant": "treatment",
      "priority": 1
    }
  ]
}
```

Response `201 Created`:

```json
{
  "key": "checkout_flow",
  "enabled": true,
  "default": "control",
  "version": 1,
  "variants": [
    {"name": "control", "weight": 50},
    {"name": "treatment", "weight": 50}
  ],
  "rules": [
    {
      "attribute": "country",
      "operator": "eq",
      "values": ["BR"],
      "variant": "treatment",
      "priority": 1
    }
  ]
}
```

Validation errors return `400 Bad Request` with a `validation_failed` error code and per-field details:

```json
{
  "error": {
    "code": "validation_failed",
    "message": "request validation failed",
    "details": [
      {
        "field": "variants",
        "code": "invalid_weights",
        "message": "weights must sum to 100"
      }
    ]
  }
}
```

### Get All Flags

```http
GET /flags
```

Response `200 OK`:

```json
[
  {
    "key": "checkout_flow",
    "enabled": true,
    "default": "control",
    "version": 1,
    "variants": [...],
    "rules": [...]
  }
]
```

Returns an empty array if no flags exist.

### Get Flag by Key

```http
GET /flags/{key}
```

Response `200 OK`:

```json
{
  "key": "checkout_flow",
  "enabled": true,
  "default": "control",
  "version": 1,
  "variants": [...],
  "rules": [...]
}
```

Returns `404 Not Found` if the flag does not exist.

### Update Flag

```http
PUT /flags/{key}
Content-Type: application/json

{
  "enabled": false,
  "default": "control",
  "variants": [
    {"name": "control", "weight": 50},
    {"name": "treatment", "weight": 50}
  ],
  "rules": [...]
}
```

Response `200 OK`:

```json
{
  "key": "checkout_flow",
  "enabled": false,
  "default": "control",
  "version": 2,
  "variants": [...],
  "rules": [...]
}
```

The version is automatically incremented. Returns `404 Not Found` if the flag does not exist.

### Delete Flag

```http
DELETE /flags/{key}
```

Response `204 No Content` on success.

Returns `404 Not Found` if the flag does not exist.

### Data-Plane Refresh

After a flag is created, updated, or deleted, the control plane triggers a refresh to propagate the changes to the data-plane in-memory store. Phase 8 implements the automatic polling and Phase 9 adds real-time updates via database triggers.

## Evaluation API

Phase 7 adds remote evaluation support. The `/evaluate` endpoint provides fast, in-memory evaluation that uses only the data plane store (no database calls).

### Remote Evaluation

```http
POST /evaluate
Content-Type: application/json

{
  "flag_key": "checkout_flow",
  "context": {
    "user_id": "user123",
    "country": "BR"
  }
}
```

Response `200 OK`:

```json
{
  "variant": "treatment"
}
```

Returns `404 Not Found` if the flag does not exist in the data-plane store.

This endpoint is suitable for low-latency evaluation in production. It uses only the in-memory store and performs deterministic bucketing to ensure consistent results for the same user and flag.

## Go SDK

Phase 10 adds a Go client in `pkg/client` with two evaluation modes:

- `remote`: calls `POST /evaluate` for each evaluation
- `local`: polls `GET /flags`, compiles flags locally, and evaluates from an in-memory store

Example:

```go
sdk, err := client.New(client.Config{
	Mode:    client.ModeRemote,
	BaseURL: "http://localhost:8080",
})
if err != nil {
	// handle error
}

variant, err := sdk.Eval("checkout_flow", domain.Context{
	UserID:  "123",
	Country: "BR",
})
if err != nil {
	// handle error
}
```

Tradeoff:

- Local evaluation is fastest on the hot path and avoids network calls, but it is eventually consistent with the server until the next successful sync.
- Remote evaluation is fresher because it always uses the server's current in-memory data plane, but every evaluation pays network latency.

## Data-Plane Sync

Phase 8 implements automatic polling to keep the in-memory store synchronized with the database:

- Polls every `SYNC_INTERVAL` (default `5s`)
- Uses PostgreSQL `LISTEN/NOTIFY` for near-real-time refresh signals
- Compiles all flags from the database into a fresh immutable store
- Atomically swaps the new store
- If a refresh fails, the old store remains active (no downtime)
- Deleted flags are removed after refresh
- Updated flag versions replace old versions
- Manual refresh is triggered immediately after control-plane writes
- Polling remains enabled as a fallback if notifications are missed during reconnects

The sync process ensures that:

1. The data plane is always up-to-date with the control plane
2. Evaluation continues even if a sync fails
3. Store generation increments after each successful sync

### Configuration

Set `SYNC_INTERVAL` to control polling frequency:

```bash
SYNC_INTERVAL=10s ./server
```

### Real-Time Verification

To manually verify the realtime update path end-to-end:

1. Start the stack with `make docker-up`.
2. Create a flag with `POST /flags`.
3. Call `POST /evaluate` and note the returned variant.
4. Update the same flag with `PUT /flags/{key}` so the expected variant changes.
5. Call `POST /evaluate` again without restarting the server.

Expected result: the second evaluation reflects the new configuration immediately, and the server logs show a realtime listener connection followed by a sync.

### Consistency Tradeoff

Evaluation stays available during refreshes because the service swaps in a brand-new immutable store only after a full reload succeeds. The tradeoff is brief eventual consistency: between a control-plane write and the next successful refresh, readers may still observe the previous configuration for a short window.

## Test

```bash
make test
```

## Benchmark

```bash
make bench
```

Current evaluator benchmarks target zero allocations in the hot path.
