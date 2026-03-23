# github.com/richrobertson/notification-platform/internal/store

_Generated from Go doc comments. Run `make docs-godoc` to refresh._

```text
package store // import "github.com/richrobertson/notification-platform/internal/store"

Package store contains the Postgres-backed durable model for the
notification_service runtime.

It is the system's authoritative state layer for tenants, templates,
notifications, delivery attempts, dead letters, policies, scheduled work,
dispatch intents, audit events, and maintenance visibility.

The exported Postgres methods are intentionally explicit instead of generic.
That keeps the invariants near the queries that enforce them, which is
especially important for:

  - monotonic attempt state transitions
  - idempotent repair paths
  - Stage 8 dispatch outbox publication
  - Stage 9 policy and scheduling controls
  - Stage 10 operational metrics and cleanup flows

New contributors should read the exported Postgres methods as the durable
workflow contract for the rest of the system.

VARIABLES

var (
	// ErrNotFound reports that the requested durable record does not exist.
	ErrNotFound = errors.New("store: not found")
	// ErrConflict reports that a uniqueness or ownership invariant was violated.
	ErrConflict = errors.New("store: conflict")
	// ErrAttemptAlreadyFinalized reports that an attempt can no longer move into a
	// new active state.
	ErrAttemptAlreadyFinalized = errors.New("store: attempt already finalized")
	// ErrAttemptAlreadyProcessing reports that another worker already owns the
	// active execution of an attempt.
	ErrAttemptAlreadyProcessing = errors.New("store: attempt already processing")
	// ErrInvalidStateTransition reports that the requested transition would break
	// the durable workflow rules.
	ErrInvalidStateTransition = errors.New("store: invalid state transition")
)

FUNCTIONS

func IsAttemptAlreadyFinalized(err error) bool
    IsAttemptAlreadyFinalized reports whether an error means the attempt is
    already terminal.

func IsAttemptAlreadyProcessing(err error) bool
    IsAttemptAlreadyProcessing reports whether another worker already owns the
    attempt.

func IsConflict(err error) bool
    IsConflict reports whether an error should be treated as a durable conflict.

func IsInvalidStateTransition(err error) bool
    IsInvalidStateTransition reports whether the requested durable transition
    was rejected by workflow rules.


TYPES

type AuditEvent struct {
	ID           string         `json:"id"`
	TenantID     string         `json:"tenant_id"`
	Actor        string         `json:"actor"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"created_at"`
}
    AuditEvent is the durable audit-trail record for operator and runtime
    actions.

type CleanupParams struct {
	Now                 time.Time
	AuditRetention      time.Duration
	OutboxRetention     time.Duration
	DeadLetterRetention time.Duration
	DryRun              bool
}
    CleanupParams controls one explicit maintenance pass.

type CleanupResult struct {
	DryRun                 bool      `json:"dry_run"`
	AuditEventsDeleted     int64     `json:"audit_events_deleted"`
	PublishedOutboxDeleted int64     `json:"published_outbox_deleted"`
	DeadLettersDeleted     int64     `json:"dead_letters_deleted"`
	ExecutedAt             time.Time `json:"executed_at"`
}
    CleanupResult reports what a maintenance pass deleted, or would have deleted
    in dry-run mode.

type CreateDeliveryAttemptParams struct {
	ID                 string
	NotificationID     string
	Channel            string
	AttemptNumber      int
	Status             string
	NextRetryAt        *time.Time
	LastError          *string
	DispatchEnqueuedAt *time.Time
	EnqueueKind        string
}
    CreateDeliveryAttemptParams holds the input for CreateDeliveryAttempt.

type CreateDispatchIntentParams struct {
	ID             string
	NotificationID string
	AttemptID      string
	TenantID       string
	Channel        string
	Source         string
	Status         string
	PublishedAt    *time.Time
}
    CreateDispatchIntentParams holds the input for CreateDispatchIntent.

type CreateNotificationDispatchParams struct {
	Notification CreateNotificationParams
	Channel      string
	AttemptID    string
	IntentID     string
}
    CreateNotificationDispatchParams combines notification, initial attempt,
    and initial dispatch-intent creation.

type CreateNotificationParams struct {
	ID                  string
	TenantID            string
	TemplateID          string
	IdempotencyKey      *string
	RecipientEmail      *string
	RecipientWebhookURL *string
	SecondaryWebhookURL *string
	Variables           map[string]any
	ScheduledFor        *time.Time
}
    CreateNotificationParams holds the durable notification fields created
    before initial dispatch behavior is applied.

type CreateTemplateParams struct {
	ID       string
	TenantID string
	Name     string
	Channel  string
	Version  int
	Body     string
}
    CreateTemplateParams holds the input for CreateTemplate.

type CreateTenantParams struct {
	ID         string
	Name       string
	DailyQuota int
}
    CreateTenantParams holds the input for CreateTenant.

type DeadLetter struct {
	ID              string     `json:"id"`
	NotificationID  string     `json:"notification_id"`
	Channel         string     `json:"channel"`
	FinalError      string     `json:"final_error"`
	DeadLetteredAt  time.Time  `json:"dead_lettered_at"`
	ReplayedAt      *time.Time `json:"replayed_at"`
	ReplayAttemptID *string    `json:"replay_attempt_id"`
}
    DeadLetter is the durable terminal record for a permanently failed attempt.

type DeliveryAttempt struct {
	ID                 string     `json:"id"`
	NotificationID     string     `json:"notification_id"`
	Channel            string     `json:"channel"`
	AttemptNumber      int        `json:"attempt_number"`
	Status             string     `json:"status"`
	ErrorCode          *string    `json:"error_code"`
	ErrorMessage       *string    `json:"error_message"`
	ProviderMessageID  *string    `json:"provider_message_id"`
	LastError          *string    `json:"last_error"`
	NextRetryAt        *time.Time `json:"next_retry_at"`
	StartedAt          *time.Time `json:"started_at"`
	CompletedAt        *time.Time `json:"completed_at"`
	SentAt             *time.Time `json:"sent_at"`
	FailedAt           *time.Time `json:"failed_at"`
	DispatchEnqueuedAt *time.Time `json:"dispatch_enqueued_at"`
	EnqueueKind        string     `json:"enqueue_kind"`
	ProviderUsed       *string    `json:"provider_used,omitempty"`
	FailoverUsed       bool       `json:"failover_used"`
	DeadLetterID       *string    `json:"dead_letter_id,omitempty"`
	ReplayOfDeadLetter *string    `json:"replay_of_dead_letter_id,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}
    DeliveryAttempt is the durable execution record for one delivery try.

