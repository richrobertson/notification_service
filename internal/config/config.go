package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config contains the process-level runtime settings used by the API and
// workers.
type Config struct {
	AppName                         string
	HTTPPort                        string
	LogLevel                        string
	DatabaseURL                     string
	OTLPEndpoint                    string
	Environment                     string
	RedisAddr                       string
	RedisPassword                   string
	RedisDB                         int
	HTTPReadHeaderTimeout           time.Duration
	HTTPReadTimeout                 time.Duration
	HTTPWriteTimeout                time.Duration
	HTTPIdleTimeout                 time.Duration
	HTTPShutdownTimeout             time.Duration
	MaxRequestBodyBytes             int64
	AdminToken                      string
	WebhookTimeout                  time.Duration
	QueueBlockTimeout               time.Duration
	RetryMaxAttempts                int
	RetryBaseDelay                  time.Duration
	RetryMaxDelay                   time.Duration
	RetryExponentialBackoff         bool
	RetryJitter                     time.Duration
	RetryWorkerPollInterval         time.Duration
	OutboxPollInterval              time.Duration
	SchedulerPollInterval           time.Duration
	RecoveryInterval                time.Duration
	SMTPHost                        string
	SMTPPort                        int
	SMTPUsername                    string
	SMTPPassword                    string
	SMTPFrom                        string
	SMTPUseTLS                      bool
	SMTPStartTLS                    bool
	SMTPInsecureSkipVerify          bool
	SecondarySMTPHost               string
	SecondarySMTPPort               int
	SecondarySMTPUsername           string
	SecondarySMTPPassword           string
	SecondarySMTPFrom               string
	SecondarySMTPUseTLS             bool
	SecondarySMTPStartTLS           bool
	SecondarySMTPInsecureSkipVerify bool
	APIRateLimitPerSecond           int
	APIRateLimitWindow              time.Duration
	QueueSoftLimit                  int
	QueueHardLimit                  int
	BackpressureRetryAfter          time.Duration
	DispatcherConcurrency           int
	EmailWorkerConcurrency          int
	WebhookWorkerConcurrency        int
	PerTenantWorkerBurst            int
	PerTenantMaxInFlight            int
	RetryPressureMultiplier         int
	RetryPressureMinDelay           time.Duration
	MaintenanceAuditRetention       time.Duration
	MaintenanceOutboxRetention      time.Duration
	MaintenanceDeadLetterRetention  time.Duration
	MaintenanceDryRun               bool
}

