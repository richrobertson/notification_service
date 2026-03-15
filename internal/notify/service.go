package notify

import (
	"errors"
	"sync"
	"time"
)

var (
	errTenantExists         = errors.New("tenant already exists")
	errTenantNotFound       = errors.New("tenant not found")
	errTemplateExists       = errors.New("template already exists")
	errTemplateNotFound     = errors.New("template not found")
	errNotificationNotFound = errors.New("notification not found")
	errDailyQuotaExceeded   = errors.New("daily quota exceeded")
)

type Tenant struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	DailyQuota int       `json:"daily_quota"`
	CreatedAt  time.Time `json:"created_at"`
}

type Template struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Name      string    `json:"name"`
	Channel   string    `json:"channel"`
	Version   int       `json:"version"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type DeliveryAttempt struct {
	ID             string     `json:"id"`
	NotificationID string     `json:"notification_id"`
	Channel        string     `json:"channel"`
	AttemptNumber  int        `json:"attempt_number"`
	Status         string     `json:"status"`
	ErrorCode      *string    `json:"error_code"`
	NextRetryAt    *time.Time `json:"next_retry_at"`
	CompletedAt    *time.Time `json:"completed_at"`
}

type Notification struct {
	ID             string            `json:"id"`
	TenantID       string            `json:"tenant_id"`
	TemplateID     string            `json:"template_id"`
	Channels       []string          `json:"channels"`
	Recipient      map[string]any    `json:"recipient"`
	Variables      map[string]any    `json:"variables"`
	IdempotencyKey string            `json:"idempotency_key"`
	Status         string            `json:"status"`
	SubmittedAt    time.Time         `json:"submitted_at"`
	Attempts       []DeliveryAttempt `json:"attempts"`
}

type DeadLetter struct {
	ID             string    `json:"id"`
	NotificationID string    `json:"notification_id"`
	Channel        string    `json:"channel"`
	FinalError     string    `json:"final_error"`
	DeadLetteredAt time.Time `json:"dead_lettered_at"`
}

type Usage struct {
	TenantID              string `json:"tenant_id"`
	Date                  string `json:"date"`
	AcceptedNotifications int    `json:"accepted_notifications"`
	DailyQuota            int    `json:"daily_quota"`
	RemainingQuota        int    `json:"remaining_quota"`
}

type CreateTenantInput struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	DailyQuota int    `json:"daily_quota"`
}

type CreateTemplateInput struct {
	ID       string `json:"id"`
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
	Channel  string `json:"channel"`
	Body     string `json:"body"`
}

type UpdateTemplateInput struct {
	Name string `json:"name"`
	Body string `json:"body"`
}

type CreateNotificationInput struct {
	TenantID       string         `json:"tenant_id"`
	TemplateID     string         `json:"template_id"`
	Channels       []string       `json:"channels"`
	Recipient      map[string]any `json:"recipient"`
	Variables      map[string]any `json:"variables"`
	IdempotencyKey string         `json:"idempotency_key"`
}

type Service struct {
	mu               sync.RWMutex
	tenants          map[string]Tenant
	templates        map[string]Template
	notifications    map[string]Notification
	deadLetters      map[string]DeadLetter
	idempotencyIndex map[string]string
	now              func() time.Time
	idgen            func() string
}

func NewService() *Service {
	return &Service{
		tenants:          make(map[string]Tenant),
		templates:        make(map[string]Template),
		notifications:    make(map[string]Notification),
		deadLetters:      make(map[string]DeadLetter),
		idempotencyIndex: make(map[string]string),
		now:              func() time.Time { return time.Now().UTC() },
		idgen:            newID,
	}
}

func (s *Service) CreateTenant(input CreateTenantInput) (Tenant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.tenants[input.ID]; exists {
		return Tenant{}, errTenantExists
	}

	tenant := Tenant{
		ID:         input.ID,
		Name:       input.Name,
		Status:     "active",
		DailyQuota: input.DailyQuota,
		CreatedAt:  s.now(),
	}
	s.tenants[tenant.ID] = tenant
	return tenant, nil
}

func (s *Service) GetTenant(id string) (Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tenant, ok := s.tenants[id]
	if !ok {
		return Tenant{}, errTenantNotFound
	}
	return tenant, nil
}

func (s *Service) CreateTemplate(input CreateTemplateInput) (Template, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.tenants[input.TenantID]; !ok {
		return Template{}, errTenantNotFound
	}
	if _, exists := s.templates[input.ID]; exists {
		return Template{}, errTemplateExists
	}

	template := Template{
		ID:        input.ID,
		TenantID:  input.TenantID,
		Name:      input.Name,
		Channel:   input.Channel,
		Version:   1,
		Body:      input.Body,
		CreatedAt: s.now(),
	}
	s.templates[template.ID] = template
	return template, nil
}

func (s *Service) GetTemplate(id string) (Template, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	template, ok := s.templates[id]
	if !ok {
		return Template{}, errTemplateNotFound
	}
	return template, nil
}

func (s *Service) UpdateTemplate(id string, input UpdateTemplateInput) (Template, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	template, ok := s.templates[id]
	if !ok {
		return Template{}, errTemplateNotFound
	}

	template.Name = input.Name
	template.Body = input.Body
	template.Version++
	s.templates[id] = template
	return template, nil
}

func (s *Service) CreateNotification(input CreateNotificationInput) (Notification, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tenant, ok := s.tenants[input.TenantID]
	if !ok {
		return Notification{}, false, errTenantNotFound
	}

	template, ok := s.templates[input.TemplateID]
	if !ok || template.TenantID != input.TenantID {
		return Notification{}, false, errTemplateNotFound
	}

	indexKey := input.TenantID + ":" + input.IdempotencyKey
	if existingID, ok := s.idempotencyIndex[indexKey]; ok {
		return s.notifications[existingID], true, nil
	}

	usageCount := 0
	today := s.now().Format(time.DateOnly)
	for _, notification := range s.notifications {
		if notification.TenantID == input.TenantID && notification.SubmittedAt.Format(time.DateOnly) == today {
			usageCount++
		}
	}
	if usageCount >= tenant.DailyQuota {
		return Notification{}, false, errDailyQuotaExceeded
	}

	notificationID := s.idgen()
	attempts := make([]DeliveryAttempt, 0, len(input.Channels))
	for _, channel := range input.Channels {
		attempts = append(attempts, DeliveryAttempt{
			ID:             s.idgen(),
			NotificationID: notificationID,
			Channel:        channel,
			AttemptNumber:  1,
			Status:         "pending",
		})
	}

	notification := Notification{
		ID:             notificationID,
		TenantID:       input.TenantID,
		TemplateID:     input.TemplateID,
		Channels:       append([]string(nil), input.Channels...),
		Recipient:      cloneMap(input.Recipient),
		Variables:      cloneMap(input.Variables),
		IdempotencyKey: input.IdempotencyKey,
		Status:         "accepted",
		SubmittedAt:    s.now(),
		Attempts:       attempts,
	}
	s.notifications[notification.ID] = notification
	s.idempotencyIndex[indexKey] = notification.ID
	return notification, false, nil
}

func (s *Service) GetNotification(id string) (Notification, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	notification, ok := s.notifications[id]
	if !ok {
		return Notification{}, errNotificationNotFound
	}
	return notification, nil
}

func (s *Service) ReplayNotification(id string) (Notification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	notification, ok := s.notifications[id]
	if !ok {
		return Notification{}, errNotificationNotFound
	}

	for _, channel := range notification.Channels {
		attemptCount := 0
		for _, attempt := range notification.Attempts {
			if attempt.Channel == channel {
				attemptCount++
			}
		}
		notification.Attempts = append(notification.Attempts, DeliveryAttempt{
			ID:             s.idgen(),
			NotificationID: notification.ID,
			Channel:        channel,
			AttemptNumber:  attemptCount + 1,
			Status:         "pending",
		})
	}
	notification.Status = "processing"
	s.notifications[id] = notification
	return notification, nil
}

func (s *Service) Usage(tenantID string) (Usage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tenant, ok := s.tenants[tenantID]
	if !ok {
		return Usage{}, errTenantNotFound
	}

	today := s.now().Format(time.DateOnly)
	accepted := 0
	for _, notification := range s.notifications {
		if notification.TenantID == tenantID && notification.SubmittedAt.Format(time.DateOnly) == today {
			accepted++
		}
	}

	remaining := tenant.DailyQuota - accepted
	if remaining < 0 {
		remaining = 0
	}

	return Usage{
		TenantID:              tenantID,
		Date:                  today,
		AcceptedNotifications: accepted,
		DailyQuota:            tenant.DailyQuota,
		RemainingQuota:        remaining,
	}, nil
}

func (s *Service) DeadLetters() []DeadLetter {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]DeadLetter, 0, len(s.deadLetters))
	for _, deadLetter := range s.deadLetters {
		result = append(result, deadLetter)
	}
	return result
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
