package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func decodeLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("invalid JSON log line %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func TestRequestIDAttribute(t *testing.T) {
	tests := []struct {
		name    string
		ctx     context.Context
		log     func(l *slog.Logger, ctx context.Context)
		wantID  string
		wantKey string
	}{
		{
			name:   "with request ID",
			ctx:    WithRequestID(context.Background(), "abc123"),
			log:    func(l *slog.Logger, ctx context.Context) { l.InfoContext(ctx, "hello") },
			wantID: "abc123",
		},
		{
			name: "without request ID",
			ctx:  context.Background(),
			log:  func(l *slog.Logger, ctx context.Context) { l.InfoContext(ctx, "hello") },
		},
		{
			name:   "WithAttrs keeps the wrapper",
			ctx:    WithRequestID(context.Background(), "r1"),
			log:    func(l *slog.Logger, ctx context.Context) { l.With("k", "v").InfoContext(ctx, "hello") },
			wantID: "r1",
		},
		{
			name:    "WithGroup keeps the wrapper",
			ctx:     WithRequestID(context.Background(), "r2"),
			log:     func(l *slog.Logger, ctx context.Context) { l.WithGroup("g").InfoContext(ctx, "hello") },
			wantID:  "r2",
			wantKey: "g",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.log(New(&buf, slog.LevelInfo), tt.ctx)
			lines := decodeLines(t, &buf)
			if len(lines) != 1 {
				t.Fatalf("got %d log lines, want 1", len(lines))
			}
			rec := lines[0]
			if tt.wantKey != "" {
				g, ok := rec[tt.wantKey].(map[string]any)
				if !ok {
					t.Fatalf("group %q missing in %v", tt.wantKey, rec)
				}
				rec = g
			}
			id, ok := rec["request_id"]
			if tt.wantID == "" {
				if ok {
					t.Errorf("request_id = %v, want absent", id)
				}
				return
			}
			if id != tt.wantID {
				t.Errorf("request_id = %v, want %q", id, tt.wantID)
			}
		})
	}
}

func TestLevelFiltering(t *testing.T) {
	tests := []struct {
		level slog.Level
		want  int
	}{
		{slog.LevelDebug, 4},
		{slog.LevelInfo, 3},
		{slog.LevelWarn, 2},
		{slog.LevelError, 1},
	}
	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			var buf bytes.Buffer
			l := New(&buf, tt.level)
			l.Debug("d")
			l.Info("i")
			l.Warn("w")
			l.Error("e")
			if got := len(decodeLines(t, &buf)); got != tt.want {
				t.Errorf("got %d lines, want %d", got, tt.want)
			}
		})
	}
}

func TestRequestIDRoundTrip(t *testing.T) {
	if _, ok := RequestID(context.Background()); ok {
		t.Error("RequestID on empty context reported ok")
	}
	if id, ok := RequestID(WithRequestID(context.Background(), "x")); !ok || id != "x" {
		t.Errorf("RequestID = %q, %v; want x, true", id, ok)
	}
}
