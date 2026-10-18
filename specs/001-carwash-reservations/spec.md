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
2. **Given** a create request with a missing name, an invalid email, an invalid phone number, or
   an unknown service type, **When** it is submitted, **Then** nothing is stored and the response
   identifies each invalid field.
3. **Given** a create request for a date and time in the past, **When** it is submitted, **Then**
   it is rejected with a clear validation error.
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
5. **Given** more reservations in the period than fit in one page, **When** the period is listed,
   **Then** results are returned in pages and every reservation can be reached by following pages.

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

---

### Edge Cases

- Reservation exactly on the start or end boundary of a listing period: start is inclusive, end is
  exclusive, so adjacent periods never return the same reservation twice.
- Time zones: date/times are exchanged with an explicit offset and stored as absolute instants;
  listing a period returns the same results regardless of the caller's time zone.
- Very large listing periods: results are paginated rather than returned all at once.
- Leading/trailing whitespace in name, email or phone is trimmed before validation; email
  comparison is case-insensitive.
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
  address format (max 254 characters); phone number in international format (optional leading `+`,
  7–15 digits, common separators allowed); date/time in the future at creation; service type one
  of the currently offered types.
- **FR-004**: System MUST support a list of service types that currently contains only "carwash"
  and can be extended later without changing how reservations are created, listed, updated or
  deleted.
- **FR-005**: System MUST allow listing all active reservations whose date/time falls within a
  requested period (start inclusive, end exclusive), ordered by date/time ascending, with
  pagination.
- **FR-006**: System MUST allow updating any subset of a reservation's fields, applying the same
  validation as creation to the changed fields.
- **FR-007**: System MUST allow deleting a reservation by identifier. Deletion is a soft delete:
  the reservation is marked cancelled (with the time of cancellation) and retained for history,
  but is never returned by active listings, cannot be updated, and cannot be deleted again.
  "Active" means "not cancelled"; past reservations that were not cancelled remain active.
- **FR-008**: Create, update, delete and list MUST each be offered as separate operations.
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

### Key Entities

- **Reservation**: a booked appointment. Attributes: identifier, date and time (absolute instant),
  customer name, customer email, customer phone number, service type, status (active or
  cancelled), cancellation time, created and last-updated timestamps. Its end time is derived from
  the service type's duration.
- **Service Type**: a kind of service that can be reserved. Attributes: stable code (e.g.
  `carwash`), display name, duration (carwash: 60 minutes), whether it is currently offered. Initially the only entry is
  "carwash"; more will be added later. Each reservation references exactly one service type.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A valid reservation can be created with a single request and appears in the listing
  for its period immediately afterwards (100% of cases).
- **SC-002**: Listing a one-week period with up to 500 reservations returns the first page in under
  1 second for the person using the frontend.
- **SC-003**: 100% of invalid inputs (per FR-003) are rejected with a message naming the offending
  field, and no invalid reservation is ever stored.
- **SC-004**: Zero reservations are lost across service restarts or redeployments.
- **SC-005**: Adding a second service type requires no change to the create, list, update or delete
  operations as seen by the frontend, other than the new value becoming accepted.

## Assumptions

- The consumer of these operations is the MiSzaCsi React frontend; no other clients are in scope.
- A single carwash location is served; multi-location support is out of scope.
- Payments, customer notifications (email/SMS confirmations) and reminders are out of scope.
- Opening hours / business-hour restrictions on reservation times are out of scope for this
  feature; any future date/time is accepted (subject to FR-009).
- Customers are not separate accounts; customer details are stored on each reservation.
- Adding a new service type is an operator/data change, not something exposed through these
  operations.
- Retention of past and cancelled reservations follows normal practice for a small business;
  automatic purging is out of scope.
- Until authentication is added, the API is deployed so that only the MiSzaCsi frontend (or a
  trusted network) can reach it; this is an accepted risk for customer personal data.
