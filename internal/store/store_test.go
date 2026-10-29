package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/pballok/miszacsi-bff/internal/service"
)

const (
	postgresImage    = "postgres:18"
	templateDatabase = "template_miszacsi"
)

// adminURL points at the container's maintenance database; set once by TestMain.
var adminURL string

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	ctx := context.Background()
	ctr, err := postgres.Run(ctx, postgresImage,
		postgres.WithDatabase("postgres"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.BasicWaitStrategies(),
	)
	defer func() {
		if err := testcontainers.TerminateContainer(ctr); err != nil {
			fmt.Fprintln(os.Stderr, "terminate postgres container:", err)
		}
	}()
	if err != nil {
		fmt.Fprintln(os.Stderr, "start postgres container:", err)
		return 1
	}
	if adminURL, err = ctr.ConnectionString(ctx, "sslmode=disable"); err != nil {
		fmt.Fprintln(os.Stderr, "connection string:", err)
		return 1
	}
	if err := createDatabase(ctx, templateDatabase, ""); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := Migrate(ctx, databaseURL(templateDatabase)); err != nil {
		fmt.Fprintln(os.Stderr, "migrate template database:", err)
		return 1
	}
	return m.Run()
}

// databaseURL returns the connection URL of the named database in the test container.
func databaseURL(name string) string {
	u, err := url.Parse(adminURL)
	if err != nil {
		panic(err) // adminURL comes from testcontainers and is always valid
	}
	u.Path = "/" + name
	return u.String()
}

func adminExec(ctx context.Context, sql string) error {
	conn, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		return fmt.Errorf("connect admin: %w", err)
	}
	defer conn.Close()
	if _, err := conn.Exec(ctx, sql); err != nil {
		return fmt.Errorf("exec %q: %w", sql, err)
	}
	return nil
}

func createDatabase(ctx context.Context, name, template string) error {
	sql := "CREATE DATABASE " + name
	if template != "" {
		sql += " TEMPLATE " + template
	}
	return adminExec(ctx, sql)
}

func randomName(t *testing.T) string {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return "t_" + hex.EncodeToString(b)
}

// newScratchDatabase creates an empty database, or a copy of template when it is non-empty,
// and drops it when the test ends.
func newScratchDatabase(t *testing.T, template string) string {
	t.Helper()
	name := randomName(t)
	if err := createDatabase(t.Context(), name, template); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := adminExec(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("drop database: %v", err)
		}
	})
	return databaseURL(name)
}

// newTestStore returns a Store on a fresh, fully migrated database.
func newTestStore(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), newScratchDatabase(t, templateDatabase))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return New(pool), pool
}

func TestMigrationsUpDownUp(t *testing.T) {
	ctx := t.Context()
	dbURL := newScratchDatabase(t, "")

	if err := Migrate(ctx, dbURL); err != nil {
		t.Fatalf("up: %v", err)
	}
	if err := Migrate(ctx, dbURL); err != nil {
		t.Fatalf("up again (no change): %v", err)
	}
	if err := migrateDown(ctx, dbURL); err != nil {
		t.Fatalf("down: %v", err)
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var tables int
	err = pool.QueryRow(ctx,
		`SELECT count(*) FROM pg_tables WHERE schemaname = 'public' AND tablename IN ('service_types', 'reservations')`,
	).Scan(&tables)
	if err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Errorf("%d tables left after down, want 0", tables)
	}

	if err := Migrate(ctx, dbURL); err != nil {
		t.Fatalf("up after down: %v", err)
	}
}

func TestMigrateHonoursCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := Migrate(ctx, newScratchDatabase(t, "")); err == nil {
		t.Error("Migrate with cancelled context succeeded, want error")
	}
}

func TestSeededServiceType(t *testing.T) {
	st, _ := newTestStore(t)
	got, err := st.q.GetServiceType(t.Context(), "carwash")
	if err != nil {
		t.Fatal(err)
	}
	if got.DisplayName != "Carwash" || got.DurationMinutes != 60 || !got.Offered {
		t.Errorf("carwash = %+v, want Carwash, 60, offered", got)
	}
}

func TestPing(t *testing.T) {
	st, _ := newTestStore(t)
	if err := st.Ping(t.Context()); err != nil {
		t.Errorf("Ping() = %v", err)
	}
}

