# Implementation Plan: Carwash Reservations

**Branch**: `001-carwash-reservations` | **Date**: 2026-10-20 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/001-carwash-reservations/spec.md`

## Summary

Expose a schema-first GraphQL API from a Go `net/http` server for creating, fetching, listing
(paginated by time period), updating and soft-deleting carwash reservations, persisted in
PostgreSQL. The no-overlap rule is enforced by a PostgreSQL exclusion constraint on each active
reservation's time range, so simultaneous bookings for the same slot cannot both succeed. Service
types live in a table (only `carwash`, 60 minutes) so new types are a data change. Mutations
return typed errors in their payloads; queries use stable `extensions.code` values. The service
runs as a minimal non-root container next to PostgreSQL (named volume) in docker compose, logs
JSON, and exposes the GraphQL playground and introspection only in development. Details:
[research.md](./research.md).

## Technical Context

**Language/Version**: Go 1.27

**Primary Dependencies**: gqlgen (GraphQL server + codegen), pgx/v5 + pgxpool, sqlc (codegen,
`go tool`), golang-migrate (`iofs` source, `pgx/v5` driver), testcontainers-go (postgres
module, tests only). Everything else is the standard library (`net/http`, `log/slog`,
`net/mail`, `embed`, `encoding/base64`).

**Storage**: PostgreSQL 19; tables `service_types`, `reservations` ([data-model.md](./data-model.md))

**Testing**: `testing` (table-driven), `go test -race ./...`, testcontainers PostgreSQL 19 for store
and API tests, `httptest` for end-to-end GraphQL tests

**Target Platform**: Linux container (`gcr.io/distroless/static-debian13:nonroot`), docker compose
for local development

**Project Type**: web service (backend-for-frontend, GraphQL API)

**Performance Goals**: first page of a one-week listing in < 1 s (SC-002); expected load is a
handful of staff users

**Constraints**: stateless process; config via env only; no authentication (FR-012) — protected
by network placement and CORS restricted to the frontend origin; customer email/phone never
logged

**Scale/Scope**: single location; at most ~170 active reservations per week (one 60-minute slot
at a time); thousands of rows per year including cancelled ones

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | How the plan complies |
|---|---|---|
| I. Modern, Idiomatic Go | ✅ Pass | Go 1.27; only pre-approved dependencies; `context.Context` first on every I/O path; `%w` wrapping, sentinel errors (`service.ErrNotFound`, `ErrSlotConflict`) checked with `errors.Is`; injected `*slog.Logger`; all wiring in `cmd/server/main.go`, no globals; clock injected into the service. |
| II. Schema-First GraphQL | ✅ Pass | Contract in `contracts/schema.graphqls` → `internal/graph/schema/`; gqlgen generated code committed; Relay connection with `first/after` and `last/before`, opaque cursors; one `<Name>Input` and `<Name>Payload` per mutation; typed `UserError` payloads + `extensions.code` for queries; internal errors masked; complexity + depth limits from env; introspection and playground dev-only. DataLoader: no resolver loads related entities (service type is joined in SQL), so no N+1 exists (R5). |
| III. PostgreSQL as SSoT | ✅ Pass | Stateless process; service types (runtime-mutable config) in DB; golang-migrate up/down migrations embedded and applied on startup; sqlc + pgx/v5 only; update/delete in one transaction with row lock; PK, FK, `NOT NULL`, check constraints, exclusion constraint and partial index express the invariants. No extensions needed (GiST on `tstzrange` is core). |
| IV. Containerised by Default | ✅ Pass | Multi-stage Dockerfile → distroless `nonroot`; `compose.yaml` with `bff` + `postgres:19` and named volume `pgdata`; env config validated at startup (fail fast); `/livez` + `/readyz`; graceful shutdown on SIGTERM/SIGINT (not-ready → `Server.Shutdown` with `SHUTDOWN_TIMEOUT` → pool close). |
| V. Testing Discipline | ✅ Pass | Unit tests for every logic package, table-driven, `-race`; store integration tests on testcontainers PostgreSQL 19 incl. concurrency and migrations up/down/up; API tests per acceptance scenario. |
| VI. Security & Simplicity | ✅ Pass | All GraphQL args validated in the service before use; env validated at startup; no secrets in repo (`.env` git-ignored, compose uses dev-only credentials); CORS allow-list, `*` rejected unless `APP_ENV=development`; three internal packages as requested, no extra layers; small hand-written CORS and depth-limit code instead of dependencies. |

**Post-design re-check (after Phase 1)**: ✅ All six principles still pass. Design choices that
touch the gates: exclusion constraint (III, no extension), joined service type instead of a
DataLoader (II, justified in R5), custom depth limiter (II, ~50 lines, no dependency), `go tool`
for sqlc/gqlgen (I, pre-approved tools). Complexity Tracking is empty.

## Project Structure

### Documentation (this feature)

```text
specs/001-carwash-reservations/
├── spec.md
├── plan.md              # this file
├── research.md          # Phase 0
├── data-model.md        # Phase 1
├── quickstart.md        # Phase 1
├── contracts/
│   └── schema.graphqls  # Phase 1: GraphQL contract
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 (/speckit-tasks, not created here)
```

### Source Code (repository root)

```text
cmd/server/
├── main.go                  # load config, logger, pool, migrate, wire deps, serve, shut down
└── main_test.go             # (optional) smoke test of wiring with testcontainers

