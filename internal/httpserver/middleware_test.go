package httpserver

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/pballok/miszacsi-bff/internal/logging"
)

func TestRequestID(t *testing.T) {
	generated := regexp.MustCompile(`^[0-9a-f]{32}$`)
	tests := []struct {
		name     string
		incoming string
		wantSame bool
	}{
		{"well-formed accepted", "abc-123_X.y", true},
		{"64 chars accepted", strings.Repeat("a", 64), true},
		{"missing generated", "", false},
		{"65 chars replaced", strings.Repeat("a", 65), false},
		{"invalid chars replaced", "abc 123", false},
		{"injection replaced", "a\nb", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var inCtx string
			h := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				inCtx, _ = logging.RequestID(r.Context())
			}))
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			if tt.incoming != "" {
				req.Header.Set(headerRequestID, tt.incoming)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			echoed := rec.Header().Get(headerRequestID)
			if echoed != inCtx {
				t.Errorf("echoed %q, context %q; want equal", echoed, inCtx)
			}
			if tt.wantSame && echoed != tt.incoming {
				t.Errorf("request ID = %q, want %q", echoed, tt.incoming)
			}
			if !tt.wantSame && !generated.MatchString(echoed) {
				t.Errorf("request ID = %q, want 32 hex chars", echoed)
			}
		})
	}
}

func TestAccessLog(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantStatus float64
	}{
		{"explicit status", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }, 418},
		{"implicit 200 on write", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("hi")) }, 200},
		{"implicit 200 without write", func(http.ResponseWriter, *http.Request) {}, 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			h := RequestID(AccessLog(logging.New(&logs, slog.LevelInfo))(tt.handler))
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/graphql?secret=customer@example.com", nil)
			req.Header.Set(headerRequestID, "req-9")
			h.ServeHTTP(httptest.NewRecorder(), req)

			var rec map[string]any
			if err := json.Unmarshal(logs.Bytes(), &rec); err != nil {
				t.Fatalf("log is not one JSON record: %v: %s", err, logs.String())
			}
			if rec["request_id"] != "req-9" || rec["status"] != tt.wantStatus || rec["method"] != "POST" || rec["path"] != "/graphql" {
				t.Errorf("record = %v", rec)
			}
			if _, ok := rec["duration_ms"]; !ok {
				t.Error("duration_ms missing")
			}
			if strings.Contains(logs.String(), "customer@example.com") {
				t.Error("query string leaked into the access log")
			}
		})
	}
}

func TestRecover(t *testing.T) {
	var logs bytes.Buffer
	h := Recover(logging.New(&logs, slog.LevelInfo))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("kaboom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if !strings.Contains(logs.String(), "kaboom") {
		t.Errorf("panic not logged: %s", logs.String())
	}
}

func TestCORS(t *testing.T) {
	const site = "https://app.example"
	tests := []struct {
		name        string
		allowed     []string
		method      string
		origin      string
		preflight   bool
		wantOrigin  string
		wantStatus  int
		wantMethods bool
	}{
		{"allowed origin", []string{site}, http.MethodPost, site, false, site, http.StatusOK, false},
		{"disallowed origin", []string{site}, http.MethodPost, "https://evil.example", false, "", http.StatusOK, false},
		{"no origin", []string{site}, http.MethodPost, "", false, "", http.StatusOK, false},
		{"wildcard allows any origin", []string{"*"}, http.MethodPost, "http://localhost:5173", false, "http://localhost:5173", http.StatusOK, false},
		{"preflight allowed", []string{site}, http.MethodOptions, site, true, site, http.StatusNoContent, true},
		{"preflight disallowed", []string{site}, http.MethodOptions, "https://evil.example", true, "", http.StatusOK, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := CORS(tt.allowed)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			req := httptest.NewRequestWithContext(t.Context(), tt.method, "/graphql", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.preflight {
				req.Header.Set("Access-Control-Request-Method", "POST")
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tt.wantOrigin {
				t.Errorf("Allow-Origin = %q, want %q", got, tt.wantOrigin)
			}
			if got := rec.Header().Get("Vary"); got != "Origin" {
				t.Errorf("Vary = %q, want Origin", got)
			}
			gotMethods := rec.Header().Get("Access-Control-Allow-Methods") != ""
			if gotMethods != tt.wantMethods {
				t.Errorf("Allow-Methods present = %v, want %v", gotMethods, tt.wantMethods)
			}
			if tt.wantMethods {
				if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, OPTIONS" {
					t.Errorf("Allow-Methods = %q", got)
				}
				if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type, X-Request-ID" {
					t.Errorf("Allow-Headers = %q", got)
				}
				if got := rec.Header().Get("Access-Control-Max-Age"); got != "600" {
					t.Errorf("Max-Age = %q", got)
				}
			}
		})
	}
}