// Load reads configuration from environment variables and applies pragmatic
// local-development defaults.
//
// The returned value is intentionally usable before validation so commands can
// tailor app names or command-specific behavior and then call Validate or
// ValidateForAPI.
func Load() Config {
	cfg := Config{
		AppName:                         envOrDefault("APP_NAME", "notification-platform-api"),
		HTTPPort:                        envOrDefault("HTTP_PORT", "8080"),
		LogLevel:                        envOrDefault("LOG_LEVEL", "debug"),
		DatabaseURL:                     envOrDefault("DATABASE_URL", "postgres://notification:notification@localhost:5432/notification_platform?sslmode=disable"),
		OTLPEndpoint:                    envOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
		Environment:                     envOrDefault("ENVIRONMENT", "local"),
		RedisAddr:                       envOrDefault("REDIS_ADDR", "localhost:6379"),
		RedisPassword:                   envOrDefault("REDIS_PASSWORD", ""),
		RedisDB:                         envIntOrDefault("REDIS_DB", 0),
		HTTPReadHeaderTimeout:           envDurationOrDefault("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
		HTTPReadTimeout:                 envDurationOrDefault("HTTP_READ_TIMEOUT", 10*time.Second),
		HTTPWriteTimeout:                envDurationOrDefault("HTTP_WRITE_TIMEOUT", 15*time.Second),
		HTTPIdleTimeout:                 envDurationOrDefault("HTTP_IDLE_TIMEOUT", 60*time.Second),
		HTTPShutdownTimeout:             envDurationOrDefault("HTTP_SHUTDOWN_TIMEOUT", 10*time.Second),
		MaxRequestBodyBytes:             envInt64OrDefault("HTTP_MAX_REQUEST_BODY_BYTES", 1<<20),
		AdminToken:                      envOrDefault("ADMIN_TOKEN", ""),
		WebhookTimeout:                  envDurationOrDefault("WEBHOOK_TIMEOUT", 5*time.Second),
		QueueBlockTimeout:               envDurationOrDefault("QUEUE_BLOCK_TIMEOUT", time.Second),
		RetryMaxAttempts:                envIntOrDefault("RETRY_MAX_ATTEMPTS", 3),
		RetryBaseDelay:                  envDurationOrDefault("RETRY_BASE_DELAY", 5*time.Second),
		RetryMaxDelay:                   envDurationOrDefault("RETRY_MAX_DELAY", time.Minute),
		RetryExponentialBackoff:         envBoolOrDefault("RETRY_EXPONENTIAL_BACKOFF", true),
		RetryJitter:                     envDurationOrDefault("RETRY_JITTER", time.Second),
		RetryWorkerPollInterval:         envDurationOrDefault("RETRY_WORKER_POLL_INTERVAL", 2*time.Second),
		OutboxPollInterval:              envDurationOrDefault("OUTBOX_POLL_INTERVAL", 2*time.Second),
		SchedulerPollInterval:           envDurationOrDefault("SCHEDULER_POLL_INTERVAL", 2*time.Second),
		RecoveryInterval:                envDurationOrDefault("PROCESSING_RECOVERY_INTERVAL", 30*time.Second),
		SMTPHost:                        envOrDefault("SMTP_HOST", "localhost"),
		SMTPPort:                        envIntOrDefault("SMTP_PORT", 1025),
		SMTPUsername:                    envOrDefault("SMTP_USERNAME", ""),
		SMTPPassword:                    envOrDefault("SMTP_PASSWORD", ""),
		SMTPFrom:                        envOrDefault("SMTP_FROM", "notifications@example.test"),
		SMTPUseTLS:                      envBoolOrDefault("SMTP_USE_TLS", false),
		SMTPStartTLS:                    envBoolOrDefault("SMTP_STARTTLS", false),
		SMTPInsecureSkipVerify:          envBoolOrDefault("SMTP_INSECURE_SKIP_VERIFY", false),
		SecondarySMTPHost:               envOrDefault("SMTP_SECONDARY_HOST", ""),
		SecondarySMTPPort:               envIntOrDefault("SMTP_SECONDARY_PORT", 0),
		SecondarySMTPUsername:           envOrDefault("SMTP_SECONDARY_USERNAME", ""),
		SecondarySMTPPassword:           envOrDefault("SMTP_SECONDARY_PASSWORD", ""),
		SecondarySMTPFrom:               envOrDefault("SMTP_SECONDARY_FROM", ""),
		SecondarySMTPUseTLS:             envBoolOrDefault("SMTP_SECONDARY_USE_TLS", false),
		SecondarySMTPStartTLS:           envBoolOrDefault("SMTP_SECONDARY_STARTTLS", false),
		SecondarySMTPInsecureSkipVerify: envBoolOrDefault("SMTP_SECONDARY_INSECURE_SKIP_VERIFY", false),
		APIRateLimitPerSecond:           envIntOrDefault("API_RATE_LIMIT_PER_SECOND", 10),
		APIRateLimitWindow:              envDurationOrDefault("API_RATE_LIMIT_WINDOW", time.Second),
		QueueSoftLimit:                  envIntOrDefault("QUEUE_SOFT_LIMIT", 100),
		QueueHardLimit:                  envIntOrDefault("QUEUE_HARD_LIMIT", 250),
		BackpressureRetryAfter:          envDurationOrDefault("BACKPRESSURE_RETRY_AFTER", 2*time.Second),
		DispatcherConcurrency:           envIntOrDefault("DISPATCHER_CONCURRENCY", 2),
		EmailWorkerConcurrency:          envIntOrDefault("EMAIL_WORKER_CONCURRENCY", 4),
		WebhookWorkerConcurrency:        envIntOrDefault("WEBHOOK_WORKER_CONCURRENCY", 8),
		PerTenantWorkerBurst:            envIntOrDefault("PER_TENANT_WORKER_BURST", 1),
		PerTenantMaxInFlight:            envIntOrDefault("PER_TENANT_MAX_IN_FLIGHT", 2),
		RetryPressureMultiplier:         envIntOrDefault("RETRY_PRESSURE_MULTIPLIER", 2),
		RetryPressureMinDelay:           envDurationOrDefault("RETRY_PRESSURE_MIN_DELAY", 15*time.Second),
		MaintenanceAuditRetention:       envDurationOrDefault("MAINTENANCE_AUDIT_RETENTION", 30*24*time.Hour),
		MaintenanceOutboxRetention:      envDurationOrDefault("MAINTENANCE_OUTBOX_RETENTION", 7*24*time.Hour),
		MaintenanceDeadLetterRetention:  envDurationOrDefault("MAINTENANCE_DEAD_LETTER_RETENTION", 0),
		MaintenanceDryRun:               envBoolOrDefault("MAINTENANCE_DRY_RUN", true),
	}

	if cfg.Environment == "local" && strings.TrimSpace(cfg.AdminToken) == "" {
		cfg.AdminToken = "dev-admin-token"
	}

	return cfg
}

// Validate checks that the configuration is internally consistent and safe to
// use for a long-running process.
//
// The method validates both individual fields and important cross-field
// invariants such as retry ranges, queue limits, and timeout ordering.
func (c Config) Validate() error {
	if strings.TrimSpace(c.AppName) == "" {
		return fmt.Errorf("APP_NAME must not be empty")
	}
	if strings.TrimSpace(c.HTTPPort) == "" {
		return fmt.Errorf("HTTP_PORT must not be empty")
	}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return fmt.Errorf("DATABASE_URL must not be empty")
	}
	if _, err := url.ParseRequestURI(c.DatabaseURL); err != nil {
		return fmt.Errorf("DATABASE_URL must be a valid URL: %w", err)
	}
	if strings.TrimSpace(c.RedisAddr) == "" {
		return fmt.Errorf("REDIS_ADDR must not be empty")
	}
	if _, _, err := net.SplitHostPort(c.RedisAddr); err != nil {
		return fmt.Errorf("REDIS_ADDR must be in host:port form: %w", err)
	}
	if err := positiveDuration("WEBHOOK_TIMEOUT", c.WebhookTimeout); err != nil {
		return err
	}
	if err := positiveDuration("QUEUE_BLOCK_TIMEOUT", c.QueueBlockTimeout); err != nil {
		return err
	}
	if err := positiveDuration("RETRY_BASE_DELAY", c.RetryBaseDelay); err != nil {
		return err
	}
	if err := positiveDuration("RETRY_MAX_DELAY", c.RetryMaxDelay); err != nil {
		return err
	}
	if err := positiveDuration("RETRY_WORKER_POLL_INTERVAL", c.RetryWorkerPollInterval); err != nil {
		return err
	}
	if err := positiveDuration("OUTBOX_POLL_INTERVAL", c.OutboxPollInterval); err != nil {
		return err
	}
	if err := positiveDuration("SCHEDULER_POLL_INTERVAL", c.SchedulerPollInterval); err != nil {
		return err
	}
	if err := positiveDuration("PROCESSING_RECOVERY_INTERVAL", c.RecoveryInterval); err != nil {
		return err
	}
	if err := positiveDuration("API_RATE_LIMIT_WINDOW", c.APIRateLimitWindow); err != nil {
		return err
	}
	if err := positiveDuration("BACKPRESSURE_RETRY_AFTER", c.BackpressureRetryAfter); err != nil {
		return err
	}
	if err := positiveDuration("RETRY_PRESSURE_MIN_DELAY", c.RetryPressureMinDelay); err != nil {
		return err
	}
	if err := positiveDuration("HTTP_READ_HEADER_TIMEOUT", c.HTTPReadHeaderTimeout); err != nil {
		return err
	}
	if err := positiveDuration("HTTP_READ_TIMEOUT", c.HTTPReadTimeout); err != nil {
		return err
	}
	if err := positiveDuration("HTTP_WRITE_TIMEOUT", c.HTTPWriteTimeout); err != nil {
		return err
	}
	if err := positiveDuration("HTTP_IDLE_TIMEOUT", c.HTTPIdleTimeout); err != nil {
		return err
	}
	if err := positiveDuration("HTTP_SHUTDOWN_TIMEOUT", c.HTTPShutdownTimeout); err != nil {
		return err
	}
	if err := positiveDuration("MAINTENANCE_AUDIT_RETENTION", c.MaintenanceAuditRetention); err != nil {
		return err
	}
	if err := positiveDuration("MAINTENANCE_OUTBOX_RETENTION", c.MaintenanceOutboxRetention); err != nil {
		return err
	}
	if c.MaintenanceDeadLetterRetention < 0 {
		return fmt.Errorf("MAINTENANCE_DEAD_LETTER_RETENTION must be >= 0")
	}
	if c.RetryMaxAttempts <= 0 {
		return fmt.Errorf("RETRY_MAX_ATTEMPTS must be greater than 0")
	}
	if c.RetryMaxDelay < c.RetryBaseDelay {
		return fmt.Errorf("RETRY_MAX_DELAY must be greater than or equal to RETRY_BASE_DELAY")
	}
	if c.APIRateLimitPerSecond <= 0 {
		return fmt.Errorf("API_RATE_LIMIT_PER_SECOND must be greater than 0")
	}
	if c.QueueSoftLimit <= 0 {
		return fmt.Errorf("QUEUE_SOFT_LIMIT must be greater than 0")
	}
	if c.QueueHardLimit <= 0 {
		return fmt.Errorf("QUEUE_HARD_LIMIT must be greater than 0")
	}
	if c.QueueHardLimit < c.QueueSoftLimit {
		return fmt.Errorf("QUEUE_HARD_LIMIT must be greater than or equal to QUEUE_SOFT_LIMIT")
	}
	if c.DispatcherConcurrency <= 0 {
		return fmt.Errorf("DISPATCHER_CONCURRENCY must be greater than 0")
	}
	if c.EmailWorkerConcurrency <= 0 {
		return fmt.Errorf("EMAIL_WORKER_CONCURRENCY must be greater than 0")
	}
	if c.WebhookWorkerConcurrency <= 0 {
		return fmt.Errorf("WEBHOOK_WORKER_CONCURRENCY must be greater than 0")
	}
	if c.PerTenantWorkerBurst <= 0 {
		return fmt.Errorf("PER_TENANT_WORKER_BURST must be greater than 0")
	}
	if c.PerTenantMaxInFlight <= 0 {
		return fmt.Errorf("PER_TENANT_MAX_IN_FLIGHT must be greater than 0")
	}
	if c.RetryPressureMultiplier <= 0 {
		return fmt.Errorf("RETRY_PRESSURE_MULTIPLIER must be greater than 0")
	}
	if c.MaxRequestBodyBytes <= 0 {
		return fmt.Errorf("HTTP_MAX_REQUEST_BODY_BYTES must be greater than 0")
	}
	if c.HTTPWriteTimeout < c.HTTPReadTimeout {
		return fmt.Errorf("HTTP_WRITE_TIMEOUT must be greater than or equal to HTTP_READ_TIMEOUT")
	}
	if c.SMTPPort < 0 {
		return fmt.Errorf("SMTP_PORT must be >= 0")
	}
	if c.SecondarySMTPHost != "" && c.SecondarySMTPPort <= 0 {
		return fmt.Errorf("SMTP_SECONDARY_PORT must be greater than 0 when SMTP_SECONDARY_HOST is set")
	}
	return nil
}

// ValidateForAPI applies the shared validation rules and then enforces the
// additional operator-token requirement used by the HTTP API.
func (c Config) ValidateForAPI() error {
	if err := c.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(c.AdminToken) == "" {
		return fmt.Errorf("ADMIN_TOKEN must not be empty")
	}
	return nil
}

func positiveDuration(name string, value time.Duration) error {
	if value <= 0 {
		return fmt.Errorf("%s must be greater than 0", name)
	}
	return nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

func envIntOrDefault(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func envInt64OrDefault(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}

	return parsed
}

func envBoolOrDefault(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDurationOrDefault(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
