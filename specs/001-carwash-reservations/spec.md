# Feature Specification: Carwash Reservations

**Feature Branch**: `001-carwash-reservations`

**Created**: 2026-10-18

**Status**: Draft

**Input**: User description: "The BFF should offer graphql endpoints to manage reservations for a small carwash service. Each reservation consist of a date and time, the name of the customer, their email and their phone number, plus the reserved service type. For now, only \"carwash\" can be reserved, but later the list of services will expand. The graphql endpoints must allow for listing all active reservations within a given time period. Creating, updating and deleting reservations must all be possible with their respective separate graphql endpoints. Reservations must be persisted within the database."

## Clarifications

### Session 2026-10-18

- Q: Should delete erase reservations or mark them cancelled, and what is "active"? → A: Soft
  delete; deleted reservations are marked cancelled and kept; "active" = not cancelled.
- Q: May active reservations overlap? → A: No; one car at a time, a carwash takes a fixed
  60 minutes.
- Q: Who may call these operations? → A: No authentication for now; the API is reachable only
  from the trusted frontend/network; authentication is a later feature.

### Session 2026-10-20

- Q: Should the API offer fetching a single reservation by identifier? → A: Yes; a separate
  fetch-by-identifier query returns the reservation whether active or cancelled (status included);
  "not found" only for identifiers that never existed.
- Q: Should listing include reservations that start before the period but overlap into it? → A:
  No; only reservations whose start time is within [start, end) are listed.
- Q: Must reservation start times align to a time grid? → A: Yes; whole hours only (minute
  :00), seconds and fractions must be zero. (Revised from an earlier 30-minute grid.)
- Q: Should the API let the frontend ask which service types can be booked? → A: No; the
  frontend keeps its own list of service-type codes; no service-type query in this feature.
- Q: What happens when a page request mixes forward and backward paging arguments? → A:
  *(Superseded on 2026-10-29: listing is no longer paginated.)*
- Q: Should customer emails be stored as entered or lowercased? → A: Stored as entered (only
  trimmed); no feature compares emails yet, so the case-insensitive comparison rule is dropped.
- Q: Should national-format phone numbers be accepted as well as international ones? → A: Yes;
  optional leading `+`, 7–15 digits, common separators; stored as entered, not normalized.
- Q: Does a create request that leaves a required field out get a per-field validation error?
  → A: No; "missing" means empty or whitespace-only (per-field `REQUIRED`); a field left out
  entirely is rejected as a malformed request, since all create fields stay required.

### Session 2026-10-29

- Q: Should listing be paginated? → A: No; a listing returns all matching reservations in one
  response, up to 1000. If more than 1000 active reservations start in the period, the request
  fails with a "too many results" error and returns no reservations; the frontend asks for a
  shorter period. (Replaces the earlier forward/backward page-based listing.)

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Create a reservation (Priority: P1)

A reservation for a carwash is recorded with a date and time, the customer's name, email address,
phone number, and the service type. The new reservation is stored permanently and its details,
including a unique identifier, are returned.

**Why this priority**: without the ability to create reservations, nothing else in the feature has
data to work with. Creating reservations alone already replaces a paper/phone booking book.

**Independent Test**: create a reservation with valid details, restart the service, and confirm
the reservation still exists with the same details.

**Acceptance Scenarios**:

1. **Given** no reservations exist, **When** a reservation is created for a future date and time
   with name, valid email, valid phone number and service type "carwash", **Then** the reservation
   is stored and returned with a unique identifier and all submitted details.
2. **Given** a create request with an empty (or whitespace-only) name, an invalid email, an
   invalid phone number, or an unknown service type, **When** it is submitted, **Then** nothing is
   stored and the response identifies each invalid field. (A request that leaves a required
   field out entirely is rejected as a malformed request before field validation.)
3. **Given** a create request for a date and time in the past, or one not on the hourly grid
   (e.g. 10:30, 10:15 or 10:00:30), **When** it is submitted, **Then** it is rejected with a clear
   validation error.
4. **Given** an existing active reservation at a date and time, **When** another reservation is
   created for a time overlapping its 60-minute slot, **Then** it is rejected with a slot-conflict
   error and nothing is stored (FR-009).
5. **Given** an active reservation from 10:00 to 11:00, **When** a reservation is created for
   11:00, **Then** it succeeds (back-to-back is allowed).

---

### User Story 2 - List active reservations in a time period (Priority: P1)

The frontend asks for all active reservations between a start and an end date/time (for example,
"today" or "this week") and receives them ordered by reservation time, so staff can see what is
coming up.

**Why this priority**: viewing upcoming bookings is the core daily use of the system; created
reservations are useless if nobody can see them.

**Independent Test**: create several reservations inside and outside a period, request the period,
and verify exactly the active ones inside it are returned in chronological order.