internal/config/
├── config.go                # env parsing + validation (R12)
└── config_test.go

internal/graph/
├── schema/
│   └── reservation.graphqls # copied from contracts/schema.graphqls
├── generated.go             # gqlgen executable schema (generated, committed)
├── model/
│   └── models_gen.go        # gqlgen models (generated, committed)
├── resolver.go              # Resolver struct: holds service + logger
├── reservation.resolvers.go # resolvers (gqlgen layout: follow-schema); map service results ↔ GraphQL
├── convert.go               # domain ↔ model conversion, UserError mapping
├── errors.go                # error presenter (codes, masking), recover func
├── depthlimit.go            # max-depth handler extension
├── server.go                # builds the gqlgen handler: transports, limits, dev-only introspection
├── *_test.go                # unit tests (depth limit, error mapping)
└── api_test.go              # end-to-end GraphQL tests (httptest + testcontainers)

internal/httpserver/
├── server.go                # ServeMux: /graphql, /playground (dev), /livez, /readyz
├── middleware.go            # request ID, access log, recover, CORS
└── *_test.go

internal/service/
├── reservation.go           # Service: Create, Get, List, Update, Delete; rules from data-model.md
├── validate.go              # field validation, collected FieldErrors
├── cursor.go                # opaque cursor encode/decode
├── errors.go                # ErrNotFound, ErrSlotConflict, ValidationErrors
├── store.go                 # Store interface the service depends on (consumer-defined)
└── *_test.go                # table-driven unit tests with a fake Store

internal/store/
├── migrations/
│   ├── 000001_create_service_types.up.sql / .down.sql
│   └── 000002_create_reservations.up.sql / .down.sql
├── migrations.go            # embed.FS + Migrate(ctx, databaseURL)
├── queries/
│   └── reservations.sql     # sqlc queries
├── sqlc.yaml
├── db/                      # sqlc output (generated, committed)
├── store.go                 # Store: pool + tx helper, implements service.Store, maps 23P01/no rows
└── store_test.go            # integration tests (testcontainers), migrations up/down/up

internal/logging/
└── logging.go               # JSON slog handler + request-ID-from-context wrapper

gqlgen.yml                   # gqlgen config (schema/exec/model paths, DateTime → graphql.Time)
go.mod / go.sum              # incl. `tool` directives for gqlgen and sqlc
.golangci.yml
Dockerfile
compose.yaml                 # bff + postgres:19, named volume pgdata
.dockerignore
.env.example                 # documented variables, no real secrets
```

**Structure Decision**: Single Go module, layout as requested — `cmd/server`, `internal/graph`
(schema + resolvers), `internal/service` (business logic), `internal/store` (sqlc queries +
migrations). Three small supporting packages are added because they are cross-cutting and
independently testable: `internal/config` (env, Principle IV), `internal/httpserver` (mux +
middleware, keeps HTTP concerns out of `graph`) and `internal/logging` (request-ID-aware slog
handler). Dependency direction: `cmd/server` → `httpserver` → `graph` → `service` ← `store`
(`store` implements the interface declared in `service`; `service` never imports `store`).

## Key Design Notes

- **Overlap**: enforced by `reservations_no_overlap` exclusion constraint; store maps SQLSTATE
  `23P01` to `service.ErrSlotConflict` (R2).
- **Time**: service receives `now func() time.Time`; all comparisons on UTC instants; hourly grid
  check (minute :00) in service and DB (R3).
- **Update**: one transaction — `SELECT … FOR UPDATE` (cancelled or missing → not found), apply
  rules against the locked row, recompute `ends_at` when `starts_at` or service type changes,
  `UPDATE … RETURNING` joined row (R10).
- **Delete**: `UPDATE … SET cancelled_at = $now WHERE id = $1 AND cancelled_at IS NULL
  RETURNING …`; zero rows → not found.
- **Listing**: keyset on `(starts_at, id)` over the partial index; fetch `limit + 1` rows to compute
  `hasNextPage`/`hasPreviousPage` (R7).
- **Errors**: service returns `ValidationErrors` / sentinels; `graph` maps them to payload
  `UserError`s (mutations) or `extensions.code` (queries); anything else → logged, masked as
  `INTERNAL_SERVER_ERROR` (R8).
- **HTTP**: `/graphql` always; `/playground` and introspection only when `APP_ENV=development`
  (R11).
- **Shutdown**: on signal, flip readiness to false, `http.Server.Shutdown(ctx with
  SHUTDOWN_TIMEOUT)`, then `pool.Close()`.

## Complexity Tracking

No constitution violations; nothing to justify.
