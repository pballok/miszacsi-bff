// Package graph implements the GraphQL API on top of the reservation service.
package graph

import (
	"log/slog"

	"github.com/pballok/miszacsi-bff/internal/service"
)

// Resolver is the root resolver. It holds the dependencies every resolver needs.
type Resolver struct {
	svc    *service.Service
	logger *slog.Logger
}
