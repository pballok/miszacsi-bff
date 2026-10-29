package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/pballok/miszacsi-bff/internal/logging"
)

func TestErrorPresenter(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantMsg   string
		wantCode  any
		wantField any
		wantLog   bool
	}{
		{
			name:      "bad user input keeps code and field",
			err:       badUserInput("to", "to must be after from"),
			wantMsg:   "to must be after from",
			wantCode:  codeBadUserInput,
			wantField: "to",
		},
		{
			name:     "not found keeps code",
			err:      notFound("reservation not found"),
			wantMsg:  "reservation not found",
			wantCode: codeNotFound,
		},
		{
			name:     "too many results keeps code",
			err:      tooManyResults("more than 1000 reservations match; request a shorter period"),
			wantMsg:  "more than 1000 reservations match; request a shorter period",
			wantCode: codeTooManyResults,
		},
		{
			name:     "gqlgen coercion error kept",
			err:      gqlerror.Wrap(errors.New("time: cannot parse")),
			wantMsg:  "time: cannot parse",
			wantCode: nil,
		},
		{
			name:     "resolver error masked",
			err:      &internalError{err: errors.New("connection refused")},
			wantMsg:  internalErrorMessage,
			wantCode: codeInternalError,
			wantLog:  true,
		},
		{
			name:     "resolver error wrapped by gqlgen still masked",
			err:      gqlerror.Wrap(&internalError{err: errors.New("connection refused")}),
			wantMsg:  internalErrorMessage,
			wantCode: codeInternalError,
			wantLog:  true,
		},
		{
			name:     "plain error masked",
			err:      errors.New("boom"),
			wantMsg:  internalErrorMessage,
			wantCode: codeInternalError,
			wantLog:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			ctx := logging.WithRequestID(t.Context(), "req-1")
			got := errorPresenter(logging.New(&logs, slog.LevelInfo))(ctx, tt.err)

			if got.Message != tt.wantMsg {
				t.Errorf("message = %q, want %q", got.Message, tt.wantMsg)
			}
			if code := got.Extensions["code"]; code != tt.wantCode {
				t.Errorf("code = %v, want %v", code, tt.wantCode)
			}
			if field := got.Extensions["field"]; field != tt.wantField {
				t.Errorf("field = %v, want %v", field, tt.wantField)
			}
			if !tt.wantLog {
				if logs.Len() != 0 {
					t.Errorf("unexpected log: %s", logs.String())
				}
				return
			}
			var rec map[string]any
			if err := json.Unmarshal(logs.Bytes(), &rec); err != nil {
				t.Fatalf("log is not one JSON record: %v: %s", err, logs.String())
			}
			if rec["request_id"] != "req-1" {
				t.Errorf("request_id = %v, want req-1", rec["request_id"])
			}
			if !strings.Contains(rec["err"].(string), tt.err.Error()) {
				t.Errorf("logged err = %v, want it to contain %q", rec["err"], tt.err.Error())
			}
		})
	}
}

func TestMarkInternalErrors(t *testing.T) {
	client := badUserInput("id", "bad")
	tests := []struct {
		name         string
		err          error
		wantInternal bool
	}{
		{"nil error", nil, false},
		{"client error untouched", client, false},
		{"other error marked", errors.New("db down"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := markInternalErrors(t.Context(), func(context.Context) (any, error) { return nil, tt.err })
			_, internal := errors.AsType[*internalError](err)
			if internal != tt.wantInternal {
				t.Errorf("internal = %v, want %v (err %v)", internal, tt.wantInternal, err)
			}
			if !errors.Is(err, tt.err) {
				t.Errorf("err %v does not wrap %v", err, tt.err)
			}
		})
	}
}

type gqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message    string         `json:"message"`
		Extensions map[string]any `json:"extensions"`
	} `json:"errors"`
}

// post runs query through a handler with no service; every resolver is still an
// unimplemented stub that panics.
func post(t *testing.T, logs *bytes.Buffer, query string) gqlResponse {
	t.Helper()
	h := NewHandler(nil, logging.New(logs, slog.LevelInfo), Options{MaxDepth: 10, MaxComplexity: 200})
	body, err := json.Marshal(map[string]any{"query": query})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequestWithContext(logging.WithRequestID(t.Context(), "req-2"),
		http.MethodPost, "/graphql", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var resp gqlResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
	return resp
}

func TestHandlerMasksPanic(t *testing.T) {
	var logs bytes.Buffer
	resp := post(t, &logs, `{ reservation(id: "x") { id } }`)
	if len(resp.Errors) != 1 {
		t.Fatalf("errors = %+v, want 1", resp.Errors)
	}
	if got := resp.Errors[0]; got.Message != internalErrorMessage || got.Extensions["code"] != codeInternalError {
		t.Errorf("error = %+v, want masked internal error", got)
	}
	if n := strings.Count(logs.String(), "\n"); n != 1 {
		t.Errorf("got %d log records, want exactly 1:\n%s", n, logs.String())
	}
	if !strings.Contains(logs.String(), `"request_id":"req-2"`) || !strings.Contains(logs.String(), "not implemented") {
		t.Errorf("panic log missing request_id or panic value: %s", logs.String())
	}
}

func TestHandlerKeepsInputErrors(t *testing.T) {
	var logs bytes.Buffer
	resp := post(t, &logs, `{ reservations(from: "not a time", to: "2030-01-16T10:00:00+01:00") { id } }`)
	if len(resp.Errors) == 0 {
		t.Fatal("want an error for a malformed DateTime")
	}
	if got := resp.Errors[0]; got.Message == internalErrorMessage || got.Extensions["code"] == codeInternalError {
		t.Errorf("input coercion error was masked: %+v", got)
	}
	if logs.Len() != 0 {
		t.Errorf("client input error was logged: %s", logs.String())
	}
}

func TestHandlerIntrospectionOnlyInDevelopment(t *testing.T) {
	tests := []struct {
		development bool
		wantErr     bool
	}{
		{true, false},
		{false, true},
	}
	for _, tt := range tests {
		h := NewHandler(nil, slog.New(slog.DiscardHandler), Options{MaxDepth: 10, MaxComplexity: 200, Development: tt.development})
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/graphql", strings.NewReader(`{"query":"{ __schema { queryType { name } } }"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var resp gqlResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if gotErr := len(resp.Errors) > 0; gotErr != tt.wantErr {
			t.Errorf("development=%v: errors = %+v, wantErr %v", tt.development, resp.Errors, tt.wantErr)
		}
	}
}
