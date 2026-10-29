# Quickstart & Validation: Carwash Reservations

**Feature**: `001-carwash-reservations` | **Plan**: [plan.md](./plan.md) |
**Contract**: [internal/graph/schema/schema.graphqls](../../internal/graph/schema/schema.graphqls) |
**Data model**: [data-model.md](./data-model.md)

This guide proves the feature works end to end. Each scenario maps to a spec acceptance
scenario; the automated API tests in `internal/graph` cover the same list.

## Prerequisites

- Docker with the compose plugin
- Go 1.27 (for tests and code generation)
- `sqlc` and `gqlgen` are run through `go tool` (declared as tool dependencies in `go.mod`)
- `golangci-lint` on `PATH`

## Run locally

```sh
docker compose up --build          # postgres:19 (named volume pgdata) + bff, APP_ENV=development
curl -fsS localhost:8080/livez     # 200
curl -fsS localhost:8080/readyz    # 200 once the DB is reachable and migrations ran
```

Open the playground at <http://localhost:8080/playground> (development only).

Reset all data: `docker compose down -v` (removes the `pgdata` volume).

## Quality gates (must all pass before merge)

```sh
gofmt -l .                         # no output
go vet ./...
golangci-lint run
go generate ./... && git diff --exit-code   # gqlgen + sqlc output committed
go test -race ./...                # unit + testcontainers integration + API tests
docker build -t miszacsi-bff .
```

## Validation scenarios

Use dates in the future; examples use `2030-01-15` with offset `+01:00`. Send requests to
`POST /graphql` (or the playground). Times in the table are local (`+01:00`); responses return
the same instants in UTC, e.g. `endsAt` 11:00 appears as `2030-01-15T10:00:00Z`.

| # | Action | Expected | Spec |
|---|---|---|---|
| 1 | `createReservation` at `10:00`, valid name/email/phone, `serviceType: "carwash"` | `reservation` with `id`, `status: ACTIVE`, `endsAt` = `11:00`; `errors: []` | US1-1 |
| 2 | `createReservation` with `customerName: ""`, `customerEmail: "x"`, `customerPhone: "12"`, `serviceType: "polish"` | `reservation: null`; four `ValidationError`s (`REQUIRED`, `INVALID_FORMAT`, `INVALID_FORMAT`, `UNKNOWN_SERVICE_TYPE`) | US1-2 |
| 3 | `createReservation` at `2020-01-01T10:00:00Z`; then at `…T10:30:00+01:00` | `IN_PAST`; then `NOT_ON_GRID` on `startsAt` | US1-3 |
| 4 | `createReservation` at `10:00` again (same slot as #1) | `SlotConflictError`, nothing stored | US1-4 |
| 5 | `createReservation` at `11:00` | succeeds (back-to-back) | US1-5 |
| 6 | Restart: `docker compose restart bff`, then `reservation(id)` for #1 | same details returned | US1 independent test, SC-004 |
| 7 | Create `09:00`, `15:00` and next day `09:00`; `reservations(from: 08:00, to: 12:00)` | exactly `09:00`, `10:00`, `11:00` in order | US2-1 |
| 8 | `reservations` for an empty day | `reservations: []`, no errors | US2-2 |
| 9 | `reservations` with `to` before `from` | GraphQL error `BAD_USER_INPUT`, `field: "to"` | US2-3 |
| 10 | Seed 1001 active reservations on consecutive hours from `2031-01-01T00:00:00Z` with SQL (`docker compose exec postgres psql …`, `generate_series`); list the first 1000 hours, then all 1001 | 1000 reservations; then GraphQL error `TOO_MANY_RESULTS`, `data: null` | US2-5, US2-6 |
| 11 | `updateReservation` #1: `startsAt: 13:00`, `customerPhone: "+36 30 123 4567"` | new values; other fields unchanged; `endsAt` 14:00; `updatedAt` advanced | US3-1 |
| 12 | `updateReservation` with `customerEmail: "bad"` | `ValidationError`; stored row unchanged | US3-2 |
| 13 | `updateReservation` with an unknown UUID | `NotFoundError` | US3-3 |
| 14 | `updateReservation` #1 to `11:00` (taken by #5) | `SlotConflictError`; unchanged | US3-4 |
| 15 | `deleteReservation` #5; list the day; create at `11:00` | `status: CANCELLED`; not listed; new create succeeds | US4-1 |
| 16 | `deleteReservation` #5 again | `NotFoundError` | US4-2 |
| 17 | `reservation(id)` for #5 | returned with `status: CANCELLED`, `cancelledAt` set | US4-3 |
| 18 | `reservation(id)` with an unknown UUID | GraphQL error `NOT_FOUND` | FR-013 |
| 19 | Fire 10 parallel `createReservation` for the same new slot | exactly 1 success, 9 `SlotConflictError` | Edge case, FR-009 |
| 20 | Production config (`APP_ENV=production`): introspection query; `GET /playground` | introspection rejected; 404 | Constitution II |

### Example request (scenario 1)

```graphql
mutation {
  createReservation(input: {
    startsAt: "2030-01-15T10:00:00+01:00"
    customerName: "Kovács Anna"
    customerEmail: "anna@example.hu"
    customerPhone: "+36 30 123 4567"
    serviceType: "carwash"
  }) {
    reservation { id startsAt endsAt status serviceType { code durationMinutes } }
    errors { __typename message ... on ValidationError { field code } }
  }
}
```

## Performance check (SC-002)

With 60-minute slots and no overlap, one week holds at most 168 active reservations, so the
integration suite seeds 500 rows in one week (168 active, 332 cancelled) and asserts that the
`reservations` listing for that week returns in under 1 second through the HTTP handler.
