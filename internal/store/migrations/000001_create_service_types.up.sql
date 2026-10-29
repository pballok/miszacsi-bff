CREATE TABLE service_types (
    code             text PRIMARY KEY CHECK (code ~ '^[a-z][a-z0-9_]{0,49}$'),
    display_name     text NOT NULL CHECK (length(display_name) BETWEEN 1 AND 100),
    duration_minutes integer NOT NULL CHECK (duration_minutes > 0 AND duration_minutes % 60 = 0),
    offered          boolean NOT NULL DEFAULT true
);

INSERT INTO service_types (code, display_name, duration_minutes, offered) VALUES ('carwash', 'Carwash', 60, true);
