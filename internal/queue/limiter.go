package queue

import (
	"context"
	"time"
)

// TenantRateLimiter is the Redis-backed fixed-window limiter used by the API.
type TenantRateLimiter struct {
	redis  *RedisQueue
	limit  int
	window time.Duration
}

// NewTenantRateLimiter builds the Stage 7 fixed-window rate limiter backed by
// Redis counters.
func NewTenantRateLimiter(redis *RedisQueue, limit int, window time.Duration) *TenantRateLimiter {
	return &TenantRateLimiter{redis: redis, limit: limit, window: window}
}

// Allow reports whether the tenant is still within the configured request
// budget and, when rejected, how long the caller should wait before retrying.
func (l *TenantRateLimiter) Allow(ctx context.Context, tenantID string) (bool, time.Duration, error) {
	if l == nil || l.redis == nil {
		return true, 0, nil
	}
	return l.redis.AllowTenant(ctx, tenantID, l.limit, l.window)
}
