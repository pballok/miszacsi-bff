<!--
Sync Impact Report
==================
Version change: 1.0.0 → 2.0.0
Bump rationale: MAJOR — Principle II's list-field rule is redefined incompatibly: the mandatory
Relay-style cursor pagination is removed; list fields now return plain lists with a hard
maximum and fail with a typed error when it is exceeded.

Modified principles:
- II. Schema-First GraphQL — "Relay-style cursor connections" bullet replaced by "no pagination,
  bounded plain lists"; rationale no longer cites pagination.
- III. PostgreSQL as the Single Source of Truth — index bullet says "orderings" instead of
  "pagination orderings".

Added sections: none
Removed sections: none

Templates reviewed (not modified; they read the constitution at runtime):
- .specify/templates/plan-template.md — "Constitution Check" gate derives its gates from this file ✅
- .specify/templates/spec-template.md — no constitution-specific content to align ✅
- .specify/templates/tasks-template.md — no pagination-specific content ✅

Follow-up for existing work: specs/001-carwash-reservations (spec, plan, research, data-model,
quickstart, tasks) and the implemented GraphQL schema updated to the bounded-list rule in the
same change.

Deferred TODOs: none
-->

# MiSzaCsi BFF Constitution

The MiSzaCsi BFF is a Go backend-for-frontend that serves the MiSzaCsi React application through a
GraphQL API, backed by PostgreSQL. This constitution defines the non-negotiable rules for how it is
designed, built, tested, and shipped.

## Core Principles

### I. Modern, Idiomatic Go

- The service MUST target the latest stable Go release; the `go` directive in `go.mod` MUST be
  raised to a new stable release within one minor release cycle of its availability.
- The standard library MUST be preferred. A third-party dependency MAY be added only when the
  standard library has no reasonable equivalent, and the reason MUST be stated in the PR or plan.
  Dependencies mandated by this constitution (gqlgen, pgx, sqlc, golang-migrate, testcontainers)
  are pre-approved.
- Code MUST be `gofmt`-formatted and pass `go vet` and `golangci-lint` (with the repository's
  committed configuration) with zero findings. Lint suppressions MUST carry a justification comment.
- Every function that performs I/O, blocks, or calls something that does MUST accept a
  `context.Context` as its first parameter and propagate it; `context.Background()` /
  `context.TODO()` are allowed only in `main`, tests, and top-level background workers.
- Errors MUST be returned, not panicked, and MUST be wrapped with context using
  `fmt.Errorf("...: %w", err)`; sentinel or typed errors are inspected with `errors.Is` /
  `errors.As`. Errors MUST NOT be both logged and returned at the same layer.
- Logging MUST use `log/slog` with structured key/value attributes; the logger is injected, not
  fetched from a package global. Request-scoped attributes (e.g. request ID) MUST be carried via
  context.
- There MUST be no mutable global state: no package-level mutable variables, no `init()` side
  effects beyond registration required by a library. Dependencies are wired explicitly in `main`
  and passed via constructors.

**Rationale**: explicit dependencies, context, and errors make the service testable, observable,
and safe to cancel; tooling-enforced style removes review noise.

### II. Schema-First GraphQL

- The GraphQL schema (`*.graphqls` files) is the contract with the frontend and MUST be changed
  first; resolver and model code MUST be generated from it with gqlgen. Generated code MUST NOT be
  edited by hand and MUST be regenerated and committed in the same change as the schema edit.
- The API MUST conform to the GraphQL specification (October 2021 edition).
- List fields MUST NOT be paginated: they return a plain list (`[Type!]!`), not a connection.
  Every list field that can grow MUST have a documented maximum result count (default 1000); when
  more items match, the field MUST return no items and fail with a typed, client-actionable error
  (stable `extensions.code`, e.g. `TOO_MANY_RESULTS`) instead of silently truncating. Clients
  narrow the request (e.g. a shorter period) to get under the limit.
- Every mutation MUST take a single dedicated input type (`<MutationName>Input`) and return a
  dedicated payload type.
- Expected, client-actionable failures (validation, not-found, conflict, unauthorized) MUST be
  typed: either modelled in the schema (e.g. union/`errors` field in the payload) or exposed via a
  stable `extensions.code`. Internal errors MUST be masked from clients and logged server-side.
- Resolvers that load related entities MUST batch through a per-request DataLoader; N+1 query
  patterns are a defect.
- The server MUST enforce query depth and complexity limits, configured via environment variables
  with safe defaults. Introspection and the playground MUST be disabled in production
  configuration.
- Breaking schema changes (removing/renaming fields, tightening nullability, changing argument
  types) MUST go through `@deprecated` first and be coordinated with the frontend.

**Rationale**: a schema-first contract lets frontend and backend evolve in parallel; bounded lists,
batching, and limits keep the API predictable and safe under real client usage.

### III. PostgreSQL as the Single Source of Truth

- All persistent state and runtime-mutable configuration MUST live in PostgreSQL; the service
  process MUST be stateless (no on-disk state, no in-memory state that must survive restarts).
- The target is the latest stable PostgreSQL major release. SQL MUST be standard PostgreSQL SQL;
  extensions require justification in the plan.
- Schema changes MUST be versioned migrations managed by golang-migrate, each with an `up` and a
  `down` file. Migrations MUST be applied automatically on startup before the server accepts
  traffic. A migration that has been merged MUST NOT be edited; corrections are new migrations.
- Queries MUST be written as SQL and compiled to type-safe Go with sqlc using the pgx driver
  (`pgx/v5`, `pgxpool`). ORMs and ad-hoc string-built SQL are prohibited.
- Any write that touches more than one row or table as a logical unit MUST run in a single
  transaction.
