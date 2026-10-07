package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"cg/internal/config"
	"cg/internal/probe"
)

func TestCatalogApprovalsAndRevisionIsolation(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()
	cfg := config.ProviderConfig{ID: "p", ConnectionRevision: "v1"}
	check := func(models, want []string) {
		t.Helper()
		got, err := s.ObserveCatalog(ctx, cfg, models)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("models=%v err=%v; want %v", got, err, want)
		}
	}
	check([]string{"a"}, []string{"a"})
	check([]string{"a", "b"}, []string{"a"})
	catalogs, err := s.Catalogs(ctx)
	if err != nil || len(catalogs) != 1 || !reflect.DeepEqual(catalogs[0].Added, []string{"b"}) {
		t.Fatalf("catalogs=%+v err=%v", catalogs, err)
	}
	if err := s.ApproveCatalog(ctx, "p", "stale", catalogs[0].UpdatedAt); err == nil {
		t.Fatal("approved stale revision")
	}
	if err := s.ApproveCatalog(ctx, "p", "v1", catalogs[0].UpdatedAt); err != nil {
		t.Fatal(err)
	}
	check([]string{"a", "b"}, []string{"a", "b"})
	check([]string{"b"}, []string{"b"})
	events, err := s.CatalogEvents(ctx)
	if err != nil || len(events) != 2 || events[0].ID == 0 || events[0].CreatedAt == "" || len(events[0].Removed) != 1 {
		t.Fatalf("events=%+v err=%v", events, err)
	}
	cfg.ConnectionRevision = "v2"
	check([]string{"c"}, []string{"c"})
}

func TestCatalogPreservesDiscoveryOrderAndPreviouslyApprovedModels(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()
	cfg := config.ProviderConfig{ID: "p", ConnectionRevision: "one"}
	check := func(models, expected []string) {
		t.Helper()
		got, err := s.ObserveCatalog(ctx, cfg, models)
		if err != nil || !reflect.DeepEqual(got, expected) {
			t.Fatalf("models=%v err=%v; want %v", got, err, expected)
		}
	}
	check([]string{"z", "a"}, []string{"z", "a"})
	check([]string{"a", "b"}, []string{"a"})
	catalogs, err := s.Catalogs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApproveCatalog(ctx, cfg.ID, cfg.ConnectionRevision, catalogs[0].UpdatedAt); err != nil {
		t.Fatal(err)
	}
	check([]string{"z", "b", "a"}, []string{"z", "b", "a"})
}

func TestMonitoringSettingsSecretsAndVersion(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()
	value, _ := s.MonitoringSettings(ctx)
	value.Rules = []config.AlertRule{{ID: "production", Name: "prod", Enabled: true, Platform: "webhook", URL: "https://example.invalid/secret", FailureThreshold: 2, RecoveryThreshold: 2}}
	safe, err := s.SaveMonitoringSettings(ctx, value)
	if err != nil {
		t.Fatal(err)
	}
	if safe.Rules[0].URL != "" || !safe.Rules[0].CredentialsSet {
		t.Fatalf("leaked secret: %+v", safe)
	}
	if _, err := s.SaveMonitoringSettings(ctx, value); !errors.Is(err, ErrMonitoringConflict) {
		t.Fatalf("no optimistic lock: %v", err)
	}
	safe.BackupKeep = 3
	if _, err := s.SaveMonitoringSettings(ctx, safe); err != nil {
		t.Fatal(err)
	}
	stored, _ := s.MonitoringSettings(ctx)
	if stored.Rules[0].URL != value.Rules[0].URL {
		t.Fatal("lost retained secret")
	}
}

