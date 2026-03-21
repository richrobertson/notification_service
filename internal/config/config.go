package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	AppName                  string
	HTTPPort                 string
	LogLevel                 string
	DatabaseURL              string
	OTLPEndpoint             string
	Environment              string
	RedisAddr                string
	RedisPassword            string
	RedisDB                  int
	WebhookTimeout           time.Duration
	QueueBlockTimeout        time.Duration
	RetryMaxAttempts         int
	RetryBaseDelay           time.Duration
	RetryMaxDelay            time.Duration
	RetryExponentialBackoff  bool
	RetryJitter              time.Duration
	RetryWorkerPollInterval  time.Duration
	RecoveryInterval         time.Duration
	SMTPHost                 string
	SMTPPort                 int
	SMTPUsername             string
	SMTPPassword             string
	SMTPFrom                 string
	SMTPUseTLS               bool
	SMTPStartTLS             bool
	SMTPInsecureSkipVerify   bool
	APIRateLimitPerSecond    int
	APIRateLimitWindow       time.Duration
	QueueSoftLimit           int
	QueueHardLimit           int
	BackpressureRetryAfter   time.Duration
	DispatcherConcurrency    int
	EmailWorkerConcurrency   int
	WebhookWorkerConcurrency int
	PerTenantWorkerBurst     int
	PerTenantMaxInFlight     int
	RetryPressureMultiplier  int
	RetryPressureMinDelay    time.Duration
}

func Load() Config {
	return Config{
		AppName:                  envOrDefault("APP_NAME", "notification-platform-api"),
		HTTPPort:                 envOrDefault("HTTP_PORT", "8080"),
		LogLevel:                 envOrDefault("LOG_LEVEL", "debug"),
		DatabaseURL:              envOrDefault("DATABASE_URL", "postgres://notification:notification@localhost:5432/notification_platform?sslmode=disable"),
		OTLPEndpoint:             envOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
		Environment:              envOrDefault("ENVIRONMENT", "local"),
		RedisAddr:                envOrDefault("REDIS_ADDR", "localhost:6379"),
		RedisPassword:            envOrDefault("REDIS_PASSWORD", ""),
		RedisDB:                  envIntOrDefault("REDIS_DB", 0),
		WebhookTimeout:           envDurationOrDefault("WEBHOOK_TIMEOUT", 5*time.Second),
		QueueBlockTimeout:        envDurationOrDefault("QUEUE_BLOCK_TIMEOUT", time.Second),
		RetryMaxAttempts:         envIntOrDefault("RETRY_MAX_ATTEMPTS", 3),
		RetryBaseDelay:           envDurationOrDefault("RETRY_BASE_DELAY", 5*time.Second),
		RetryMaxDelay:            envDurationOrDefault("RETRY_MAX_DELAY", time.Minute),
		RetryExponentialBackoff:  envBoolOrDefault("RETRY_EXPONENTIAL_BACKOFF", true),
		RetryJitter:              envDurationOrDefault("RETRY_JITTER", time.Second),
		RetryWorkerPollInterval:  envDurationOrDefault("RETRY_WORKER_POLL_INTERVAL", 2*time.Second),
		RecoveryInterval:         envDurationOrDefault("PROCESSING_RECOVERY_INTERVAL", 30*time.Second),
		SMTPHost:                 envOrDefault("SMTP_HOST", "localhost"),
		SMTPPort:                 envIntOrDefault("SMTP_PORT", 1025),
		SMTPUsername:             envOrDefault("SMTP_USERNAME", ""),
		SMTPPassword:             envOrDefault("SMTP_PASSWORD", ""),
		SMTPFrom:                 envOrDefault("SMTP_FROM", "notifications@example.test"),
		SMTPUseTLS:               envBoolOrDefault("SMTP_USE_TLS", false),
		SMTPStartTLS:             envBoolOrDefault("SMTP_STARTTLS", false),
		SMTPInsecureSkipVerify:   envBoolOrDefault("SMTP_INSECURE_SKIP_VERIFY", false),
		APIRateLimitPerSecond:    envIntOrDefault("API_RATE_LIMIT_PER_SECOND", 10),
		APIRateLimitWindow:       envDurationOrDefault("API_RATE_LIMIT_WINDOW", time.Second),
		QueueSoftLimit:           envIntOrDefault("QUEUE_SOFT_LIMIT", 100),
		QueueHardLimit:           envIntOrDefault("QUEUE_HARD_LIMIT", 250),
		BackpressureRetryAfter:   envDurationOrDefault("BACKPRESSURE_RETRY_AFTER", 2*time.Second),
		DispatcherConcurrency:    envIntOrDefault("DISPATCHER_CONCURRENCY", 2),
		EmailWorkerConcurrency:   envIntOrDefault("EMAIL_WORKER_CONCURRENCY", 4),
		WebhookWorkerConcurrency: envIntOrDefault("WEBHOOK_WORKER_CONCURRENCY", 8),
		PerTenantWorkerBurst:     envIntOrDefault("PER_TENANT_WORKER_BURST", 1),
		PerTenantMaxInFlight:     envIntOrDefault("PER_TENANT_MAX_IN_FLIGHT", 2),
		RetryPressureMultiplier:  envIntOrDefault("RETRY_PRESSURE_MULTIPLIER", 2),
		RetryPressureMinDelay:    envDurationOrDefault("RETRY_PRESSURE_MIN_DELAY", 15*time.Second),
	}
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
