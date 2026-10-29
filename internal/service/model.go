// Package service holds the reservation domain types and business rules.
package service

import "time"

// ServiceType is a kind of service a reservation can be booked for.
//
//nolint:revive // the name stutters but matches the data model and the GraphQL contract.
type ServiceType struct {
	Code            string
	DisplayName     string
	DurationMinutes int
	Offered         bool
}

// Duration returns how long one booking of the service type lasts.
func (s ServiceType) Duration() time.Duration {
	return time.Duration(s.DurationMinutes) * time.Minute
}

// Status is the lifecycle state of a reservation.
type Status string

// Reservation statuses.
const (
	StatusActive    Status = "ACTIVE"
	StatusCancelled Status = "CANCELLED"
)

// Reservation is a stored reservation.
type Reservation struct {
	ID            string
	StartsAt      time.Time
	EndsAt        time.Time
	CustomerName  string
	CustomerEmail string
	CustomerPhone string
	ServiceType   ServiceType
	CancelledAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Status reports CANCELLED when the reservation has been cancelled, ACTIVE otherwise.
func (r Reservation) Status() Status {
	if r.CancelledAt != nil {
		return StatusCancelled
	}
	return StatusActive
}

// NewReservation holds the fields written when a reservation is created.
type NewReservation struct {
	StartsAt        time.Time
	EndsAt          time.Time
	CustomerName    string
	CustomerEmail   string
	CustomerPhone   string
	ServiceTypeCode string
}
