package queue

import (
	"context"
	"time"
)

type TenantRateLimiter struct {
	redis  *RedisQueue
	limit  int
	window time.Duration
}

func NewTenantRateLimiter(redis *RedisQueue, limit int, window time.Duration) *TenantRateLimiter {
	return &TenantRateLimiter{redis: redis, limit: limit, window: window}
}

func (l *TenantRateLimiter) Allow(ctx context.Context, tenantID string) (bool, time.Duration, error) {
	if l == nil || l.redis == nil {
		return true, 0, nil
	}
	return l.redis.AllowTenant(ctx, tenantID, l.limit, l.window)
}
