package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	AppName                string
	HTTPPort               string
	LogLevel               string
	DatabaseURL            string
	OTLPEndpoint           string
	Environment            string
	RedisAddr              string
	RedisPassword          string
	RedisDB                int
	WebhookTimeout         time.Duration
	QueueBlockTimeout      time.Duration
	SMTPHost               string
	SMTPPort               int
	SMTPUsername           string
	SMTPPassword           string
	SMTPFrom               string
	SMTPUseTLS             bool
	SMTPStartTLS           bool
	SMTPInsecureSkipVerify bool
}

func Load() Config {
	return Config{
		AppName:                envOrDefault("APP_NAME", "notification-platform-api"),
		HTTPPort:               envOrDefault("HTTP_PORT", "8080"),
		LogLevel:               envOrDefault("LOG_LEVEL", "debug"),
		DatabaseURL:            envOrDefault("DATABASE_URL", "postgres://notification:notification@localhost:5432/notification_platform?sslmode=disable"),
		OTLPEndpoint:           envOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
		Environment:            envOrDefault("ENVIRONMENT", "local"),
		RedisAddr:              envOrDefault("REDIS_ADDR", "localhost:6379"),
		RedisPassword:          envOrDefault("REDIS_PASSWORD", ""),
		RedisDB:                envIntOrDefault("REDIS_DB", 0),
		WebhookTimeout:         envDurationOrDefault("WEBHOOK_TIMEOUT", 5*time.Second),
		QueueBlockTimeout:      envDurationOrDefault("QUEUE_BLOCK_TIMEOUT", time.Second),
		SMTPHost:               envOrDefault("SMTP_HOST", "localhost"),
		SMTPPort:               envIntOrDefault("SMTP_PORT", 1025),
		SMTPUsername:           envOrDefault("SMTP_USERNAME", ""),
		SMTPPassword:           envOrDefault("SMTP_PASSWORD", ""),
		SMTPFrom:               envOrDefault("SMTP_FROM", "notifications@example.test"),
		SMTPUseTLS:             envBoolOrDefault("SMTP_USE_TLS", false),
		SMTPStartTLS:           envBoolOrDefault("SMTP_STARTTLS", false),
		SMTPInsecureSkipVerify: envBoolOrDefault("SMTP_INSECURE_SKIP_VERIFY", false),
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
