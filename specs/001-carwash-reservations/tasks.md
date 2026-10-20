---

description: "Task list for 001-carwash-reservations"
---

# Tasks: Carwash Reservations

**Input**: Design documents from `/specs/001-carwash-reservations/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md),
[data-model.md](./data-model.md), [contracts/schema.graphqls](./contracts/schema.graphqls),
[quickstart.md](./quickstart.md)

**Tests**: Included. Constitution Principle V makes them mandatory: table-driven unit tests for
every logic package, testcontainers integration tests for the data layer, migrations tested up and
down, everything green under `go test -race ./...`.

**Organization**: Tasks are grouped by user story so each story can be implemented and verified
as its own increment.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: User story the task belongs to (US1–US4)
- Paths are relative to the repository root. Go module path: `github.com/pballok/miszacsi-bff`.

## Conventions used by every task

- Rules from the constitution apply everywhere: `context.Context` first on every I/O function,
  errors wrapped with `fmt.Errorf("...: %w", err)` and inspected with `errors.Is`/`errors.As`,
  never log-and-return in the same layer, no package-level mutable state, injected `*slog.Logger`,
  no new dependencies beyond gqlgen, pgx/v5, sqlc, golang-migrate, testcontainers-go.
- Generated code (`internal/graph/generated.go`, `internal/graph/model/models_gen.go`,
  `internal/store/db/`) is never edited by hand; after changing a `.graphqls` or `.sql` file run
  `go generate ./...` and commit the output in the same change.
- Dependency direction: `cmd/server` → `internal/httpserver` → `internal/graph` → `internal/service`
  ← `internal/store`. `internal/service` never imports `internal/store`, `internal/graph` or pgx.
- Times: compare and store UTC instants; the hourly grid is checked on the UTC instant.
- Test dates are in 2030 (`2030-01-15`, offset `+01:00`) so they stay in the future.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Module, tooling and code-generation configuration

- [ ] T001 Initialise the Go module in go.mod: `module github.com/pballok/miszacsi-bff`, `go 1.27`; add dependencies `github.com/jackc/pgx/v5`, `github.com/99designs/gqlgen`, `github.com/vektah/gqlparser/v2`, `github.com/golang-migrate/migrate/v4` (with `database/pgx/v5` and `source/iofs`), `github.com/testcontainers/testcontainers-go` and `github.com/testcontainers/testcontainers-go/modules/postgres`; add tool directives with `go get -tool github.com/99designs/gqlgen` and `go get -tool github.com/sqlc-dev/sqlc/cmd/sqlc`; create the empty directories `cmd/server`, `internal/{config,logging,graph/schema,graph/model,httpserver,service,store/migrations,store/queries,store/db}` from plan.md
- [ ] T002 [P] Create .golangci.yml (golangci-lint v2 format) enabling `errcheck`, `govet`, `staticcheck`, `ineffassign`, `unused`, `errorlint`, `gosec`, `bodyclose`, `noctx`, `misspell`, `revive`; formatters `gofmt`; exclude generated files (`generated: strict`) so `internal/graph/generated.go`, `internal/graph/model/models_gen.go` and `internal/store/db/` are skipped
- [ ] T003 [P] Create gqlgen.yml at the repository root: `schema: [internal/graph/schema/*.graphqls]`, `exec: {filename: internal/graph/generated.go, package: graph}`, `model: {filename: internal/graph/model/models_gen.go, package: model}`, `resolver: {layout: follow-schema, dir: internal/graph, package: graph, filename_template: "{name}.resolvers.go"}`, models `DateTime` → `github.com/99designs/gqlgen/graphql.Time`, `ID` → `github.com/99designs/gqlgen/graphql.ID`; and internal/graph/generate.go containing only `package graph` and `//go:generate go tool gqlgen generate`
- [ ] T004 [P] Create internal/store/sqlc.yaml (version 2): engine `postgresql`, `queries: queries/`, `schema: migrations/`, gen go `package: db`, `out: db`, `sql_package: pgx/v5`, `emit_pointers_for_null_types: true`, overrides `timestamptz` → `time.Time` (and nullable → `*time.Time`); and internal/store/generate.go containing only `package store` and `//go:generate go tool sqlc generate -f sqlc.yaml`
- [ ] T005 [P] Create .env.example documenting every variable from research.md R12 with its default and a dev-only `DATABASE_URL=postgres://miszacsi:miszacsi@localhost:5432/miszacsi?sslmode=disable` (no real secrets), and .dockerignore excluding `.git`, `.idea`, `.claude`, `.specify`, `specs`, `.env`, `*.test`, `*.out`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Config, logging, database schema, store/service skeletons, GraphQL plumbing, HTTP
server, container setup and test harnesses that every story builds on

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

### Configuration and logging

- [ ] T006 [P] Implement internal/config/config.go: `type Config struct { DatabaseURL string; HTTPAddr string; AppEnv string; CORSAllowedOrigins []string; GraphQLMaxDepth int; GraphQLMaxComplexity int; ShutdownTimeout time.Duration; LogLevel slog.Level }`, method `Development() bool`, and `Load(getenv func(string) string) (Config, error)` applying research.md R12 exactly: `DATABASE_URL` required; `HTTP_ADDR` default `:8080`; `APP_ENV` `development`|`production`, default `production`; `CORS_ALLOWED_ORIGINS` comma-separated, trimmed, required in production, `*` rejected in production, allowed in development (default `*` there); `GRAPHQL_MAX_DEPTH` default `10`, `GRAPHQL_MAX_COMPLEXITY` default `200` (both positive integers); `SHUTDOWN_TIMEOUT` default `15s` (`time.ParseDuration`, > 0); `LOG_LEVEL` `debug`|`info`|`warn`|`error`, default `info`. Collect every problem and return them joined with `errors.Join` so the message lists all of them
- [ ] T007 [P] Write table-driven tests in internal/config/config_test.go covering every default, every invalid value, `*` accepted in development and rejected in production, missing `CORS_ALLOWED_ORIGINS` in production, and multiple errors reported together (use a map-backed `getenv`)
- [ ] T008 [P] Implement internal/logging/logging.go: `New(w io.Writer, level slog.Level) *slog.Logger` using `slog.NewJSONHandler` wrapped in a `contextHandler` whose `Handle` adds `request_id` when present in the record's context; `WithRequestID(ctx, id) context.Context` and `RequestID(ctx) (string, bool)` using an unexported key type. Never log customer email or phone anywhere in the code base
- [ ] T009 [P] Write tests in internal/logging/logging_test.go: output is valid JSON, `request_id` present when logged with `InfoContext` on a context from `WithRequestID`, absent otherwise, level filtering works, `WithAttrs`/`WithGroup` keep the wrapper

### Database schema and migrations

- [ ] T010 [P] Write internal/store/migrations/000001_create_service_types.up.sql creating `service_types` with exactly: `code text PRIMARY KEY CHECK (code ~ '^[a-z][a-z0-9_]{0,49}$')`, `display_name text NOT NULL CHECK (length(display_name) BETWEEN 1 AND 100)`, `duration_minutes integer NOT NULL CHECK (duration_minutes > 0 AND duration_minutes % 60 = 0)`, `offered boolean NOT NULL DEFAULT true`, followed by `INSERT INTO service_types (code, display_name, duration_minutes, offered) VALUES ('carwash', 'Carwash', 60, true);`; and 000001_create_service_types.down.sql with `DROP TABLE service_types;`
- [ ] T011 [P] Write internal/store/migrations/000002_create_reservations.up.sql creating `reservations` with exactly: `id uuid PRIMARY KEY DEFAULT uuidv7()`; `starts_at timestamptz NOT NULL CHECK (extract(epoch FROM starts_at)::numeric % 3600 = 0)`; `ends_at timestamptz NOT NULL CHECK (ends_at > starts_at)`; `customer_name text NOT NULL CHECK (length(customer_name) BETWEEN 1 AND 200)`; `customer_email text NOT NULL CHECK (length(customer_email) BETWEEN 3 AND 254)`; `customer_phone text NOT NULL CHECK (length(customer_phone) BETWEEN 7 AND 40)`; `service_type text NOT NULL REFERENCES service_types(code) ON UPDATE RESTRICT ON DELETE RESTRICT`; `cancelled_at timestamptz NULL`; `created_at timestamptz NOT NULL DEFAULT now()`; `updated_at timestamptz NOT NULL DEFAULT now()`; table constraint `CHECK (cancelled_at IS NULL OR cancelled_at >= created_at)`; `CONSTRAINT reservations_no_overlap EXCLUDE USING gist (tstzrange(starts_at, ends_at, '[)') WITH &&) WHERE (cancelled_at IS NULL)`; index `CREATE INDEX reservations_active_starts_at_idx ON reservations (starts_at, id) WHERE cancelled_at IS NULL;`. Down file: `DROP TABLE reservations;`
- [ ] T012 Implement internal/store/migrations.go: `//go:embed migrations/*.sql` into an unexported `embed.FS`; `Migrate(ctx context.Context, databaseURL string) error` using golang-migrate `iofs.New` + `pgx/v5` database driver (convert the `postgres://`/`postgresql://` scheme to `pgx5://`), run `Up`, treat `migrate.ErrNoChange` as success, close source and driver, honour `ctx` cancellation via `GracefulStop`; plus unexported `migrateDown` used only by tests (depends on T010, T011)

### Domain types and store/service skeleton

- [ ] T013 [P] Create internal/service/model.go with domain types: `ServiceType{Code, DisplayName string; DurationMinutes int; Offered bool}` and method `Duration() time.Duration`; `Status` (`StatusActive`, `StatusCancelled`); `Reservation{ID string; StartsAt, EndsAt time.Time; CustomerName, CustomerEmail, CustomerPhone string; ServiceType ServiceType; CancelledAt *time.Time; CreatedAt, UpdatedAt time.Time}` with method `Status() Status` (`CANCELLED` iff `CancelledAt != nil`); `NewReservation` (fields written on create, including `EndsAt`)
- [ ] T014 [P] Create internal/service/errors.go: sentinels `ErrNotFound`, `ErrSlotConflict`; `type Code string` with constants matching the contract's `ValidationErrorCode` enum (`REQUIRED`, `TOO_LONG`, `INVALID_FORMAT`, `IN_PAST`, `NOT_ON_GRID`, `UNKNOWN_SERVICE_TYPE`, `NOT_ALLOWED`); `FieldError{Field string; Code Code; Message string}` where `Field` is the GraphQL input field name (`customerEmail`, `startsAt`, `to`, `after`, …); `ValidationErrors []FieldError` implementing `error`; helper `(*ValidationErrors).Add(field, code, msg)`
- [ ] T015 Create internal/service/store.go declaring the consumer-side `Store` interface (start empty; each story adds the methods it needs) and internal/service/reservation.go with `type Service struct { store Store; now func() time.Time }` and `New(store Store, now func() time.Time) *Service` (depends on T013, T014)
- [ ] T016 Create internal/store/store.go: `type Store struct { pool *pgxpool.Pool; q *db.Queries }`, `New(pool *pgxpool.Pool) *Store`, `Ping(ctx) error`, unexported `withTx(ctx, fn func(q *db.Queries) error) error` (begin, rollback on error, commit), `mapErr(err error) error` translating `pgx.ErrNoRows` → `service.ErrNotFound` and `*pgconn.PgError` with `Code == "23P01"` → `service.ErrSlotConflict` (wrapped with `%w`, other errors wrapped with context), and UUID helpers `parseUUID(string) (pgtype.UUID, error)` / `uuidString(pgtype.UUID) string`. Conversion from sqlc rows to `service.Reservation` is added by the stories once sqlc row types exist (depends on T012, T015)
- [ ] T017 Write the store test harness and migration tests in internal/store/store_test.go: `TestMain` starts one `postgres:19` testcontainer (`modules/postgres`, wait for readiness), runs `Migrate` on a `template_miszacsi` database, and a helper `newTestStore(t) (*Store, *pgxpool.Pool)` that creates a fresh database `CREATE DATABASE t_<random> TEMPLATE template_miszacsi`, registers cleanup; tests: migrations up → down → up on a scratch database succeed; seeded `carwash` row exists with duration 60; raw SQL inserts proving the constraints: `starts_at` at `10:30` violates the grid check, at `10:00:30` violates it, `10:00` passes; `duration_minutes = 90` rejected; two overlapping active rows rejected with SQLSTATE `23P01`; back-to-back rows (10:00–11:00 and 11:00–12:00) accepted; a row overlapping only a cancelled row accepted (depends on T012, T016)

### GraphQL plumbing

- [ ] T018 Copy specs/001-carwash-reservations/contracts/schema.graphqls verbatim to internal/graph/schema/reservation.graphqls, create internal/graph/resolver.go with `type Resolver struct { svc *service.Service; logger *slog.Logger }`, run `go generate ./internal/graph/...` and commit internal/graph/generated.go, internal/graph/model/models_gen.go and the generated stub internal/graph/reservation.resolvers.go (stubs remain `panic("not implemented")` until each story fills them in) (depends on T003, T015)
- [ ] T019 [P] Implement internal/graph/errors.go: unexported `clientError{code, field, msg string}`; constructors `badUserInput(field, msg string) error` (`extensions.code = "BAD_USER_INPUT"`, `extensions.field = field`) and `notFound(msg string) error` (`extensions.code = "NOT_FOUND"`); `errorPresenter(logger)` for `handler.SetErrorPresenter` that keeps gqlgen's own parse/validation errors and `clientError`s (setting `extensions.code`) and turns any other error into message `"internal server error"` with `extensions.code = "INTERNAL_SERVER_ERROR"` after logging it with `logger.ErrorContext(ctx, "graphql resolver error", "err", err)` (request ID comes from context); `recoverFunc(logger)` for `handler.SetRecoverFunc` that logs the panic and stack and returns an internal error (depends on T008, T018)
- [ ] T020 [P] Implement internal/graph/depthlimit.go: `type depthLimit struct{ max int }` implementing `graphql.HandlerExtension` and `graphql.OperationContextMutator`; `MutateOperationContext` computes the maximum selection depth of `rc.Operation` (following fragment spreads via `rc.Doc.Fragments` and inline fragments, ignoring `__typename`, guarding against fragment cycles) and returns a `gqlerror` with message `"query depth N exceeds limit M"` and `extensions.code = "DEPTH_LIMIT_EXCEEDED"` when exceeded (depends on T018)
- [ ] T021 Write table-driven tests in internal/graph/depthlimit_test.go (depth exactly at limit passes, limit + 1 fails, depth via named and inline fragments counted, `__typename` ignored) and internal/graph/errors_test.go (client errors keep code/field, unknown error masked as `INTERNAL_SERVER_ERROR` with generic message and logged with `request_id`, panic recovered and masked) (depends on T019, T020)
- [ ] T022 Implement internal/graph/server.go: `type Options struct { MaxDepth, MaxComplexity int; Development bool }` and `NewHandler(svc *service.Service, logger *slog.Logger, opts Options) http.Handler` using `handler.New(NewExecutableSchema(Config{Resolvers: &Resolver{...}}))`, transports `transport.Options{}`, `transport.GET{}`, `transport.POST{}`, `extension.FixedComplexityLimit(opts.MaxComplexity)`, `depthLimit{opts.MaxDepth}`, `extension.Introspection{}` only when `opts.Development`, `SetErrorPresenter` and `SetRecoverFunc` from T019 (depends on T019, T020)

### HTTP server and process wiring

- [ ] T023 [P] Implement internal/httpserver/middleware.go: `RequestID` (accept incoming `X-Request-ID` matching `^[A-Za-z0-9._-]{1,64}$`, otherwise generate 16 random bytes from `crypto/rand` hex-encoded; store with `logging.WithRequestID`; echo in the response header), `AccessLog(logger)` (one `info` record per request: `method`, `path`, `status`, `duration_ms`, logged with the request context; never logs bodies or query strings), `Recover(logger)` (logs panic, responds 500), `CORS(allowed []string)` (exact-match origin allow-list; `*` allows any origin; sets `Access-Control-Allow-Origin` to the request origin plus `Vary: Origin`; preflight `OPTIONS` with an allowed origin → 204 with `Access-Control-Allow-Methods: GET, POST, OPTIONS`, `Access-Control-Allow-Headers: Content-Type, X-Request-ID`, `Access-Control-Max-Age: 600`; disallowed origin → no CORS headers) (depends on T008)
- [ ] T024 Implement internal/httpserver/server.go: `type Pinger interface{ Ping(ctx context.Context) error }`; `New(graphql http.Handler, db Pinger, ready *atomic.Bool, logger *slog.Logger, cfg config.Config) http.Handler` building an `http.ServeMux` with `POST /graphql` and `GET /graphql` → graphql handler, `GET /playground` → `playground.Handler("MiSzaCsi BFF", "/graphql")` registered only when `cfg.Development()`, `GET /livez` → 200 `ok`, `GET /readyz` → 200 when `ready.Load()` and `db.Ping` (2 s timeout) succeeds, otherwise 503; whole mux wrapped as Recover → RequestID → AccessLog → CORS (depends on T006, T023)
- [ ] T025 Write table-driven tests in internal/httpserver/middleware_test.go and internal/httpserver/server_test.go with `httptest`: request ID accepted/replaced/generated and echoed; access log JSON contains `request_id` and `status`; panic → 500; CORS allowed origin, disallowed origin, `*` in development, preflight 204; `/livez` 200; `/readyz` 503 when not ready or ping fails, 200 otherwise; `/playground` 200 in development and 404 in production (depends on T023, T024)
- [ ] T026 Implement cmd/server/main.go: `main` calls `run(ctx, os.Getenv, os.Stdout) error` and exits 1 on error after printing it to stderr; `run` uses `signal.NotifyContext(ctx, syscall.SIGTERM, os.Interrupt)`, `config.Load`, `logging.New`, `pgxpool.New(ctx, cfg.DatabaseURL)` + `Ping`, `store.Migrate`, wires `store.New` → `service.New(st, time.Now)` → `graph.NewHandler` → `httpserver.New`, sets `ready` to true only after migrations succeed, starts `http.Server{Addr: cfg.HTTPAddr, Handler: h, ReadHeaderTimeout: 5 * time.Second}`; on signal: `ready.Store(false)`, `srv.Shutdown` with a `cfg.ShutdownTimeout` context, then `pool.Close()`; logs `starting`/`shutting down`/`stopped` (depends on T006, T008, T012, T016, T022, T024)
- [ ] T027 [P] Write the multi-stage Dockerfile: build stage `golang:1.27` (`go mod download` layer first, then `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server`); final stage `gcr.io/distroless/static-debian13:nonroot` with only `/server`, `USER nonroot:nonroot`, `EXPOSE 8080`, `ENTRYPOINT ["/server"]`
- [ ] T028 [P] Write compose.yaml: service `postgres` (`image: postgres:19`, env `POSTGRES_USER/POSTGRES_PASSWORD/POSTGRES_DB = miszacsi` dev-only, volume `pgdata:/var/lib/postgresql`, healthcheck `pg_isready -U miszacsi -d miszacsi` every 5 s, port `5432:5432`); service `bff` (`build: .`, `depends_on: {postgres: {condition: service_healthy}}`, env `DATABASE_URL=postgres://miszacsi:miszacsi@postgres:5432/miszacsi?sslmode=disable`, `APP_ENV=development`, `CORS_ALLOWED_ORIGINS=http://localhost:5173`, port `8080:8080`); top-level named volume `volumes: {pgdata: {}}`

### API test harness

- [ ] T029 Write the API test harness in internal/graph/api_test.go (package `graph_test`): `TestMain` starts one `postgres:19` testcontainer and migrates a template database; helper `newTestServer(t) (*httptest.Server, *pgxpool.Pool)` creates a fresh database from the template and builds the real stack (`store.New` → `service.New(st, time.Now)` → `graph.NewHandler` with `Development: true` → `httpserver.New`); helper `gql(t, srv, query string, vars map[string]any) response` posting JSON to `/graphql` and decoding `data` and `errors`; helper `at(hhmm string) string` returning `2030-01-15T<hh:mm>:00+01:00`. The real clock is used because `created_at` comes from PostgreSQL `now()` and `cancelled_at >= created_at` is enforced (depends on T017, T026)

**Checkpoint**: `go generate ./... && go build ./... && go test -race ./...` passes; `docker compose up --build` starts, `/livez` and `/readyz` return 200, `/playground` loads (resolvers still unimplemented)

---

## Phase 3: User Story 1 - Create a reservation (Priority: P1) 🎯 MVP

**Goal**: `createReservation` stores a validated, non-overlapping reservation; `reservation(id)`
(FR-013) fetches it back, active or cancelled

**Independent Test**: create a valid reservation, restart the BFF, fetch it by ID and get the same
details (quickstart scenarios 1–6, 18, 19)

### Tests for User Story 1

- [ ] T030 [P] [US1] Write table-driven tests in internal/service/validate_test.go for every create rule in data-model.md "Validation rules": name `""`/whitespace → `REQUIRED`, 201 code points → `TOO_LONG`, 200 → ok, surrounding whitespace trimmed; email `""` → `REQUIRED`, 255 chars → `TOO_LONG`, `x`, `Bob <b@x.hu>`, `a@localhost` → `INVALID_FORMAT`, `anna@example.hu` ok; phone `""` → `REQUIRED`, `12`, `+36 30 abc`, 16 digits → `INVALID_FORMAT`, `+36 30 123 4567` and `(06) 30/123-4567` ok; startsAt equal to now or earlier → `IN_PAST`, `10:30`, `10:15`, `10:00:30`, `10:00:00.5` → `NOT_ON_GRID`, `10:00+01:00` ok, `11:30+05:30` ok (06:00 UTC); all invalid fields of one input reported together
- [ ] T031 [P] [US1] Write tests in internal/service/reservation_test.go for `Create` and `Get` against a hand-written `fakeStore` (in the same test file): values trimmed before storing, `EndsAt = StartsAt + 60m`, unknown or not-offered service type → `UNKNOWN_SERVICE_TYPE` field error, `ErrSlotConflict` from the store returned unchanged (`errors.Is`), validation failure never calls the store, `Get` with a malformed ID → `ValidationErrors{id INVALID_FORMAT}`, unknown ID → `ErrNotFound`
- [ ] T032 [P] [US1] Add integration tests to internal/store/store_test.go: `GetServiceType("carwash")` returns duration 60, unknown code → `ErrNotFound`; `CreateReservation` round-trips every field and `GetReservation` returns it with the joined service type; overlapping create → `ErrSlotConflict`; back-to-back create succeeds; 10 goroutines creating the same slot concurrently → exactly 1 success and 9 `ErrSlotConflict`
- [ ] T033 [P] [US1] Add end-to-end tests to internal/graph/api_test.go for quickstart.md scenarios 1–6, 18 and 19: valid create returns `id`, `status: ACTIVE`, `endsAt` 11:00, `errors: []`; four `ValidationError`s for scenario 2; `IN_PAST` and `NOT_ON_GRID` (at `10:30`) for scenario 3; `SlotConflictError` for a second create at `10:00`; back-to-back `11:00` succeeds; a new `httptest` server on the same database (simulated restart) returns the same reservation from `reservation(id)`; unknown UUID → GraphQL error `extensions.code = "NOT_FOUND"`; malformed ID → `BAD_USER_INPUT`; 10 parallel creates for one slot → 1 success, 9 `SlotConflictError`

### Implementation for User Story 1

- [ ] T034 [US1] Write internal/store/queries/reservations.sql with sqlc queries `GetServiceType :one` (`SELECT * FROM service_types WHERE code = $1`), `CreateReservation :one` (a `WITH ins AS (INSERT INTO reservations (starts_at, ends_at, customer_name, customer_email, customer_phone, service_type) VALUES (...) RETURNING *) SELECT ins.*, st.display_name, st.duration_minutes, st.offered FROM ins JOIN service_types st ON st.code = ins.service_type`) and `GetReservation :one` (same projection from `reservations r JOIN service_types st` `WHERE r.id = $1`); run `go generate ./internal/store/...` and commit internal/store/db/
- [ ] T035 [US1] Implement in internal/store/store.go: `GetServiceType(ctx, code) (service.ServiceType, error)`, `CreateReservation(ctx, service.NewReservation) (service.Reservation, error)`, `GetReservation(ctx, id string) (service.Reservation, error)` plus the unexported row → `service.Reservation` conversion; every error goes through `mapErr` (depends on T034)
- [ ] T036 [P] [US1] Implement internal/service/validate.go: unexported validators `validateName`, `validateEmail`, `validatePhone`, `validateStartsAt(t, now)`, `validateID` each appending to `*ValidationErrors` with the codes and rules from data-model.md "Validation rules" — name 1–200 code points (`utf8.RuneCountInString`); email ≤ 254 chars, `net/mail.ParseAddress` succeeds, parsed `Address` equals the input, domain contains a dot; phone matches `^\+?[0-9 ()./-]+$` with 7–15 digits; startsAt strictly after `now`, and `t.UTC().Truncate(time.Hour).Equal(t)` else `NOT_ON_GRID`; ID matches the canonical UUID pattern `^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`. All inputs are trimmed with `strings.TrimSpace` before validation
- [ ] T037 [US1] Add `GetServiceType`, `CreateReservation`, `GetReservation` to the `Store` interface in internal/service/store.go and implement in internal/service/reservation.go `type CreateInput struct{ StartsAt time.Time; CustomerName, CustomerEmail, CustomerPhone, ServiceType string }`, `Create(ctx, CreateInput) (Reservation, error)` (trim, validate all fields, look up the service type — missing or `!Offered` → `UNKNOWN_SERVICE_TYPE` on `serviceType` —, return `ValidationErrors` if any, compute `EndsAt = StartsAt.UTC() + type.Duration()`, call the store) and `Get(ctx, id string) (Reservation, error)` (depends on T035, T036)
- [ ] T038 [US1] Implement internal/graph/convert.go: `toModelReservation(service.Reservation) *model.Reservation` (status, `cancelledAt`, nested `ServiceType`); `toUserErrors(err error) ([]model.UserError, bool)` mapping `service.ValidationErrors` → one `*model.ValidationError` per field, `service.ErrSlotConflict` → `*model.SlotConflictError{Message: "the requested time overlaps another reservation"}`, `service.ErrNotFound` → `*model.NotFoundError{Message: "reservation not found"}`, and `false` for anything else; `toQueryError(err error) error` mapping `ValidationErrors` → `badUserInput(field, msg)` for the first field, `ErrNotFound` → `notFound(...)`, otherwise the error unchanged (masked by the presenter) (depends on T019, T037)
- [ ] T039 [US1] Implement `CreateReservation` and `Reservation` in internal/graph/reservation.resolvers.go: create returns `{Reservation, Errors: []}` on success, `{Reservation: nil, Errors: userErrors}` for client errors, and a top-level error otherwise; `Reservation(id)` returns the reservation or `toQueryError(err)` (depends on T038)

**Checkpoint**: US1 tests pass; quickstart scenarios 1–6, 18, 19 work in the playground

---

## Phase 4: User Story 2 - List active reservations in a time period (Priority: P1)

**Goal**: `reservations(from, to, first, after, last, before)` returns active reservations whose
start is in `[from, to)`, ascending, as a Relay connection

**Independent Test**: create reservations inside and outside a period and a cancelled one; the
listing returns exactly the active ones inside it, in order, across pages (quickstart 7–10)

### Tests for User Story 2

- [ ] T040 [P] [US2] Write table-driven tests in internal/service/cursor_test.go: encode → decode round-trip; decoding non-base64, missing `|`, bad time or bad UUID fails; encoded form is base64url without padding
- [ ] T041 [P] [US2] Add `List` tests to internal/service/reservation_test.go with `fakeStore`: `to` not after `from` → field `to`; `first` and `last` both set → field `last`; `first`/`last` outside 1–100 → that field; undecodable `after`/`before` → that field; default page size 20; the store is asked for `limit + 1` rows and `HasNextPage` (forward) / `HasPreviousPage` (backward) is true only when the extra row came back; backward results are returned in ascending order; edge cursors encode each row's `(StartsAt, ID)`
- [ ] T042 [P] [US2] Add integration tests to internal/store/store_test.go for `ListReservations`: start inclusive / end exclusive at the boundaries, reservation starting before `from` (08:00 when listing from 08:30) not returned, cancelled rows (inserted with raw SQL `cancelled_at = now()`) excluded, ascending order, forward keyset after a cursor, backward keyset before a cursor
- [ ] T043 [P] [US2] Add end-to-end tests to internal/graph/api_test.go for quickstart.md scenarios 7–10 and US2-4/US2-5: create 09:00, 10:00, 11:00, 15:00 and next-day 09:00, list 08:00–12:00 → exactly 09:00, 10:00, 11:00; empty day → `edges: []`; `to` before `from` → `BAD_USER_INPUT` with `extensions.field = "to"`; a cancelled row (raw SQL) is not listed; `first: 2` then `after: endCursor` → 2 then 1 edges with `hasNextPage` true then false; `last: 2` then `before: startCursor` walks backward over the same rows

### Implementation for User Story 2

- [ ] T044 [US2] Add to internal/store/queries/reservations.sql `ListReservationsForward :many` (`WHERE r.cancelled_at IS NULL AND r.starts_at >= @from AND r.starts_at < @to AND (sqlc.narg('after_starts_at')::timestamptz IS NULL OR (r.starts_at, r.id) > (sqlc.narg('after_starts_at')::timestamptz, sqlc.narg('after_id')::uuid)) ORDER BY r.starts_at, r.id LIMIT @row_limit`) and `ListReservationsBackward :many` (same with `before_*`, `<` and `ORDER BY r.starts_at DESC, r.id DESC`), both joining `service_types` like `GetReservation`; run `go generate ./internal/store/...`
- [ ] T045 [P] [US2] Implement internal/service/cursor.go: `type Cursor struct{ StartsAt time.Time; ID string }`, `EncodeCursor(Cursor) string` = `base64.RawURLEncoding` of `StartsAt.UTC().Format(time.RFC3339Nano) + "|" + ID`, `DecodeCursor(string) (Cursor, error)` validating both parts
- [ ] T046 [US2] Implement in internal/store/store.go `ListReservations(ctx, service.ListQuery) ([]service.Reservation, error)` (first declare `type ListQuery struct{From, To time.Time; Forward bool; Cursor *Cursor; Limit int}` in internal/service/store.go) that dispatches to the forward or backward query and returns rows in query order (depends on T044, T045)
- [ ] T047 [US2] Add `ListReservations` to the `Store` interface and implement `List(ctx, ListInput) (Page, error)` in internal/service/reservation.go with `ListInput{From, To time.Time; First, Last *int; After, Before *string}`, `ListQuery`, and `Page{Edges []Edge; HasNextPage, HasPreviousPage bool}` / `Edge{Cursor string; Node Reservation}`: validate per data-model.md (`to` after `from`; `first`/`last` in 1–100 and not both; cursors decode), default 20 forward, fetch `limit + 1`, trim the extra row, reverse backward results to ascending; forward `HasPreviousPage = After != nil`, backward `HasNextPage = Before != nil` (depends on T045, T046)
- [ ] T048 [US2] Implement `Reservations` in internal/graph/reservation.resolvers.go: map arguments to `service.ListInput`, map `Page` to `model.ReservationConnection` (edges, `PageInfo` with `startCursor`/`endCursor` from the first/last edge or null when empty), errors via `toQueryError` (depends on T038, T047)

**Checkpoint**: US1 and US2 tests pass; quickstart scenarios 1–10 work

---

## Phase 5: User Story 3 - Update a reservation (Priority: P2)

**Goal**: `updateReservation` changes any subset of fields with the same validation, atomically,
without overlapping other active reservations

**Independent Test**: create a reservation, update its time and phone, fetch it; only those fields
(and `endsAt`, `updatedAt`) changed (quickstart 11–14)

### Tests for User Story 3

- [ ] T049 [P] [US3] Add `Update` tests to internal/service/reservation_test.go with `fakeStore`: only supplied (non-nil) fields change; supplied fields validated with the create rules; new `startsAt` recomputes `EndsAt`; changing to an unknown or not-offered service type → `UNKNOWN_SERVICE_TYPE`; keeping an existing retired type is allowed; reservation whose `StartsAt` is not after now: `startsAt` or `serviceType` supplied → `NOT_ALLOWED` on that field, customer fields still updatable; cancelled or missing → `ErrNotFound`; `UpdatedAt` set to the clock's now; no store write when validation fails
- [ ] T050 [P] [US3] Add integration tests to internal/store/store_test.go for `UpdateReservation`: callback receives the locked current row; changing only the name does not conflict with the row's own slot; moving onto another active reservation → `ErrSlotConflict` and the row is unchanged; cancelled row → `ErrNotFound`; callback error rolls back and is returned unchanged
- [ ] T051 [P] [US3] Add end-to-end tests to internal/graph/api_test.go for quickstart.md scenarios 11–14: update `startsAt` to 13:00 and `customerPhone` → new values, other fields equal, `endsAt` 14:00, `updatedAt` advanced; `customerEmail: "bad"` → `ValidationError` and stored row unchanged; unknown UUID → `NotFoundError`; moving to 11:00 occupied by another → `SlotConflictError` and unchanged

### Implementation for User Story 3

- [ ] T052 [US3] Add to internal/store/queries/reservations.sql `GetReservationForUpdate :one` (joined projection, `WHERE r.id = $1 FOR UPDATE OF r`) and `UpdateReservation :one` (`WITH upd AS (UPDATE reservations SET starts_at = @starts_at, ends_at = @ends_at, customer_name = @customer_name, customer_email = @customer_email, customer_phone = @customer_phone, service_type = @service_type, updated_at = @updated_at WHERE id = @id RETURNING *) SELECT upd.*, st.display_name, st.duration_minutes, st.offered FROM upd JOIN service_types st ON st.code = upd.service_type`); run `go generate ./internal/store/...`
- [ ] T053 [US3] Implement in internal/store/store.go `UpdateReservation(ctx, id string, apply func(current service.Reservation) (service.Reservation, error)) (service.Reservation, error)`: in one `withTx`, lock the row with `GetReservationForUpdate` (no row or `CancelledAt != nil` → `service.ErrNotFound`), call `apply`, return its error unchanged, otherwise write the returned reservation with `UpdateReservation`; errors through `mapErr` (depends on T052)
- [ ] T054 [US3] Add `UpdateReservation` to the `Store` interface and implement `Update(ctx, UpdateInput) (Reservation, error)` in internal/service/reservation.go with `UpdateInput{ID string; StartsAt *time.Time; CustomerName, CustomerEmail, CustomerPhone, ServiceType *string}`: validate the ID and supplied fields up front, then inside the `apply` callback enforce the rules against the locked row (already started → `NOT_ALLOWED` for `startsAt`/`serviceType`; a changed service type must exist and be offered; recompute `EndsAt` when `startsAt` or service type changes; set `UpdatedAt = now`), returning `ValidationErrors` from the callback so the transaction rolls back (depends on T037, T053)
- [ ] T055 [US3] Implement `UpdateReservation` in internal/graph/reservation.resolvers.go mapping `model.UpdateReservationInput` (nil pointer = keep) to `service.UpdateInput` and results/errors exactly like create (depends on T038, T054)

**Checkpoint**: US1–US3 tests pass; quickstart scenarios 1–14 work

---

## Phase 6: User Story 4 - Delete a reservation (Priority: P2)

**Goal**: `deleteReservation` soft-deletes (sets `cancelled_at`), frees the slot, keeps the
reservation fetchable as `CANCELLED`

**Independent Test**: create, delete, list the period (gone), fetch by ID (`CANCELLED`), create
again in the same slot (succeeds) (quickstart 15–17)

### Tests for User Story 4

- [ ] T056 [P] [US4] Add `Delete` tests to internal/service/reservation_test.go with `fakeStore`: malformed ID → `ValidationErrors{id INVALID_FORMAT}`; store `ErrNotFound` passed through; the clock's now is passed as cancellation time
- [ ] T057 [P] [US4] Add integration tests to internal/store/store_test.go for `CancelReservation`: sets `cancelled_at` and `updated_at`; second cancel → `ErrNotFound`; unknown ID → `ErrNotFound`; a new reservation in the freed slot succeeds; `GetReservation` still returns the cancelled row
- [ ] T058 [P] [US4] Add end-to-end tests to internal/graph/api_test.go for quickstart.md scenarios 15–17: delete returns `status: CANCELLED` with `cancelledAt`; the period listing no longer contains it; create in the same slot succeeds; deleting again → `NotFoundError`; `reservation(id)` returns it with `status: CANCELLED`; `updateReservation` on it → `NotFoundError`

### Implementation for User Story 4

- [ ] T059 [US4] Add to internal/store/queries/reservations.sql `CancelReservation :one` (`WITH c AS (UPDATE reservations SET cancelled_at = @now, updated_at = @now WHERE id = @id AND cancelled_at IS NULL RETURNING *) SELECT c.*, st.display_name, st.duration_minutes, st.offered FROM c JOIN service_types st ON st.code = c.service_type`); run `go generate ./internal/store/...`
- [ ] T060 [US4] Implement `CancelReservation(ctx, id string, now time.Time) (service.Reservation, error)` in internal/store/store.go (no row → `service.ErrNotFound` via `mapErr`) (depends on T059)
- [ ] T061 [US4] Add `CancelReservation` to the `Store` interface and implement `Delete(ctx, id string) (Reservation, error)` in internal/service/reservation.go (validate ID, call the store with `s.now()`) (depends on T037, T060)
- [ ] T062 [US4] Implement `DeleteReservation` in internal/graph/reservation.resolvers.go: success → `{Reservation, Errors: []}`; `ErrNotFound` or malformed ID → `{Reservation: nil, Errors: [...]}` via `toUserErrors` (depends on T038, T061)

**Checkpoint**: all four stories pass; quickstart scenarios 1–19 work

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Production-mode guarantees, performance, privacy, docs and final validation

- [ ] T063 [P] Add production-mode tests in internal/graph/api_test.go using `Development: false` and a production config: introspection query rejected; `GET /playground` → 404; a query nested deeper than `GRAPHQL_MAX_DEPTH` → `DEPTH_LIMIT_EXCEEDED`; a query exceeding `GRAPHQL_MAX_COMPLEXITY` rejected; a request from a non-allowed origin gets no `Access-Control-Allow-Origin`; with the pool closed, `reservation(id)` returns `INTERNAL_SERVER_ERROR` with the generic message and no database details (quickstart scenario 20)
- [ ] T064 [P] Add a privacy test in internal/graph/api_test.go that captures all log output (`logging.New` on a `bytes.Buffer`) while running create, update, list and a failing request, and asserts the customer email and phone strings never appear in it
- [ ] T065 [P] Add the SC-002 performance test in internal/graph/perf_test.go: seed one week with 500 rows via raw SQL (168 active on consecutive hours, 332 cancelled), request the first page of `reservations` for that week through the HTTP handler and assert it completes in under 1 second
- [ ] T066 [P] Add cmd/server/main_test.go: start `run` against a testcontainers `postgres:19` with `HTTP_ADDR=127.0.0.1:0`-style free port and `APP_ENV=development`, wait for `/readyz` 200, cancel the context, assert `run` returns nil within `SHUTDOWN_TIMEOUT`; and an invalid-config case where `run` fails fast listing every missing variable
- [ ] T067 [P] Update README.md: what the service is, `docker compose up --build`, endpoints (`/graphql`, `/playground` dev only, `/livez`, `/readyz`), environment variables table from research.md R12, `go generate ./...`, quality gates from quickstart.md, link to specs/001-carwash-reservations/
- [ ] T068 Run every quality gate from quickstart.md (`gofmt -l .`, `go vet ./...`, `golangci-lint run`, `go generate ./... && git diff --exit-code`, `go test -race ./...`, `docker build -t miszacsi-bff .`) and fix all findings
- [ ] T069 Run `docker compose up --build` and walk through quickstart.md validation scenarios 1–20 in the playground or with `curl`; then `docker compose down` / `up` and confirm data survived via the `pgdata` volume (SC-004)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies; T002–T005 in parallel after T001
- **Foundational (Phase 2)**: depends on Setup; blocks all user stories
- **US1 (Phase 3)**: depends on Foundational
- **US2 (Phase 4)**: depends on Foundational; its API tests create data through `createReservation`, so it is easiest after US1 (store/service listing work can start in parallel with US1)
- **US3 (Phase 5)**: depends on US1 (reuses `validate.go`, `Create` for test data and `GetServiceType`)
- **US4 (Phase 6)**: depends on US1 (test data via create, fetch via `reservation(id)`); independent of US2 and US3 except that T058's listing assertion uses US2's query
- **Polish (Phase 7)**: depends on all stories

### Story completion order

```text
Setup → Foundational → US1 (MVP) ─┬─→ US2
                                  ├─→ US3
                                  └─→ US4 (listing check in T058 needs US2)
                                        └─→ Polish
```

### Within each story

- Tests first (they fail until the story is implemented)
- Queries → `go generate` → store methods → service → graph conversion → resolvers
- Tasks touching the same file (`reservations.sql`, `store.go`, `service/reservation.go`,
  `reservation.resolvers.go`, `api_test.go`, `store_test.go`) are sequential across stories

### Parallel Opportunities

- Setup: T002, T003, T004, T005
- Foundational: T006–T011 and T013, T014 together; then T019, T020, T023, T027, T028; then T021 and T024 → T025
- Each story's test tasks are in different files and can be written in parallel
- US1: T036 (`validate.go`) in parallel with T034/T035 (store)
- US2: T045 (`cursor.go`) in parallel with T044 (queries)
- After US1, US2/US3/US4 store and service work can proceed in parallel by different people if
  edits to shared files are coordinated
- Polish: T063–T067

---

## Parallel Example: User Story 1

```bash
# Tests (different files):
Task: "Validation table tests in internal/service/validate_test.go"
Task: "Create/Get tests with fakeStore in internal/service/reservation_test.go"
Task: "Store integration tests for create/get/concurrency in internal/store/store_test.go"
Task: "API tests for quickstart scenarios 1–6, 18, 19 in internal/graph/api_test.go"

# Implementation that does not share files:
Task: "sqlc queries + generate in internal/store/queries/reservations.sql"
Task: "Field validators in internal/service/validate.go"
```

## Parallel Example: User Story 2

```bash
Task: "Cursor tests in internal/service/cursor_test.go"
Task: "ListReservations integration tests in internal/store/store_test.go"
Task: "Cursor encode/decode in internal/service/cursor.go"
Task: "Forward/backward list queries in internal/store/queries/reservations.sql"
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Phase 1: Setup
2. Phase 2: Foundational (config, migrations, plumbing, compose)
3. Phase 3: US1 — create and fetch
4. **Stop and validate**: quickstart scenarios 1–6, 18, 19; `go test -race ./...`
5. Demo: bookings can be recorded and survive restarts

### Incremental Delivery

1. Setup + Foundational → service boots, health endpoints work
2. + US1 → bookings can be made (MVP)
3. + US2 → staff can see the schedule (both P1 stories done: usable product)
4. + US3 → rescheduling and corrections
5. + US4 → cancellations
6. Polish → production-mode checks, performance, docs, full quickstart run

---

## Notes

- [P] = different files, no dependency on incomplete tasks
- Commit after each task or logical group; schema (`.graphqls`) and migration changes are called
  out in the PR description (constitution workflow)
- Merged migrations are never edited; corrections are new migrations
- Stop at any checkpoint to validate the stories completed so far
