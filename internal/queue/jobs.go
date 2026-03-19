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
