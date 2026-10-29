CREATE TABLE reservations (
    id             uuid PRIMARY KEY DEFAULT uuidv7(),
    starts_at      timestamptz NOT NULL CHECK (extract(epoch FROM starts_at)::numeric % 3600 = 0),
    ends_at        timestamptz NOT NULL CHECK (ends_at > starts_at),
    customer_name  text NOT NULL CHECK (length(customer_name) BETWEEN 1 AND 200),
    customer_email text NOT NULL CHECK (length(customer_email) BETWEEN 3 AND 254),
    customer_phone text NOT NULL CHECK (length(customer_phone) BETWEEN 7 AND 40),
    service_type   text NOT NULL REFERENCES service_types(code) ON UPDATE RESTRICT ON DELETE RESTRICT,
    cancelled_at   timestamptz NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    CHECK (cancelled_at IS NULL OR cancelled_at >= created_at),
    CONSTRAINT reservations_no_overlap EXCLUDE USING gist (tstzrange(starts_at, ends_at, '[)') WITH &&) WHERE (cancelled_at IS NULL)
);

CREATE INDEX reservations_active_starts_at_idx ON reservations (starts_at, id) WHERE cancelled_at IS NULL;
