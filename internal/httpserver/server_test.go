package httpserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/pballok/miszacsi-bff/internal/config"
)

type pingFunc func(ctx context.Context) error

func (f pingFunc) Ping(ctx context.Context) error { return f(ctx) }

func newTestHandler(t *testing.T, ready bool, pingErr error, appEnv string) http.Handler {
	t.Helper()
	var r atomic.Bool
	r.Store(ready)
	gql := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	db := pingFunc(func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("readiness ping has no deadline")
		}
		return pingErr
	})
	cfg := config.Config{AppEnv: appEnv, CORSAllowedOrigins: []string{"https://app.example"}}
	return New(gql, db, &r, slog.New(slog.DiscardHandler), cfg)
}

func TestRoutes(t *testing.T) {
	errDown := errors.New("db down")
	tests := []struct {
		name       string
		ready      bool
		pingErr    error
		appEnv     string
		method     string
		path       string
		wantStatus int
	}{
		{"livez", false, errDown, "production", http.MethodGet, "/livez", http.StatusOK},
		{"readyz when ready", true, nil, "production", http.MethodGet, "/readyz", http.StatusOK},
		{"readyz before startup finished", false, nil, "production", http.MethodGet, "/readyz", http.StatusServiceUnavailable},
		{"readyz when ping fails", true, errDown, "production", http.MethodGet, "/readyz", http.StatusServiceUnavailable},
		{"graphql POST", true, nil, "production", http.MethodPost, "/graphql", http.StatusOK},
		{"graphql GET", true, nil, "production", http.MethodGet, "/graphql", http.StatusOK},
		{"graphql PUT not allowed", true, nil, "production", http.MethodPut, "/graphql", http.StatusMethodNotAllowed},
		{"playground in development", true, nil, "development", http.MethodGet, "/playground", http.StatusOK},
		{"playground absent in production", true, nil, "production", http.MethodGet, "/playground", http.StatusNotFound},
		{"unknown path", true, nil, "production", http.MethodGet, "/nope", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(t, tt.ready, tt.pingErr, tt.appEnv)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, nil))
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if rec.Header().Get(headerRequestID) == "" {
				t.Error("X-Request-ID missing: middleware not applied")
			}
		})
	}
}

func TestPreflightThroughServer(t *testing.T) {
	h := newTestHandler(t, true, nil, "production")
	req := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/graphql", nil)
	req.Header.Set("Origin", "https://app.example")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
}
