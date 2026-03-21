package pressure

import (
	"context"
	"testing"
	"time"

	"github.com/richrobertson/notification-platform/internal/queue"
)

func TestMonitorSnapshotNilReceiver(t *testing.T) {
	var m *Monitor
	snapshot, err := m.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if snapshot.Depths != nil || snapshot.SoftLimit != 0 || snapshot.HardLimit != 0 || snapshot.RetryAfter != 0 {
		t.Fatalf("Snapshot() = %+v, want zero value", snapshot)
	}
}

func TestMonitorSnapshotAppliesRetryAfterWhenQueueLeavesItUnset(t *testing.T) {
	m := NewMonitor(fakeSnapshotter{snapshot: queue.PressureSnapshot{Depths: map[string]int{"q": 1}}}, 5, 10, 3*time.Second)
	snapshot, err := m.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if snapshot.RetryAfter != 3*time.Second {
		t.Fatalf("RetryAfter = %v, want %v", snapshot.RetryAfter, 3*time.Second)
	}
}

type fakeSnapshotter struct {
	snapshot queue.PressureSnapshot
}

func (f fakeSnapshotter) PressureSnapshot(context.Context) (queue.PressureSnapshot, error) {
	return f.snapshot, nil
}
