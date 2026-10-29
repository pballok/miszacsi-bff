package service

import "time"

// Service implements the reservation use cases.
type Service struct {
	store Store
	now   func() time.Time
}

// New returns a Service backed by store. now is the clock used to validate requests.
func New(store Store, now func() time.Time) *Service {
	return &Service{store: store, now: now}
}

// MaxListResults is the most reservations one listing returns. A period with more matches fails
// with ErrTooManyResults instead of being truncated.
const MaxListResults = 1000
