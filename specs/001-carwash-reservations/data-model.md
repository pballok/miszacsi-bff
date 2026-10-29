# Data Model: Carwash Reservations

**Feature**: `001-carwash-reservations` | **Date**: 2026-10-20 | **Plan**: [plan.md](./plan.md)

Two tables. All timestamps are `timestamptz` (absolute instants); the API exchanges them as
RFC 3339 with an explicit offset.

## Entity: ServiceType (`service_types`)

| Column | Type | Constraints | Notes |
|---|---|---|---|
| `code` | `text` | PK, `CHECK (code ~ '^[a-z][a-z0-9_]{0,49}$')` | stable code, e.g. `carwash` |
| `display_name` | `text` | `NOT NULL`, `CHECK (length(display_name) BETWEEN 1 AND 100)` | |
| `duration_minutes` | `integer` | `NOT NULL`, `CHECK (duration_minutes > 0 AND duration_minutes % 60 = 0)` | carwash = 60 |
| `offered` | `boolean` | `NOT NULL DEFAULT true` | `false` = retired: kept by existing reservations, rejected for new ones |

Seed (in the same migration): `('carwash', 'Carwash', 60, true)`.

Duration must be a multiple of 60 so every slot starts and ends on the hourly grid.
Changing a type's duration does not rewrite existing reservations: `ends_at` is fixed when a
reservation is written (see below).

## Entity: Reservation (`reservations`)

| Column | Type | Constraints | Notes |
|---|---|---|---|
| `id` | `uuid` | PK, `DEFAULT uuidv7()` | exposed as GraphQL `ID` |
| `starts_at` | `timestamptz` | `NOT NULL`, `CHECK (extract(epoch FROM starts_at)::numeric % 3600 = 0)` | hourly grid (minute 0), zero seconds |
| `ends_at` | `timestamptz` | `NOT NULL`, `CHECK (ends_at > starts_at)` | `starts_at + service duration`, set by the service on create and whenever `starts_at` or `service_type` changes |
| `customer_name` | `text` | `NOT NULL`, `CHECK (length(customer_name) BETWEEN 1 AND 200)` | trimmed |
| `customer_email` | `text` | `NOT NULL`, `CHECK (length(customer_email) BETWEEN 3 AND 254)` | trimmed; format validated in service |
| `customer_phone` | `text` | `NOT NULL`, `CHECK (length(customer_phone) BETWEEN 7 AND 40)` | trimmed; digit count validated in service |
| `service_type` | `text` | `NOT NULL`, FK → `service_types(code)` `ON UPDATE RESTRICT ON DELETE RESTRICT` | |
| `cancelled_at` | `timestamptz` | `NULL`, `CHECK (cancelled_at IS NULL OR cancelled_at >= created_at)` | `NULL` = active; set to PostgreSQL `now()` on cancel |
| `created_at` | `timestamptz` | `NOT NULL DEFAULT now()` | never supplied by the service |
| `updated_at` | `timestamptz` | `NOT NULL DEFAULT now()` | set to PostgreSQL `now()` in the SQL of every update/cancel |

**Time source**: `created_at`, `updated_at` and `cancelled_at` are always written with
PostgreSQL `now()` inside the SQL statement, never from a value the service passes in, so the
`cancelled_at >= created_at` check cannot fail because the application and database clocks
disagree. The service's injected clock is used only to validate requests (`startsAt` in the
future, "already started" on update).

### Constraints and indexes

- `reservations_no_overlap`:
  `EXCLUDE USING gist (tstzrange(starts_at, ends_at, '[)') WITH &&) WHERE (cancelled_at IS NULL)`
  — enforces FR-009 including simultaneous requests; violation SQLSTATE `23P01` → slot conflict.
- `reservations_active_starts_at_idx`: `btree (starts_at, id) WHERE cancelled_at IS NULL` —
  serves period listing in `(starts_at, id)` order (FR-005, SC-002).
- FK on `service_type` is indexed implicitly enough for this scale (one type); no extra index.

### Derived fields

- `status`: `ACTIVE` when `cancelled_at IS NULL`, else `CANCELLED`.
- `endsAt` is exposed read-only in GraphQL so the frontend can render the slot.

## State transitions

```text
            create (valid, future, on grid, no overlap)
  (none) ───────────────────────────────────────────▶ ACTIVE
                                                       │  ▲
                        update (rules below)           │  │
                                                       ▼──┘
                                                       │
                        delete                         │
                                                       ▼
                                                   CANCELLED   (terminal)
```

- **ACTIVE → ACTIVE (update)**: any subset of fields. If the reservation's `starts_at` is already
  in the past, only `customer_name`, `customer_email`, `customer_phone` may change. A new
  `starts_at` must be in the future and on the grid. A new `service_type` must be offered.
  Changing `starts_at` or `service_type` recomputes `ends_at`; the exclusion constraint
  re-checks overlap (excluding nothing special: the row's own old range is replaced in the same
  statement, so it never conflicts with itself).
- **ACTIVE → CANCELLED (delete)**: sets `cancelled_at = now()`, `updated_at = now()` (PostgreSQL clock).
- **CANCELLED**: terminal. Update and delete return "not found"; fetch-by-ID still returns it.
- Past, non-cancelled reservations stay `ACTIVE` (no automatic transition).

## Validation rules (service layer)

Applied after trimming; all failures collected and returned together. Codes are the
`ValidationErrorCode` enum in the [GraphQL schema](../../internal/graph/schema/schema.graphqls).

| Field | Rule | Code |
|---|---|---|
| `id` (fetch/update/delete) | parses as a UUID | `INVALID_FORMAT` (mutations: `ValidationError` in the payload; `reservation(id)` query: `BAD_USER_INPUT`, field `id`) |
| `customerName` | non-empty | `REQUIRED` |
| `customerName` | ≤ 200 code points | `TOO_LONG` |
| `customerEmail` | non-empty | `REQUIRED` |
| `customerEmail` | ≤ 254 code points (same count as PostgreSQL `length()`) | `TOO_LONG` |
| `customerEmail` | `net/mail.ParseAddress` succeeds, parsed address equals input, domain has a dot | `INVALID_FORMAT` |
| `customerPhone` | non-empty | `REQUIRED` |
| `customerPhone` | only digits, spaces, `( ) . / -`, optional leading `+`; 7–15 digits | `INVALID_FORMAT` |
| `startsAt` | strictly after now | `IN_PAST` |
| `startsAt` | minute 0, seconds and nanoseconds 0 (UTC instant) | `NOT_ON_GRID` |
| `serviceType` | exists and `offered` (on update: only when changed) | `UNKNOWN_SERVICE_TYPE` |
| `startsAt`, `serviceType` | change attempted on a reservation that already started | `NOT_ALLOWED` |

On update, "changed" means supplied **and** different from the locked row's current value
(`startsAt` compared as instants, `serviceType` by code, customer fields after trimming). A
supplied value equal to the current one is treated as omitted: it is not validated and never
triggers `IN_PAST`, `UNKNOWN_SERVICE_TYPE` or `NOT_ALLOWED`.

Listing arguments: `to` must be after `from` (`BAD_USER_INPUT`, field `to`). There are no paging
arguments.

## Listing limit

A listing returns at most `MaxListResults = 1000` reservations (active only; cancelled rows do not
count). The store fetches up to 1001 rows; if it gets 1001, the service returns
`ErrTooManyResults` and the `reservations` query fails with `extensions.code = "TOO_MANY_RESULTS"`
and no data. Exactly 1000 matches is still a success.
