package graph

import (
	"log/slog"
	"net/http"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"

	"github.com/pballok/miszacsi-bff/internal/service"
)

// Options configures the GraphQL handler.
type Options struct {
	MaxDepth      int
	MaxComplexity int
	// Development enables introspection.
	Development bool
}

// NewHandler returns the GraphQL HTTP handler serving the reservation API.
func NewHandler(svc *service.Service, logger *slog.Logger, opts Options) http.Handler {
	srv := handler.New(NewExecutableSchema(Config{Resolvers: &Resolver{svc: svc, logger: logger}}))
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.Use(extension.FixedComplexityLimit(opts.MaxComplexity))
	srv.Use(depthLimit{max: opts.MaxDepth})
	if opts.Development {
		srv.Use(extension.Introspection{})
	}
	srv.AroundFields(markInternalErrors)
	srv.SetErrorPresenter(errorPresenter(logger))
	srv.SetRecoverFunc(recoverFunc(logger))
	return srv
}
