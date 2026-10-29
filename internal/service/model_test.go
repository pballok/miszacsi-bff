package service

import (
	"testing"
	"time"
)

func TestReservationStatus(t *testing.T) {
	now := time.Date(2030, 1, 15, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		cancelledAt *time.Time
		want        Status
	}{
		{"active", nil, StatusActive},
		{"cancelled", &now, StatusCancelled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Reservation{CancelledAt: tt.cancelledAt}).Status(); got != tt.want {
				t.Errorf("Status() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestServiceTypeDuration(t *testing.T) {
	tests := []struct {
		minutes int
		want    time.Duration
	}{
		{60, time.Hour},
		{120, 2 * time.Hour},
	}
	for _, tt := range tests {
		if got := (ServiceType{DurationMinutes: tt.minutes}).Duration(); got != tt.want {
			t.Errorf("Duration(%d min) = %v, want %v", tt.minutes, got, tt.want)
		}
	}
}

func TestValidationErrors(t *testing.T) {
	var v ValidationErrors
	v.Add("customerEmail", CodeInvalidFormat, "not an email address")
	v.Add("startsAt", CodeNotOnGrid, "must start on the hour")

	want := ValidationErrors{
		{Field: "customerEmail", Code: CodeInvalidFormat, Message: "not an email address"},
		{Field: "startsAt", Code: CodeNotOnGrid, Message: "must start on the hour"},
	}
	if len(v) != len(want) || v[0] != want[0] || v[1] != want[1] {
		t.Fatalf("errors = %+v, want %+v", v, want)
	}
	const wantMsg = "validation failed: customerEmail: not an email address; startsAt: must start on the hour"
	if got := v.Error(); got != wantMsg {
		t.Errorf("Error() = %q, want %q", got, wantMsg)
	}
}
