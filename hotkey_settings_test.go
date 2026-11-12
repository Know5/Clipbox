package main

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	"clipbox/internal/hotkey"
)

func TestHotkeyPersistenceFailureIsReported(t *testing.T) {
	app := newTestAppWithStore(t)
	mods := uint32(hotkey.ModControl | hotkey.ModAlt | hotkey.ModShift)
	manager := hotkey.NewManager()
	if err := manager.Start(mods, 0x86); err != nil {
		t.Skipf("native F23 test shortcut unavailable: %v", err)
	}
	defer manager.Stop()
	app.hotkeyManager = manager
	if err := app.store.SetSettings(map[string]string{"hotkey_modifiers": "7", "hotkey_key": "134"}); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(app.dataDir, "clipbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER fail_hotkey_settings BEFORE INSERT ON settings WHEN new.key IN ('hotkey_modifiers', 'hotkey_key') BEGIN SELECT RAISE(ABORT, 'injected settings failure'); END`); err != nil {
		t.Fatal(err)
	}
	err = app.UpdateHotkey(int(mods), 0x87)
	if err == nil {
		stored := app.GetHotkeySettings()
		runtimeVK := reflect.ValueOf(manager).Elem().FieldByName("currentVK").Uint()
		t.Fatalf("save reported success despite database failure: runtime key=%#x, persisted key=%#x", runtimeVK, stored.KeyCode)
	}
}

func TestLegacyBackupHotkeyIsValidatedOnImport(t *testing.T) {
	app := newTestAppWithStore(t)
	if err := app.store.SetSettings(map[string]string{"hotkey_modifiers": "2", "hotkey_key": "49", "start_at_login": "false"}); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "legacy.clipbox-backup")
	if _, err := app.store.ExportBackupFile(backup, "0.2.1"); err != nil {
		t.Fatal(err)
	}
	if err := app.store.SetSettings(map[string]string{"hotkey_modifiers": "3", "hotkey_key": "86"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.store.ImportBackupFile(backup); err != nil {
		t.Fatal(err)
	}
	app.applyImportedRuntimeSettings()
	settings := app.GetHotkeySettings()
	if reason := reservedHotkeyReason(settings.Modifiers, settings.KeyCode); reason != "" {
		t.Fatalf("import retained a reserved shortcut: %s (%s)", settings.Display, reason)
	}
}

func TestStartupHotkeyConflictCanBeRepairedInSettings(t *testing.T) {
	app := newTestAppWithStore(t)
	mods := uint32(hotkey.ModControl | hotkey.ModAlt | hotkey.ModShift)
	blocker := hotkey.NewManager()
	if err := blocker.Start(mods, 0x86); err != nil {
		t.Skipf("native F23 test shortcut unavailable: %v", err)
	}
	defer blocker.Stop()
	manager := hotkey.NewManager()
	if err := manager.Start(mods, 0x86); err == nil {
		manager.Stop()
		t.Fatal("failed to create the startup conflict")
	}
	defer manager.Stop()
	app.hotkeyManager = manager
	if err := app.UpdateHotkey(int(mods), 0x87); err != nil {
		t.Fatalf("settings cannot recover from a startup registration conflict using a free shortcut: %v", err)
	}
}