func insertReservation(ctx context.Context, pool *pgxpool.Pool, startsAt, endsAt string, cancelled bool) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO reservations (starts_at, ends_at, customer_name, customer_email, customer_phone, service_type, cancelled_at)
		VALUES ($1, $2, 'Test Customer', 'test@example.com', '+36 30 123 4567', 'carwash',
		        CASE WHEN $3::boolean THEN now() END)`,
		startsAt, endsAt, cancelled)
	return err
}

func sqlState(err error) string {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code
	}
	return ""
}

func TestStartsAtGridConstraint(t *testing.T) {
	tests := []struct {
		name     string
		startsAt string
		endsAt   string
		wantCode string
	}{
		{"on the hour", "2030-01-15T10:00:00+01:00", "2030-01-15T11:00:00+01:00", ""},
		{"half past", "2030-01-15T10:30:00+01:00", "2030-01-15T11:30:00+01:00", "23514"},
		{"thirty seconds past", "2030-01-15T10:00:30+01:00", "2030-01-15T11:00:30+01:00", "23514"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, pool := newTestStore(t)
			err := insertReservation(t.Context(), pool, tt.startsAt, tt.endsAt, false)
			if got := sqlState(err); got != tt.wantCode {
				t.Errorf("insert error = %v (SQLSTATE %q), want SQLSTATE %q", err, got, tt.wantCode)
			}
		})
	}
}

func TestServiceTypeDurationConstraint(t *testing.T) {
	tests := []struct {
		minutes  int
		wantCode string
	}{
		{120, ""},
		{90, "23514"},
		{0, "23514"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.minutes), func(t *testing.T) {
			_, pool := newTestStore(t)
			_, err := pool.Exec(t.Context(),
				`INSERT INTO service_types (code, display_name, duration_minutes) VALUES ('detailing', 'Detailing', $1)`,
				tt.minutes)
			if got := sqlState(err); got != tt.wantCode {
				t.Errorf("insert error = %v (SQLSTATE %q), want SQLSTATE %q", err, got, tt.wantCode)
			}
		})
	}
}

func TestNoOverlapConstraint(t *testing.T) {
	type row struct {
		startsAt, endsAt string
		cancelled        bool
	}
	tests := []struct {
		name     string
		existing row
		second   row
		wantCode string
	}{
		{
			name:     "same slot rejected",
			existing: row{"2030-01-15T10:00:00+01:00", "2030-01-15T11:00:00+01:00", false},
			second:   row{"2030-01-15T10:00:00+01:00", "2030-01-15T11:00:00+01:00", false},
			wantCode: sqlStateExclusionViolation,
		},
		{
			name:     "partial overlap rejected",
			existing: row{"2030-01-15T10:00:00+01:00", "2030-01-15T12:00:00+01:00", false},
			second:   row{"2030-01-15T11:00:00+01:00", "2030-01-15T12:00:00+01:00", false},
			wantCode: sqlStateExclusionViolation,
		},
		{
			name:     "back to back accepted",
			existing: row{"2030-01-15T10:00:00+01:00", "2030-01-15T11:00:00+01:00", false},
			second:   row{"2030-01-15T11:00:00+01:00", "2030-01-15T12:00:00+01:00", false},
		},
		{
			name:     "overlap with cancelled row accepted",
			existing: row{"2030-01-15T10:00:00+01:00", "2030-01-15T11:00:00+01:00", true},
			second:   row{"2030-01-15T10:00:00+01:00", "2030-01-15T11:00:00+01:00", false},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			_, pool := newTestStore(t)
			if err := insertReservation(ctx, pool, tt.existing.startsAt, tt.existing.endsAt, tt.existing.cancelled); err != nil {
				t.Fatalf("insert existing: %v", err)
			}
			err := insertReservation(ctx, pool, tt.second.startsAt, tt.second.endsAt, tt.second.cancelled)
			if got := sqlState(err); got != tt.wantCode {
				t.Fatalf("insert second error = %v (SQLSTATE %q), want SQLSTATE %q", err, got, tt.wantCode)
			}
			if tt.wantCode != "" && !errors.Is(mapErr(err), service.ErrSlotConflict) {
				t.Errorf("mapErr(%v) is not ErrSlotConflict", err)
			}
		})
	}
}

func TestMapErrNotFound(t *testing.T) {
	st, _ := newTestStore(t)
	_, err := st.q.GetServiceType(t.Context(), "nope")
	if !errors.Is(mapErr(err), service.ErrNotFound) {
		t.Errorf("mapErr(%v) is not ErrNotFound", err)
	}
}

func TestUUIDRoundTrip(t *testing.T) {
	const id = "0192a3b4-c5d6-7e8f-9a0b-1c2d3e4f5a6b"
	u, err := parseUUID(id)
	if err != nil {
		t.Fatal(err)
	}
	if got := uuidString(u); got != id {
		t.Errorf("uuidString() = %q, want %q", got, id)
	}
	if _, err := parseUUID("not-a-uuid"); err == nil {
		t.Error("parseUUID(not-a-uuid) succeeded, want error")
	}
}
