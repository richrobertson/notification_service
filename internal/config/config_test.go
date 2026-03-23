package config

import (
	"strings"
	"testing"
	"time"
)

func TestValidateRejectsInvalidRedisAddr(t *testing.T) {
	cfg := Load()
	cfg.RedisAddr = "not-a-hostport"

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "REDIS_ADDR") {
		t.Fatalf("expected REDIS_ADDR validation error, got %v", err)
	}
}

func TestValidateRejectsInvalidRetryRange(t *testing.T) {
	cfg := Load()
	cfg.RetryBaseDelay = 10 * time.Second
	cfg.RetryMaxDelay = 5 * time.Second

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "RETRY_MAX_DELAY") {
		t.Fatalf("expected retry validation error, got %v", err)
	}
}

func TestValidateRejectsInvalidHTTPPort(t *testing.T) {
	cfg := Load()
	cfg.HTTPPort = "70000"

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "HTTP_PORT") {
		t.Fatalf("expected HTTP_PORT validation error, got %v", err)
	}
}

func TestValidateForAPIRequiresAdminTokenOutsideLocal(t *testing.T) {
	cfg := Load()
	cfg.Environment = "prod"
	cfg.AdminToken = ""

	err := cfg.ValidateForAPI()
	if err == nil || !strings.Contains(err.Error(), "ADMIN_TOKEN") {
		t.Fatalf("expected ADMIN_TOKEN validation error, got %v", err)
	}
}
