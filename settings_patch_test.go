package main

import (
	"testing"
)

func TestSettingsPatchPreservesConcurrentTrayPause(t *testing.T) {
	a := newTestAppWithStore(t)
	if err := a.setCapturePaused(true); err != nil {
		t.Fatal(err)
	}
	settings, err := a.PatchAppSettings(map[string]any{"minTextLength": 2})
	if err != nil || !settings.CapturePaused || settings.MinTextLength != 2 {
		t.Fatalf("patch=%+v,%v", settings, err)
	}
	if _, err := a.PatchAppSettings(map[string]any{"notASetting": true}); err == nil {
		t.Fatal("unknown field accepted")
	}
	if _, err := a.PatchAppSettings(map[string]any{"maxClips": "invalid"}); err == nil {
		t.Fatal("invalid field type accepted")
	}
}

func TestSettingsCacheDoesNotShareMutableRules(t *testing.T) {
	a := newTestAppWithStore(t)
	settings := defaultAppSettings()
	settings.ExcludedApps = []string{"keep.exe"}
	if err := a.saveAppSettings(settings); err != nil {
		t.Fatal(err)
	}
	settings.ExcludedApps[0] = "mutated.exe"
	first := a.GetAppSettings()
	first.ExcludedApps[0] = "other.exe"
	if a.GetAppSettings().ExcludedApps[0] != "keep.exe" {
		t.Fatal("caller mutated the settings cache")
	}
}