**Acceptance Scenarios**:

1. **Given** reservations at 09:00, 11:00 and 15:00 on a day and one on the next day, **When**
   active reservations from 08:00 to 12:00 on that day are requested, **Then** exactly the 09:00 and
   11:00 reservations are returned, in that order.
2. **Given** a period containing no reservations, **When** it is requested, **Then** an empty list
   is returned (not an error).
3. **Given** a period whose end is before its start, **When** it is requested, **Then** a
   validation error is returned.
4. **Given** a deleted reservation inside the requested period, **When** the period is listed,
   **Then** the deleted reservation is not included.
5. **Given** exactly 1000 active reservations in the period, **When** the period is listed,
   **Then** all 1000 are returned in one response.
6. **Given** more than 1000 active reservations in the period, **When** the period is listed,
   **Then** no reservations are returned and a "too many results" error tells the caller to
   request a shorter period.

---

### User Story 3 - Update a reservation (Priority: P2)

An existing reservation's date and time, customer details, or service type is changed, for example
when a customer calls to move their appointment or correct their phone number.

**Why this priority**: rescheduling and corrections are common but the system is usable without
them (delete and re-create is a workaround).

**Independent Test**: create a reservation, update its time and phone number, then fetch/list it
and verify only the changed fields differ.

**Acceptance Scenarios**:

1. **Given** an active reservation, **When** its date/time and phone number are updated with valid
   values, **Then** the stored reservation reflects the new values and unchanged fields keep their
   previous values.
2. **Given** an update with invalid values, **When** it is submitted, **Then** nothing changes and
   the response identifies each invalid field.
3. **Given** an identifier that does not exist or refers to a deleted reservation, **When** an
   update is submitted, **Then** a "not found" error is returned.
4. **Given** an update moving a reservation to a time that conflicts with another active
   reservation, **When** it is submitted, **Then** it is rejected with a slot-conflict error and
   nothing changes (FR-009).

---

### User Story 4 - Delete a reservation (Priority: P2)

A reservation that will not take place (customer cancels) is deleted so it no longer appears in
listings and its time slot becomes free.

**Why this priority**: cancellations must be handled, but the core booking flow works without it.

**Independent Test**: create a reservation, delete it, then list its period and verify it is gone.

**Acceptance Scenarios**:

1. **Given** an active reservation, **When** it is deleted, **Then** it no longer appears in
   active-reservation listings, is retained as cancelled, and its time becomes available for new
   reservations.
2. **Given** an identifier that does not exist or was already deleted, **When** delete is
   requested, **Then** a "not found" error is returned and nothing changes.
3. **Given** a deleted reservation, **When** it is fetched by its identifier, **Then** it is
   returned with status cancelled and its cancellation time (FR-013).

---

### Edge Cases

- Reservation exactly on the start or end boundary of a listing period: start is inclusive, end is
  exclusive, so adjacent periods never return the same reservation twice.
- Reservation starting before a listing period but running into it (e.g. 08:00 when listing from
  08:30): not included; listing is by start time only.
- Time zones: date/times are exchanged with an explicit offset and stored as absolute instants;
  listing a period returns the same results regardless of the caller's time zone. Returned
  date/times are always expressed in UTC (`Z` offset); the frontend converts them for display. The hourly
  grid (FR-003) is checked on the absolute instant (UTC), which matches local :00 for any
  whole-hour offset.
- Very large listing periods: results are not paginated. Up to 1000 reservations are returned at
  once; a period with more than 1000 active reservations is rejected with a "too many results"
  error (no partial list), and the caller must request a shorter period. Cancelled reservations
  do not count toward the limit.
- Leading/trailing whitespace in name, email or phone is trimmed before validation; otherwise
  values are stored exactly as entered (email letter case is preserved, e.g. `Anna@Example.HU`
  is returned unchanged).
- Two create requests for the same or overlapping slot arriving simultaneously: exactly one
  succeeds, the other receives a slot-conflict error (FR-009).
- A reservation overlapping only a cancelled reservation: allowed.
- Updating a reservation whose time has already passed: allowed only for correcting customer
  details, not for moving it into the past.
- A service type that is later retired: existing reservations keep it; new reservations cannot use
  it.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST allow creating a reservation consisting of: date and time, customer name,
  customer email, customer phone number, and service type. All fields are required.
- **FR-002**: System MUST assign each reservation a unique, stable identifier and return the full
  reservation after create and update.
- **FR-003**: System MUST validate input: name non-empty (max 200 characters); email in a valid
  address format (max 254 characters); phone number in international or national format (optional
  leading `+`, 7–15 digits, separators space and `( ) . / -` allowed; stored as entered, not
  normalized); date/time in the future at creation and aligned to an
  hourly grid (minute :00, zero seconds and fractions); service type one of the
  currently offered types.
