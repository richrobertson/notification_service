package queue

import "time"

const (
	DispatchQueueName        = "notify:dispatch"
	DispatchWebhookQueueName = "notify:dispatch:webhook"
	DispatchEmailQueueName   = "notify:dispatch:email"
)

type DispatchJob struct {
	JobID          string    `json:"job_id"`
	NotificationID string    `json:"notification_id"`
	AttemptID      string    `json:"attempt_id"`
	TenantID       string    `json:"tenant_id"`
	Channel        string    `json:"channel"`
	CreatedAt      time.Time `json:"created_at"`
}

type PressureSnapshot struct {
	Depths     map[string]int `json:"depths"`
	SoftLimit  int            `json:"soft_limit"`
	HardLimit  int            `json:"hard_limit"`
	RetryAfter time.Duration  `json:"retry_after"`
}

func (s PressureSnapshot) AnySoftLimited() bool {
	for _, depth := range s.Depths {
		if depth >= s.SoftLimit && s.SoftLimit > 0 {
			return true
		}
	}
	return false
}
func (s PressureSnapshot) AnyHardLimited() bool {
	for _, depth := range s.Depths {
		if depth >= s.HardLimit && s.HardLimit > 0 {
			return true
		}
	}
	return false
}
func (s PressureSnapshot) AcceptingWrites() bool { return !s.AnyHardLimited() }
