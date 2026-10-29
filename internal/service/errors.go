package service

import (
	"errors"
	"strings"
)

var (
	// ErrNotFound means the reservation does not exist (or, for update and delete, is cancelled).
	ErrNotFound = errors.New("reservation not found")
	// ErrSlotConflict means the requested slot overlaps another active reservation.
	ErrSlotConflict = errors.New("slot overlaps another active reservation")
	// ErrTooManyResults means a listing matched more than MaxListResults reservations.
	ErrTooManyResults = errors.New("too many reservations match")
)

// Code classifies a validation failure. The values match the GraphQL ValidationErrorCode enum.
type Code string

// Validation error codes.
const (
	CodeRequired           Code = "REQUIRED"
	CodeTooLong            Code = "TOO_LONG"
	CodeInvalidFormat      Code = "INVALID_FORMAT"
	CodeInPast             Code = "IN_PAST"
	CodeNotOnGrid          Code = "NOT_ON_GRID"
	CodeUnknownServiceType Code = "UNKNOWN_SERVICE_TYPE"
	CodeNotAllowed         Code = "NOT_ALLOWED"
)

// FieldError is a validation failure of one input field. Field is the GraphQL input field
// name, e.g. "customerEmail".
type FieldError struct {
	Field   string
	Code    Code
	Message string
}

// ValidationErrors collects every field error found in one request.
type ValidationErrors []FieldError

func (v ValidationErrors) Error() string {
	msgs := make([]string, len(v))
	for i, fe := range v {
		msgs[i] = fe.Field + ": " + fe.Message
	}
	return "validation failed: " + strings.Join(msgs, "; ")
}

// Add records a field error.
func (v *ValidationErrors) Add(field string, code Code, msg string) {
	*v = append(*v, FieldError{Field: field, Code: code, Message: msg})
}
