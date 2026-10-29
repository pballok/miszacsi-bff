package graph_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/pballok/miszacsi-bff/internal/config"
	"github.com/pballok/miszacsi-bff/internal/graph"
	"github.com/pballok/miszacsi-bff/internal/httpserver"
	"github.com/pballok/miszacsi-bff/internal/service"
	"github.com/pballok/miszacsi-bff/internal/store"
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
	if err := adminExec(ctx, "CREATE DATABASE "+templateDatabase); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := store.Migrate(ctx, databaseURL(templateDatabase)); err != nil {
		fmt.Fprintln(os.Stderr, "migrate template database:", err)
		return 1
	}
	return m.Run()
}

func databaseURL(name string) string {
	u, err := url.Parse(adminURL)
	if err != nil {
		panic(err) // adminURL comes from testcontainers and is always valid
	}
	u.Path = "/" + name
	return u.String()
}

func adminExec(ctx context.Context, sql string) error {
	pool, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		return fmt.Errorf("connect admin: %w", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, sql); err != nil {
		return fmt.Errorf("exec %q: %w", sql, err)
	}
	return nil
}

// newTestServer starts the real HTTP stack on a fresh database copied from the migrated
// template. The real clock is used so "in the future" validation agrees with PostgreSQL now().
func newTestServer(t *testing.T) (*httptest.Server, *pgxpool.Pool) {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	name := "t_" + hex.EncodeToString(b)
	if err := adminExec(t.Context(), "CREATE DATABASE "+name+" TEMPLATE "+templateDatabase); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(t.Context(), databaseURL(name))
	if err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.DiscardHandler)
	cfg := config.Config{AppEnv: "development", CORSAllowedOrigins: []string{"*"}}
	st := store.New(pool)
	gql := graph.NewHandler(service.New(st, time.Now), logger, graph.Options{MaxDepth: 10, MaxComplexity: 200, Development: true})
	var ready atomic.Bool
	ready.Store(true)
	srv := httptest.NewServer(httpserver.New(gql, st, &ready, logger, cfg))

	t.Cleanup(func() {
		srv.Close()
		pool.Close()
		if err := adminExec(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("drop database: %v", err)
		}
	})
	return srv, pool
}

type gqlError struct {
	Message    string         `json:"message"`
	Path       []any          `json:"path"`
	Extensions map[string]any `json:"extensions"`
}

type response struct {
	Data   json.RawMessage `json:"data"`
	Errors []gqlError      `json:"errors"`
}

// gql posts query with vars to /graphql and decodes the response.
func gql(t *testing.T, srv *httptest.Server, query string, vars map[string]any) response {
	t.Helper()
	body, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/graphql", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out response
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response (status %d): %v", resp.StatusCode, err)
	}
	return out
}

// at returns the RFC 3339 instant of hh:mm on the fixed test day, 2030-01-15 at +01:00.
func at(hhmm string) string {
	return "2030-01-15T" + hhmm + ":00+01:00"
}

func TestHarness(t *testing.T) {
	srv, pool := newTestServer(t)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/readyz", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/readyz status = %d, want 200", resp.StatusCode)
	}

	var n int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM service_types WHERE code = 'carwash'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("carwash rows = %d, want 1 (template not migrated?)", n)
	}

	out := gql(t, srv, `query($from: DateTime!, $to: DateTime!) { reservations(from: $from, to: $to) { id } }`,
		map[string]any{"from": at("10:00"), "to": "bad"})
	if len(out.Errors) == 0 {
		t.Error("want a GraphQL error for a malformed DateTime variable")
	}
}
