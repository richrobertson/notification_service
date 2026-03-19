package config

import (
	"os"
	"strconv"
)

type Config struct {
	AppName       string
	HTTPPort      string
	LogLevel      string
	DatabaseURL   string
	OTLPEndpoint  string
	Environment   string
	RedisAddr     string
	RedisPassword string
	RedisDB       int
}

func Load() Config {
	return Config{
		AppName:       envOrDefault("APP_NAME", "notification-platform-api"),
		HTTPPort:      envOrDefault("HTTP_PORT", "8080"),
		LogLevel:      envOrDefault("LOG_LEVEL", "debug"),
		DatabaseURL:   envOrDefault("DATABASE_URL", "postgres://notification:notification@localhost:5432/notification_platform?sslmode=disable"),
		OTLPEndpoint:  envOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
		Environment:   envOrDefault("ENVIRONMENT", "local"),
		RedisAddr:     envOrDefault("REDIS_ADDR", "localhost:6379"),
		RedisPassword: envOrDefault("REDIS_PASSWORD", ""),
		RedisDB:       envIntOrDefault("REDIS_DB", 0),
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