func TestBackupIntegrityRetentionAndUnsafeNames(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()
	if err := s.SetKV(ctx, "fixture", "before"); err != nil {
		t.Fatal(err)
	}
	first, err := s.CreateBackup(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if first.Size == 0 || len(first.SHA256) != 64 {
		t.Fatalf("invalid backup: %+v", first)
	}
	if err := s.VerifyBackup(ctx, first.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := s.backupPath("../cg.sqlite"); err == nil {
		t.Fatal("path traversal accepted")
	}
	second, err := s.CreateBackup(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.dataDir, "backups", first.Name)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old backup not removed: %v", err)
	}
	backups, _ := s.Backups(ctx)
	if len(backups) != 1 || backups[0].Name != second.Name {
		t.Fatalf("bad retention: %+v", backups)
	}
	path := filepath.Join(s.dataDir, "backups", second.Name)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	file.WriteString("tampering")
	file.Close()
	if err := s.VerifyBackup(ctx, second.Name); err == nil {
		t.Fatal("tampering accepted")
	}
}

func TestIncidentsDoNotResolveOnUnknownOrConfigurationChange(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()
	cfg := config.Config{Providers: []config.ProviderConfig{{ID: "p", ConnectionRevision: "one"}}}
	result := probe.Result{ProviderID: "p", Model: "m", Status: "error", Completed: true}
	if err := s.ObserveIncidents(ctx, cfg, []probe.Result{result}); err != nil {
		t.Fatal(err)
	}
	items, _ := s.Incidents(ctx)
	if len(items) != 1 {
		t.Fatal(items)
	}
	if err := s.AcknowledgeIncident(ctx, items[0].ID, "investigating"); err != nil {
		t.Fatal(err)
	}
	result.Status = "unknown"
	if err := s.ObserveIncidents(ctx, cfg, []probe.Result{result}); err != nil {
		t.Fatal(err)
	}
	items, _ = s.Incidents(ctx)
	if items[0].Status != "open" || items[0].AcknowledgedAt == "" {
		t.Fatal(items)
	}
	cfg.Providers[0].ConnectionRevision = "two"
	result.Status = "ok"
	if err := s.ObserveIncidents(ctx, cfg, []probe.Result{result}); err != nil {
		t.Fatal(err)
	}
	items, _ = s.Incidents(ctx)
	if items[0].Status != "superseded" {
		t.Fatal(items)
	}
	result.Status = "error"
	if err := s.ObserveIncidents(ctx, cfg, []probe.Result{result}); err != nil {
		t.Fatal(err)
	}
	result.Status = "ok"
	if err := s.ObserveIncidents(ctx, cfg, []probe.Result{result}); err != nil {
		t.Fatal(err)
	}
	items, _ = s.Incidents(ctx)
	if items[0].Status != "resolved" || items[0].ResolvedAt == "" {
		t.Fatal(items)
	}
}

func TestCostAccountingPreservesHistoricalPricesAndUnknowns(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()
	settings, _ := s.MonitoringSettings(ctx)
	settings.MonthlyBudget = 0.001
	settings.Prices = []config.ModelPrice{{ProviderID: "p", Model: "m", InputPerMillion: 2, OutputPerMillion: 4}}
	settings, err := s.SaveMonitoringSettings(ctx, settings)
	if err != nil {
		t.Fatal(err)
	}
	known := probe.Result{ProviderID: "p", Model: "m", Status: "ok", PromptTokens: 1000, CompletionTokens: 500, TotalTokens: 1500, UsageKnown: true}
	if err := s.RecordResults(ctx, []probe.Result{known, {ProviderID: "p", Model: "m", Status: "error"}}, time.Now(), 100, false); err != nil {
		t.Fatal(err)
	}
	settings.Prices[0].InputPerMillion = 20
	settings, err = s.SaveMonitoringSettings(ctx, settings)
	if err != nil {
		t.Fatal(err)
	}
	value, err := s.CostSummary(ctx, settings)
	if err != nil || value.EstimatedUSD != 0.004 || value.UnknownProbes != 1 || !value.BudgetExceeded {
		t.Fatalf("cost=%+v err=%v", value, err)
	}
}

func TestProviderSchedulesPersistAndResetOnRevision(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	due, err := s.ScheduleDue(ctx, "p", "one", 5, now)
	if err != nil || due {
		t.Fatalf("new schedule fired: %v %v", due, err)
	}
	due, err = s.ScheduleDue(ctx, "p", "one", 5, now.Add(5*time.Minute))
	if err != nil || !due {
		t.Fatalf("schedule not due: %v %v", due, err)
	}
	due, err = s.ScheduleDue(ctx, "p", "two", 5, now.Add(5*time.Minute))
	if err != nil || due {
		t.Fatalf("old revision survived: %v %v", due, err)
	}
	values, _ := s.ScheduleStatus(ctx)
	if len(values) != 1 || !strings.Contains(values[0].NextAt, "T") {
		t.Fatal(values)
	}
}