type DeliveryPolicy struct {
	ID                    string    `json:"id"`
	TenantID              *string   `json:"tenant_id,omitempty"`
	Channel               *string   `json:"channel,omitempty"`
	Paused                *bool     `json:"paused,omitempty"`
	FailoverEnabled       *bool     `json:"failover_enabled,omitempty"`
	SchedulingEnabled     *bool     `json:"scheduling_enabled,omitempty"`
	ReplayAllowed         *bool     `json:"replay_allowed,omitempty"`
	MaxAttemptsOverride   *int      `json:"max_attempts_override,omitempty"`
	RetryBaseDelaySeconds *int      `json:"retry_base_delay_seconds,omitempty"`
	RetryMaxDelaySeconds  *int      `json:"retry_max_delay_seconds,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}
    DeliveryPolicy stores one policy row that may contribute to tenant/channel
    policy resolution.

type DispatchIntent struct {
	ID             string     `json:"id"`
	NotificationID string     `json:"notification_id"`
	AttemptID      string     `json:"attempt_id"`
	TenantID       string     `json:"tenant_id"`
	Channel        string     `json:"channel"`
	Source         string     `json:"source"`
	Status         string     `json:"status"`
	LastError      *string    `json:"last_error"`
	CreatedAt      time.Time  `json:"created_at"`
	ClaimedAt      *time.Time `json:"claimed_at"`
	PublishedAt    *time.Time `json:"published_at"`
}
    DispatchIntent is the Stage 8 outbox record that tracks whether an attempt
    still needs Redis publication.

type Notification struct {
	ID                  string         `json:"id"`
	TenantID            string         `json:"tenant_id"`
	TemplateID          string         `json:"template_id"`
	IdempotencyKey      *string        `json:"idempotency_key"`
	Status              string         `json:"status"`
	RecipientEmail      *string        `json:"recipient_email"`
	RecipientWebhookURL *string        `json:"recipient_webhook_url"`
	SecondaryWebhookURL *string        `json:"secondary_webhook_url,omitempty"`
	Variables           map[string]any `json:"variables"`
	ScheduledFor        *time.Time     `json:"scheduled_for,omitempty"`
	PromotedAt          *time.Time     `json:"promoted_at,omitempty"`
	CancelledAt         *time.Time     `json:"cancelled_at,omitempty"`
	SubmittedAt         time.Time      `json:"submitted_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}
    Notification is the durable notification record exposed by the API and
    operator inspection endpoints.

type OperationalMetrics struct {
	OutboxPendingCount        int       `json:"outbox_pending_count"`
	OutboxPublishingCount     int       `json:"outbox_publishing_count"`
	OutboxOldestLagSeconds    int64     `json:"outbox_oldest_lag_seconds"`
	DueRetryCount             int       `json:"due_retry_count"`
	OpenDeadLetterCount       int       `json:"open_dead_letter_count"`
	DuplicateSuppressionCount int       `json:"duplicate_suppression_count"`
	RetryScheduledCount       int       `json:"retry_scheduled_count"`
	DeadLetteredCount         int       `json:"dead_lettered_count"`
	ScheduledPendingCount     int       `json:"scheduled_pending_count"`
	ScheduledDueCount         int       `json:"scheduled_due_count"`
	ScheduledOldestLagSeconds int64     `json:"scheduled_oldest_lag_seconds"`
	CollectedAt               time.Time `json:"collected_at"`
}
    OperationalMetrics is the Stage 10 store-backed metrics snapshot returned by
    the metrics endpoint.

type PendingDispatchIntent struct {
	Intent       DispatchIntent
	DeadLetterID *string
}
    PendingDispatchIntent is the outbox publisher's durable input row.

type PendingEnqueueAttempt struct {
	Attempt      DeliveryAttempt
	TenantID     string
	DeadLetterID *string
}
    PendingEnqueueAttempt exposes the older compatibility view of attempts that
    still need queue publication.

type Postgres struct {
	DB *sql.DB
}
    Postgres wraps the shared sql.DB used by the durable store layer.

func NewPostgres(ctx context.Context, databaseURL string) (*Postgres, error)
    NewPostgres opens the durable Postgres connection used by the runtime.

func (p *Postgres) CancelScheduledNotification(ctx context.Context, notificationID string) (Notification, error)
    CancelScheduledNotification cancels still-future scheduled work that has not
    yet been promoted.

func (p *Postgres) ClaimPendingDispatchIntents(ctx context.Context, limit int, staleAfter time.Duration) ([]PendingDispatchIntent, error)
    ClaimPendingDispatchIntents leases a batch of outbox rows to one publisher.

func (p *Postgres) Close() error
    Close releases the underlying database connection pool.

func (p *Postgres) CollectOperationalMetrics(ctx context.Context, now time.Time) (OperationalMetrics, error)
    CollectOperationalMetrics returns the Stage 10 store-backed metrics snapshot
    used by the metrics endpoint.

func (p *Postgres) CreateDeliveryAttempt(ctx context.Context, params CreateDeliveryAttemptParams) (DeliveryAttempt, error)
    CreateDeliveryAttempt inserts a delivery attempt row.

func (p *Postgres) CreateDispatchIntent(ctx context.Context, params CreateDispatchIntentParams) (DispatchIntent, error)
    CreateDispatchIntent inserts a Stage 8 outbox row.

func (p *Postgres) CreateNotification(ctx context.Context, params CreateNotificationParams) (Notification, error)
    CreateNotification inserts a notification row without creating an initial
    attempt or dispatch intent.

func (p *Postgres) CreateNotificationWithInitialDispatch(ctx context.Context, params CreateNotificationDispatchParams) (Notification, DeliveryAttempt, DispatchIntent, error)
    CreateNotificationWithInitialDispatch atomically creates the notification
    and its initial attempt, plus an initial dispatch intent when the work is
    immediately publishable.

func (p *Postgres) CreateTemplate(ctx context.Context, params CreateTemplateParams) (Template, error)
    CreateTemplate inserts a new template row.

func (p *Postgres) CreateTenant(ctx context.Context, params CreateTenantParams) (Tenant, error)
    CreateTenant inserts a new tenant row.

func (p *Postgres) EnsureInitialAttempt(ctx context.Context, notificationID, channel, attemptID, intentID string) (DeliveryAttempt, DispatchIntent, error)
    EnsureInitialAttempt repairs or recreates the initial attempt and dispatch
    intent for idempotent request recovery paths.

func (p *Postgres) EnsureReplayAttempt(ctx context.Context, deadLetterID, newAttemptID string) (ReplayDeadLetterResult, error)
    EnsureReplayAttempt repairs or recreates the replay attempt and its dispatch
    intent.

func (p *Postgres) EnsureRetryAttempt(ctx context.Context, scheduledAttemptID, newAttemptID string) (RetryDueAttempt, error)
    EnsureRetryAttempt repairs or recreates the retry attempt and its dispatch
    intent.

func (p *Postgres) FinalizeDeadLetterReplay(ctx context.Context, deadLetterID, newAttemptID string) (ReplayDeadLetterResult, error)
    FinalizeDeadLetterReplay is the older replay path that recreates work and
    marks the dead letter replayed in one transaction.

func (p *Postgres) FinalizeReplayEnqueue(ctx context.Context, deadLetterID, attemptID string) error
    FinalizeReplayEnqueue is the older replay compatibility path that marks work
    as enqueued and replayed together.

func (p *Postgres) FinalizeRetryDispatch(ctx context.Context, scheduledAttemptID, newAttemptID string) (RetryDueAttempt, error)
    FinalizeRetryDispatch is the older retry path that creates retry work in one
    transaction.

func (p *Postgres) GetDeadLetterByID(ctx context.Context, id string) (DeadLetter, error)
    GetDeadLetterByID returns one dead-letter row by ID.

func (p *Postgres) GetDeliveryAttemptByID(ctx context.Context, id string) (DeliveryAttempt, error)
    GetDeliveryAttemptByID returns one attempt by ID.

func (p *Postgres) GetDeliveryPolicyByID(ctx context.Context, id string) (DeliveryPolicy, error)
    GetDeliveryPolicyByID returns one policy row by ID.

func (p *Postgres) GetInitialAttemptByNotificationID(ctx context.Context, notificationID string) (DeliveryAttempt, error)
    GetInitialAttemptByNotificationID returns the initial attempt for a
    notification.

func (p *Postgres) GetNotificationByID(ctx context.Context, id string) (Notification, error)
    GetNotificationByID returns one notification by ID.

func (p *Postgres) GetNotificationByTenantAndIdempotencyKey(ctx context.Context, tenantID, idempotencyKey string) (Notification, error)
    GetNotificationByTenantAndIdempotencyKey returns the durable notification
    for a tenant-scoped idempotency key.

func (p *Postgres) GetTemplateByID(ctx context.Context, id string) (Template, error)
    GetTemplateByID returns one template by ID.

func (p *Postgres) GetTenantByID(ctx context.Context, id string) (Tenant, error)
    GetTenantByID returns one tenant by ID.

func (p *Postgres) InsertDeadLetter(ctx context.Context, id, notificationID, channel, finalError string) (DeadLetter, error)
    InsertDeadLetter creates a durable dead-letter row.

func (p *Postgres) ListAttemptsPendingEnqueue(ctx context.Context, limit int) ([]PendingEnqueueAttempt, error)
    ListAttemptsPendingEnqueue exposes the older pre-outbox repair surface for
    compatibility and migration-safe inspection.

func (p *Postgres) ListDeadLetters(ctx context.Context, limit int) ([]DeadLetter, error)
    ListDeadLetters returns recent dead letters for operator inspection.

func (p *Postgres) ListDeliveryAttemptsByNotificationID(ctx context.Context, notificationID string) ([]DeliveryAttempt, error)
    ListDeliveryAttemptsByNotificationID returns all attempts for a notification
    in inspection-friendly order.

func (p *Postgres) ListDeliveryPolicies(ctx context.Context) ([]DeliveryPolicy, error)
    ListDeliveryPolicies returns all durable policy rows.

func (p *Postgres) ListDueRetryAttempts(ctx context.Context, limit int) ([]RetryDueAttempt, error)
    ListDueRetryAttempts returns retry-scheduled attempts whose retry time is
    due.

func (p *Postgres) LoadDeliveryJob(ctx context.Context, notificationID, attemptID string) (Notification, Template, DeliveryAttempt, error)
    LoadDeliveryJob loads the durable notification, template, and attempt needed
    by a channel worker.

func (p *Postgres) MarkAttemptDeadLettered(ctx context.Context, attemptID, lastError string) error
    MarkAttemptDeadLettered finalizes an attempt as dead-lettered.

func (p *Postgres) MarkAttemptEnqueued(ctx context.Context, attemptID string) error
    MarkAttemptEnqueued records that an attempt has been published to Redis.

func (p *Postgres) MarkAttemptFailed(ctx context.Context, attemptID string, lastError string) error
    MarkAttemptFailed finalizes an attempt as failed without retry scheduling.

func (p *Postgres) MarkAttemptInProgress(ctx context.Context, attemptID string) error
    MarkAttemptInProgress atomically transitions an attempt from pending to
    in_progress.

func (p *Postgres) MarkAttemptSent(ctx context.Context, attemptID string, providerMessageID *string) error
    MarkAttemptSent finalizes an attempt as sent.

func (p *Postgres) MarkDispatchIntentPublished(ctx context.Context, intentID string, claimedAt time.Time) error
    MarkDispatchIntentPublished finalizes a claimed dispatch intent as
    published.

func (p *Postgres) Ping(ctx context.Context) error
    Ping verifies that Postgres is reachable.

func (p *Postgres) PromoteDueScheduledNotifications(ctx context.Context, limit int, now time.Time) ([]PromotedScheduledNotification, error)
    PromoteDueScheduledNotifications turns due scheduled notifications into live
    dispatch intents.

func (p *Postgres) RecalculateNotificationStatus(ctx context.Context, notificationID string) error
    RecalculateNotificationStatus recomputes the notification rollup from its
    current attempt state.

func (p *Postgres) RecordAuditEvent(ctx context.Context, id, tenantID, actor, action, resourceType, resourceID string, metadata map[string]any) error
    RecordAuditEvent inserts one durable audit event.

func (p *Postgres) RecordDispatchIntentError(ctx context.Context, intentID string, claimedAt time.Time, lastError string) error
    RecordDispatchIntentError releases a claimed intent back to pending with the
    latest publish failure recorded.

func (p *Postgres) RedriveNotification(ctx context.Context, notificationID string) (PromotedScheduledNotification, error)
    RedriveNotification manually promotes eligible scheduled work into the
    outbox path.

func (p *Postgres) ResolveDeliveryPolicy(ctx context.Context, tenantID, channel string) (ResolvedDeliveryPolicy, error)
    ResolveDeliveryPolicy applies the tenant/channel policy precedence rules and
    returns the effective policy.

func (p *Postgres) RunMaintenance(ctx context.Context, params CleanupParams) (CleanupResult, error)
    RunMaintenance performs one explicit retention-driven cleanup pass.

func (p *Postgres) ScheduleRetry(ctx context.Context, attemptID, lastError string, nextRetryAt time.Time) error
    ScheduleRetry moves an attempt into retry_scheduled and stores the next
    retry timestamp.

func (p *Postgres) SetDeliveryPolicyPaused(ctx context.Context, id string, paused bool) (DeliveryPolicy, error)
    SetDeliveryPolicyPaused updates the paused flag for a policy row.

func (p *Postgres) UpdateAttemptProvider(ctx context.Context, attemptID, provider string, failoverUsed bool) error
    UpdateAttemptProvider stores which provider handled an attempt and whether
    failover was used.

func (p *Postgres) UpsertDeliveryPolicy(ctx context.Context, params UpsertDeliveryPolicyParams) (DeliveryPolicy, error)
    UpsertDeliveryPolicy inserts or updates one delivery policy row.

type PromotedScheduledNotification struct {
	Notification Notification
	Attempt      DeliveryAttempt
	Intent       DispatchIntent
}
    PromotedScheduledNotification is the result of turning scheduled work into
    an active dispatch intent.

type ReplayDeadLetterResult struct {
	DeadLetter DeadLetter
	Attempt    DeliveryAttempt
}
    ReplayDeadLetterResult returns the durable rows produced by a replay path.

type ResolvedDeliveryPolicy struct {
	TenantID              string `json:"tenant_id"`
	Channel               string `json:"channel"`
	Paused                bool   `json:"paused"`
	FailoverEnabled       bool   `json:"failover_enabled"`
	SchedulingEnabled     bool   `json:"scheduling_enabled"`
	ReplayAllowed         bool   `json:"replay_allowed"`
	MaxAttemptsOverride   *int   `json:"max_attempts_override,omitempty"`
	RetryBaseDelaySeconds *int   `json:"retry_base_delay_seconds,omitempty"`
	RetryMaxDelaySeconds  *int   `json:"retry_max_delay_seconds,omitempty"`
}
    ResolvedDeliveryPolicy is the final policy view after precedence and
    override rules have been applied.

type RetryDueAttempt struct {
	Attempt        DeliveryAttempt
	NotificationID string
	TenantID       string
}
    RetryDueAttempt is the retry scheduler's durable input row.

type Template struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Name      string    `json:"name"`
	Channel   string    `json:"channel"`
	Version   int       `json:"version"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}
    Template is the durable template record stored in Postgres.

type Tenant struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	DailyQuota int       `json:"daily_quota"`
	CreatedAt  time.Time `json:"created_at"`
}
    Tenant is the durable tenant record stored in Postgres.

type UpsertDeliveryPolicyParams struct {
	ID                    string
	TenantID              *string
	Channel               *string
	Paused                *bool
	FailoverEnabled       *bool
	SchedulingEnabled     *bool
	ReplayAllowed         *bool
	MaxAttemptsOverride   *int
	RetryBaseDelaySeconds *int
	RetryMaxDelaySeconds  *int
}
    UpsertDeliveryPolicyParams holds the mutable policy fields for upsert.

```