- Tables MUST declare primary keys, `NOT NULL` where applicable, foreign keys, unique and check
  constraints that express domain invariants, and indexes supporting the queries and
  orderings actually used.

**Rationale**: one authoritative, constraint-enforced store with reviewable, reversible
migrations and compile-time-checked queries prevents drift and runtime surprises.

### IV. Containerised by Default

- The service MUST be built by a multi-stage Dockerfile whose final stage is a minimal image
  (distroless or scratch-class) running as a non-root user, containing only the binary and
  required runtime assets.
- A `docker compose` file MUST run the BFF and PostgreSQL as separate containers for local
  development; PostgreSQL data MUST use a named volume.
- All configuration MUST come from environment variables (12-factor). Required variables MUST be
  validated at startup and the process MUST fail fast with a clear message if any are missing or
  invalid.
- The service MUST expose a liveness endpoint (process up) and a readiness endpoint (database
  reachable and migrations applied), separate from the GraphQL endpoint.
- On `SIGTERM`/`SIGINT` the service MUST shut down gracefully: mark itself not-ready, stop
  accepting new connections, drain in-flight requests within a configurable timeout, then close
  the database pool.

**Rationale**: identical, minimal, non-root images and env-driven config make every environment
reproducible and safe to orchestrate.

### V. Testing Discipline

- TDD is not required, but every package containing logic MUST have unit tests. Unit tests MUST use
  the standard `testing` package and be table-driven where more than one case exists.
- All tests MUST pass under the race detector (`go test -race ./...`).
- The data layer (sqlc queries, transactions, migrations) MUST have integration tests against a
  real PostgreSQL started with testcontainers, using the same major version as production.
  Mocking the database in place of these tests is not acceptable.
- Migrations MUST be tested up and down.
- The full test suite MUST pass before any change is merged; a failing or skipped-without-reason
  test blocks the merge.

**Rationale**: fast table-driven unit tests plus real-database integration tests catch both logic
and SQL/contract defects without the cost of mandatory TDD ceremony.

### VI. Security and Simplicity

- All external input (GraphQL arguments, headers, environment variables) MUST be validated at the
  boundary before use; validation failures return typed client errors (Principle II).
- Secrets MUST NOT appear in source code, committed files, container images, or logs. They are
  supplied only via environment variables or the runtime's secret mechanism; `.env` files with real
  values MUST be git-ignored.
- CORS MUST be restricted to the configured frontend origin(s); wildcard origins are prohibited
  outside local development.
- The simplest design that satisfies the current requirement MUST be chosen (YAGNI). Additional
  layers, abstractions, services, or dependencies MUST be justified in the plan's Complexity
  Tracking table.

**Rationale**: a small attack surface and a small codebase are easier to secure, reason about,
and change.

## Technology Stack & Constraints

| Concern | Mandated choice |
|---|---|
| Language | Go, latest stable release |
| API | GraphQL (Oct 2021 spec), gqlgen, schema-first |
| Database | PostgreSQL, latest stable major |
| DB access | sqlc + pgx/v5 (`pgxpool`); no ORM |
| Migrations | golang-migrate, up/down, applied on startup |
| Logging | `log/slog`, structured (JSON in containers) |
| Linting | gofmt, go vet, golangci-lint |
| Testing | `testing` (table-driven), `-race`, testcontainers for PostgreSQL |
| Packaging | Multi-stage Dockerfile, minimal non-root image, docker compose |
| Config | Environment variables only |

Replacing any mandated choice is a constitutional amendment (see Governance).

## Development Workflow & Quality Gates

- Features follow the Spec Kit flow: specify → (clarify) → plan → tasks → implement.
- Every plan MUST complete the **Constitution Check** gate before Phase 0 research and re-check it
  after Phase 1 design, listing each principle (I–VI) as pass or justified exception.
- Before merge, a change MUST pass: `gofmt` check, `go vet`, `golangci-lint`,
  `go generate`/gqlgen and `sqlc generate` producing no diff, `go test -race ./...` including
  integration tests, and a successful container image build.
- Schema (`.graphqls`) and migration changes MUST be called out explicitly in the PR description.
- Code review MUST verify compliance with this constitution; reviewers MUST reject changes that
  violate a MUST without a recorded, justified exception.

## Governance

- **Supremacy**: this constitution supersedes other project practices and guidance. Where a
  guidance document conflicts with it, the constitution wins and the document is corrected.
- **Amendments**: proposed via a PR that edits `.specify/memory/constitution.md` (normally through
  `/speckit-constitution`), states the motivation, the version bump, and any migration needed for
  existing code. An amendment takes effect when the PR is merged by a project maintainer. Existing
  code that becomes non-compliant MUST be tracked as follow-up work.
- **Versioning**: the constitution uses semantic versioning:
  - MAJOR — removal or backward-incompatible redefinition of a principle or governance rule;
  - MINOR — a new principle or section, or materially expanded guidance;
  - PATCH — clarifications, wording, and typo fixes with no change in meaning.
  `Last Amended` is updated on every change; `Ratified` never changes.
- **Compliance during planning**: `/speckit-plan` MUST evaluate the Constitution Check gate; any
  violation MUST be recorded in Complexity Tracking with why it is needed and why the simpler,
  compliant alternative was rejected. Unjustified violations block the plan.
- **Compliance during review**: every PR review MUST confirm the Development Workflow quality gates
  passed and that no principle is violated without a recorded exception. `/speckit-analyze` treats
  constitution conflicts as CRITICAL findings.
- **Periodic review**: the constitution SHOULD be reviewed when a new Go or PostgreSQL major/minor
  release ships, or at least every six months, to keep the mandated stack current.

**Version**: 2.0.0 | **Ratified**: 2026-10-18 | **Last Amended**: 2026-10-29
