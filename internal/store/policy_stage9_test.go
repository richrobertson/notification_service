package store

import "testing"

func TestApplyPolicyRowResolutionOrder(t *testing.T) {
	resolved := ResolvedDeliveryPolicy{
		TenantID:          "tenant-1",
		Channel:           "email",
		SchedulingEnabled: true,
		ReplayAllowed:     true,
	}
	globalRetry := 30
	tenantRetry := 10
	channelFailover := true
	tenantPaused := true

	applyPolicyRow(&resolved, DeliveryPolicy{RetryBaseDelaySeconds: &globalRetry})
	applyPolicyRow(&resolved, DeliveryPolicy{FailoverEnabled: &channelFailover})
	applyPolicyRow(&resolved, DeliveryPolicy{RetryBaseDelaySeconds: &tenantRetry, Paused: &tenantPaused})

	if resolved.RetryBaseDelaySeconds == nil || *resolved.RetryBaseDelaySeconds != 10 {
		t.Fatalf("retry_base_delay_seconds=%v", resolved.RetryBaseDelaySeconds)
	}
	if !resolved.FailoverEnabled {
		t.Fatal("expected failover_enabled=true")
	}
	if !resolved.Paused {
		t.Fatal("expected paused=true")
	}
}

func TestNewerSameScopePolicyWins(t *testing.T) {
	resolved := ResolvedDeliveryPolicy{
		TenantID:          "tenant-1",
		Channel:           "email",
		SchedulingEnabled: true,
		ReplayAllowed:     true,
	}
	olderRetry := 30
	newerRetry := 10
	ordered := []DeliveryPolicy{
		{RetryBaseDelaySeconds: &newerRetry},
		{RetryBaseDelaySeconds: &olderRetry},
	}

	for i := len(ordered) - 1; i >= 0; i-- {
		applyPolicyRow(&resolved, ordered[i])
	}

	if resolved.RetryBaseDelaySeconds == nil || *resolved.RetryBaseDelaySeconds != 10 {
		t.Fatalf("retry_base_delay_seconds=%v", resolved.RetryBaseDelaySeconds)
	}
}
