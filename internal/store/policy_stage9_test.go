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
