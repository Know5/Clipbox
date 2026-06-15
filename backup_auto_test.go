package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"clipbox/internal/clipboard"
)

func TestAutoBackupDueUsesLastBackupTimestamp(t *testing.T) {
	app := newTestAppWithStore(t)
	settings := normalizeAppSettings(AppSettings{
		AutoBackupEnabled:      true,
		AutoBackupDir:          t.TempDir(),
		AutoBackupIntervalDays: 2,
		AutoBackupMaxFiles:     5,
	})
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)

	if !app.autoBackupDue(settings, now) {
		t.Fatal("autoBackupDue without previous timestamp = false, want true")
	}
	if err := app.store.SetSetting(autoBackupSettingLastAt, timeToMillis(now.Add(-24*time.Hour))); err != nil {
		t.Fatalf("SetSetting last backup: %v", err)
	}
	if app.autoBackupDue(settings, now) {
		t.Fatal("autoBackupDue after one day = true, want false")
	}
	if err := app.store.SetSetting(autoBackupSettingLastAt, timeToMillis(now.Add(-49*time.Hour))); err != nil {
		t.Fatalf("SetSetting old last backup: %v", err)
	}
	if !app.autoBackupDue(settings, now) {
		t.Fatal("autoBackupDue after interval = false, want true")
	}
}

func TestRunAutoBackupCreatesBackupAndPrunesOldFiles(t *testing.T) {
	app := newTestAppWithStore(t)
	if _, err := app.store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "text",
		Content:   "auto backup text",
		Preview:   "auto backup text",
		Timestamp: 1000,
	}); err != nil {
		t.Fatalf("SaveOrUpdate: %v", err)
	}

	dir := t.TempDir()
	oldA := filepath.Join(dir, "clipbox-auto-20200101-000000.clipbox-backup")
	oldB := filepath.Join(dir, "clipbox-auto-20200102-000000.clipbox-backup")
	manual := filepath.Join(dir, "manual.clipbox-backup")
	for _, path := range []string{oldA, oldB, manual} {
		if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
			t.Fatalf("WriteFile %s: %v", path, err)
		}
	}
	if err := os.Chtimes(oldA, time.Now().Add(-3*time.Hour), time.Now().Add(-3*time.Hour)); err != nil {
		t.Fatalf("Chtimes oldA: %v", err)
	}
	if err := os.Chtimes(oldB, time.Now().Add(-2*time.Hour), time.Now().Add(-2*time.Hour)); err != nil {
		t.Fatalf("Chtimes oldB: %v", err)
	}

	settings := normalizeAppSettings(AppSettings{
		AutoBackupEnabled:      true,
		AutoBackupDir:          dir,
		AutoBackupIntervalDays: 1,
		AutoBackupMaxFiles:     2,
	})
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	stats, err := app.runAutoBackup(settings, now)
	if err != nil {
		t.Fatalf("runAutoBackup: %v", err)
	}
	if stats.Cancelled || stats.Path == "" || stats.Clips != 1 {
		t.Fatalf("backup stats = %+v, want one exported clip", stats)
	}
	if _, err := os.Stat(stats.Path); err != nil {
		t.Fatalf("backup file missing: %v", err)
	}
	if _, err := os.Stat(oldA); !os.IsNotExist(err) {
		t.Fatalf("oldest auto backup still exists, err=%v", err)
	}
	if _, err := os.Stat(manual); err != nil {
		t.Fatalf("manual backup should be preserved: %v", err)
	}
	lastRaw, err := app.store.GetSetting(autoBackupSettingLastAt)
	if err != nil {
		t.Fatalf("GetSetting last backup: %v", err)
	}
	if lastRaw != timeToMillis(now) {
		t.Fatalf("last backup setting = %q, want %q", lastRaw, timeToMillis(now))
	}
}

func TestNextAutoBackupPathAvoidsExistingFile(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	first := filepath.Join(dir, "clipbox-auto-20260609-120000.clipbox-backup")
	if err := os.WriteFile(first, []byte("existing"), 0644); err != nil {
		t.Fatalf("WriteFile existing: %v", err)
	}
	got := nextAutoBackupPath(dir, now)
	want := filepath.Join(dir, "clipbox-auto-20260609-120000-1.clipbox-backup")
	if got != want {
		t.Fatalf("nextAutoBackupPath = %q, want %q", got, want)
	}
}

func timeToMillis(t time.Time) string {
	return strconv.FormatInt(t.UnixMilli(), 10)
}
