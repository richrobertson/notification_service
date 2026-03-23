package queue

import "time"

const (
	// DispatchQueueName is the shared entry queue used by the Stage 8 outbox
	// publisher before channel-specific routing happens.
	DispatchQueueName = "notify:dispatch"
	// DispatchWebhookQueueName is the queue consumed by webhook workers.
	DispatchWebhookQueueName = "notify:dispatch:webhook"
	// DispatchEmailQueueName is the queue consumed by email workers.
	DispatchEmailQueueName = "notify:dispatch:email"
)

// DispatchJob is the transport payload carried through Redis.
type DispatchJob struct {
	JobID          string    `json:"job_id"`
	NotificationID string    `json:"notification_id"`
	AttemptID      string    `json:"attempt_id"`
	TenantID       string    `json:"tenant_id"`
	Channel        string    `json:"channel"`
	CreatedAt      time.Time `json:"created_at"`
}

// PressureSnapshot summarizes current queue depths and the thresholds the API
// should use for soft warnings and hard write rejection.
type PressureSnapshot struct {
	Depths     map[string]int `json:"depths"`
	SoftLimit  int            `json:"soft_limit"`
	HardLimit  int            `json:"hard_limit"`
	RetryAfter time.Duration  `json:"retry_after"`
}

// AnySoftLimited reports whether any tracked queue has reached the soft limit.
func (s PressureSnapshot) AnySoftLimited() bool {
	for _, depth := range s.Depths {
		if depth >= s.SoftLimit && s.SoftLimit > 0 {
			return true
		}
	}
	return false
}

// AnyHardLimited reports whether any tracked queue has reached the hard limit.
func (s PressureSnapshot) AnyHardLimited() bool {
	for _, depth := range s.Depths {
		if depth >= s.HardLimit && s.HardLimit > 0 {
			return true
		}
	}
	return false
}

// AcceptingWrites reports whether the current snapshot still permits new work
// to be accepted safely.
func (s PressureSnapshot) AcceptingWrites() bool { return !s.AnyHardLimited() }
