package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/richrobertson/notification-platform/internal/config"
	"github.com/richrobertson/notification-platform/internal/store"
)

type fakeMaintenanceStore struct {
	params store.CleanupParams
	result store.CleanupResult
	err    error
	called bool
}

func (f *fakeMaintenanceStore) RunMaintenance(_ context.Context, params store.CleanupParams) (store.CleanupResult, error) {
	f.called = true
	f.params = params
	return f.result, f.err
}

func TestRunMaintenancePassesConfiguredRetention(t *testing.T) {
	now := time.Now().UTC()
	st := &fakeMaintenanceStore{
		result: store.CleanupResult{DryRun: true, AuditEventsDeleted: 3, PublishedOutboxDeleted: 2, DeadLettersDeleted: 1},
	}
	cfg := config.Load()
	cfg.MaintenanceAuditRetention = 24 * time.Hour
	cfg.MaintenanceOutboxRetention = 48 * time.Hour
	cfg.MaintenanceDeadLetterRetention = 72 * time.Hour
	cfg.MaintenanceDryRun = true
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var out bytes.Buffer

	if err := run(context.Background(), logger, st, cfg, now, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !st.called {
		t.Fatal("expected maintenance store to be called")
	}
	if st.params.AuditRetention != cfg.MaintenanceAuditRetention || st.params.OutboxRetention != cfg.MaintenanceOutboxRetention || st.params.DeadLetterRetention != cfg.MaintenanceDeadLetterRetention {
		t.Fatalf("unexpected params: %+v", st.params)
	}
	if out.Len() == 0 {
		t.Fatal("expected maintenance output JSON")
	}
}

func TestRunMaintenanceReturnsStoreError(t *testing.T) {
	st := &fakeMaintenanceStore{err: context.DeadlineExceeded}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	err := run(context.Background(), logger, st, config.Load(), time.Now().UTC(), &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
}
