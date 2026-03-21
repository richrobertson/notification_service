package store

import "testing"

func TestDeriveNotificationStatus(t *testing.T) {
	tests := []struct {
		name     string
		attempts []DeliveryAttempt
		want     string
	}{
		{name: "accepted with no attempts", want: "accepted"},
		{name: "processing pending", attempts: []DeliveryAttempt{{Status: "pending"}}, want: "processing"},
		{name: "delivered", attempts: []DeliveryAttempt{{Status: "sent"}}, want: "delivered"},
		{name: "failed", attempts: []DeliveryAttempt{{Status: "failed"}}, want: "failed"},
		{name: "dead lettered", attempts: []DeliveryAttempt{{Status: "dead_lettered"}}, want: "dead_lettered"},
		{name: "partial", attempts: []DeliveryAttempt{{Status: "sent"}, {Status: "retry_scheduled"}}, want: "partially_delivered"},
		{name: "processing after dead letter with pending", attempts: []DeliveryAttempt{{Status: "dead_lettered"}, {Status: "pending"}}, want: "processing"},
		{name: "processing after dead letter with in progress", attempts: []DeliveryAttempt{{Status: "dead_lettered"}, {Status: "in_progress"}}, want: "processing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveNotificationStatus(tt.attempts); got != tt.want {
				t.Fatalf("deriveNotificationStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAttemptStateClassification(t *testing.T) {
	for _, status := range []string{"sent", "failed", "retry_scheduled", "dead_lettered"} {
		if !isAttemptTerminalState(status) {
			t.Fatalf("status %q should be terminal", status)
		}
	}
	for _, status := range []string{"pending", "in_progress"} {
		if isAttemptTerminalState(status) {
			t.Fatalf("status %q should not be terminal", status)
		}
	}
}
