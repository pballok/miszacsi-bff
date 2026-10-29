package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/99designs/gqlgen/graphql/playground"

	"github.com/pballok/miszacsi-bff/internal/config"
)

const readyPingTimeout = 2 * time.Second

// Pinger checks that a dependency is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

// New returns the service's HTTP handler: the GraphQL endpoint, health probes and, in
// development, the GraphQL playground. ready reports whether startup (migrations) finished
// and is cleared on shutdown.
func New(graphql http.Handler, db Pinger, ready *atomic.Bool, logger *slog.Logger, cfg config.Config) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /graphql", graphql)
	mux.Handle("GET /graphql", graphql)
	if cfg.Development() {
		mux.Handle("GET /playground", playground.Handler("MiSzaCsi BFF", "/graphql"))
	}
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) {
		writeText(w, http.StatusOK, "ok")
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if !ready.Load() {
			writeText(w, http.StatusServiceUnavailable, "not ready")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), readyPingTimeout)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			logger.WarnContext(r.Context(), "readiness check failed", "err", err)
			writeText(w, http.StatusServiceUnavailable, "database unavailable")
			return
		}
		writeText(w, http.StatusOK, "ok")
	})

	return Recover(logger)(RequestID(AccessLog(logger)(CORS(cfg.CORSAllowedOrigins)(mux))))
}

func writeText(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body + "\n")) // the client is gone if this fails; nothing to do
}