- **FR-004**: System MUST support a list of service types that currently contains only "carwash"
  and can be extended later without changing how reservations are created, listed, updated or
  deleted.
- **FR-005**: System MUST allow listing all active reservations whose start date/time falls within
  a requested period (start inclusive, end exclusive), ordered by date/time ascending, in a
  single response without pagination. A reservation that starts before the period is not listed,
  even if its slot runs into the period. At most 1000 reservations are returned; if more than
  1000 match, the request MUST fail with a distinct "too many results" error and return none.
- **FR-006**: System MUST allow updating any subset of a reservation's fields, applying the same
  validation as creation to the changed fields. A supplied value equal to the current one counts
  as unchanged, so resending a full reservation with only some fields edited is accepted.
- **FR-007**: System MUST allow deleting a reservation by identifier. Deletion is a soft delete:
  the reservation is marked cancelled (with the time of cancellation) and retained for history,
  but is never returned by active listings, cannot be updated, and cannot be deleted again.
  "Active" means "not cancelled"; past reservations that were not cancelled remain active.
- **FR-008**: Create, update, delete, list and fetch-by-identifier (FR-013) MUST each be offered
  as separate operations.
- **FR-009**: Active reservations MUST NOT overlap in time: the carwash serves one car at a time.
  Each service type has a fixed duration ("carwash" = 60 minutes); a reservation occupies
  [start, start + duration). A create or update that would overlap any other active reservation
  MUST be rejected with a slot-conflict error, including when requests arrive simultaneously.
  Back-to-back reservations (one ends exactly when the next starts) are allowed. Cancelled
  reservations never block a slot.
- **FR-010**: System MUST persist reservations durably so they survive service restarts.
- **FR-011**: System MUST return errors that let the caller distinguish validation errors (per
  field), "not found", slot conflicts, and unexpected failures; internal details MUST NOT be exposed.
- **FR-012**: No authentication is required for these operations in this feature. Protection of
  customer personal data relies on the service being reachable only from the trusted frontend /
  network and on cross-origin access being limited to the frontend origin. Adding authentication
  is a separate, later feature.
- **FR-013**: System MUST allow fetching a single reservation by its identifier. The result
  includes the reservation's status, so both active and cancelled reservations are returned;
  a "not found" error is returned only for identifiers that never existed.

### Key Entities

- **Reservation**: a booked appointment. Attributes: identifier, date and time (absolute instant),
  customer name, customer email, customer phone number, service type, status (active or
  cancelled), cancellation time, created and last-updated timestamps. Its end time is derived from
  the service type's duration.
- **Service Type**: a kind of service that can be reserved. Attributes: stable code (e.g.
  `carwash`), display name, duration (carwash: 60 minutes), whether it is currently offered. Initially the only entry is
  "carwash"; more will be added later. Each reservation references exactly one service type.
- **Terms**: "delete" and "cancel" name the same action (the soft delete of FR-007; the
  reservation's status becomes cancelled). The listing period's start and end are also called
  "from" and "to".

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A valid reservation can be created with a single request and appears in the listing
  for its period immediately afterwards (100% of cases).
- **SC-002**: Listing a one-week period containing 500 stored reservations (at most 168 of them
  active, since slots cannot overlap; the rest cancelled) returns the full listing in under 1 second,
  measured at the BFF's HTTP endpoint.
- **SC-003**: 100% of invalid inputs (per FR-003) are rejected with a message naming the offending
  field, and no invalid reservation is ever stored.
- **SC-004**: Zero reservations are lost across service restarts or redeployments.
- **SC-005**: Adding a second service type requires no change to the create, list, update or delete
  operations as seen by the frontend, other than the new value becoming accepted. (The
  frontend's own list of selectable codes is updated separately; see Assumptions.)

## Assumptions

- The consumer of these operations is the MiSzaCsi React frontend; no other clients are in scope.
- A single carwash location is served; multi-location support is out of scope.
- Payments, customer notifications (email/SMS confirmations) and reminders are out of scope.
- Opening hours / business-hour restrictions on reservation times are out of scope for this
  feature; any future date/time on the hourly grid is accepted (subject to FR-003 and FR-009).
- Customers are not separate accounts; customer details are stored on each reservation.
- Adding a new service type is an operator/data change, not something exposed through these
  operations. The API offers no way to list service types; the frontend keeps its own list of
  selectable codes and must be updated when a type is added or retired (accepted limitation).
- Retention of past and cancelled reservations follows normal practice for a small business;
  automatic purging is out of scope.
- Until authentication is added, the API is deployed so that only the MiSzaCsi frontend (or a
  trusted network) can reach it; this is an accepted risk for customer personal data.
