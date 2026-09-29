package main

import (
	"strings"
	"testing"

	"clipbox/internal/clipboard"
	"clipbox/internal/storage"
	"clipbox/internal/windowutil"
)

func TestDefaultAppSettingsEnableAutoPaste(t *testing.T) {
	settings := defaultAppSettings()
	if !settings.AutoPaste {
		t.Fatal("default AutoPaste = false, want true")
	}
	if !settings.RecordSourceInfo {
		t.Fatal("default RecordSourceInfo = false, want true")
	}
}

func TestNormalizeAppSettingsClampsNumericFields(t *testing.T) {
	longRule := strings.Repeat("a", maxExclusionRuleLen+20)
	settings := normalizeAppSettings(AppSettings{
		AutoPaste:              false,
		MinTextLength:          -10,
		MaxClips:               5,
		RetentionDays:          -1,
		MaxImageStorageMB:      1,
		AutoBackupIntervalDays: 0,
		AutoBackupMaxFiles:     0,
		AutoBackupDir:          "  C:\\backups  ",
		ExcludedApps:           []string{"  1Password.exe  ", "1password.exe", "", longRule},
		ExcludedWindowTitles:   []string{"密码", " 密码 "},
	})

	if settings.AutoPaste {
		t.Fatal("normalizeAppSettings changed AutoPaste to true")
	}
	if settings.MinTextLength != 1 {
		t.Fatalf("MinTextLength = %d, want 1", settings.MinTextLength)
	}
	if settings.MaxClips != 50 {
		t.Fatalf("MaxClips = %d, want 50", settings.MaxClips)
	}
	if settings.RetentionDays != 0 {
		t.Fatalf("RetentionDays = %d, want 0", settings.RetentionDays)
	}
	if settings.MaxImageStorageMB != 32 {
		t.Fatalf("MaxImageStorageMB = %d, want 32", settings.MaxImageStorageMB)
	}
	if settings.AutoBackupIntervalDays != 1 {
		t.Fatalf("AutoBackupIntervalDays = %d, want 1", settings.AutoBackupIntervalDays)
	}
	if settings.AutoBackupMaxFiles != 1 {
		t.Fatalf("AutoBackupMaxFiles = %d, want 1", settings.AutoBackupMaxFiles)
	}
	if settings.AutoBackupDir != `C:\backups` {
		t.Fatalf("AutoBackupDir = %q, want trimmed path", settings.AutoBackupDir)
	}
	if got, want := settings.ExcludedApps, []string{"1Password.exe", strings.Repeat("a", maxExclusionRuleLen)}; !sameStringSlice(got, want) {
		t.Fatalf("ExcludedApps = %#v, want %#v", got, want)
	}
	if got, want := settings.ExcludedWindowTitles, []string{"密码"}; !sameStringSlice(got, want) {
		t.Fatalf("ExcludedWindowTitles = %#v, want %#v", got, want)
	}
}

func TestLooksSensitiveText(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		{name: "password assignment", text: "password = secret-value", want: true},
		{name: "token assignment", text: "api_key: abc123", want: true},
		{name: "long mixed token", text: "abcDEF1234567890!@#$%^&*()", want: true},
		{name: "ordinary sentence", text: "今天下午三点同步一下剪贴板功能", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := looksSensitiveText(tc.text); got != tc.want {
				t.Fatalf("looksSensitiveText(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

func TestShouldSkipForegroundMatchesProcessAndTitle(t *testing.T) {
	settings := normalizeAppSettings(AppSettings{
		ExcludedApps:         []string{"keepass"},
		ExcludedWindowTitles: []string{"secret vault"},
	})

	cases := []struct {
		name string
		info windowutil.ForegroundInfo
		want bool
	}{
		{
			name: "process name",
			info: windowutil.ForegroundInfo{ProcessName: "KeePassXC.exe", ProcessPath: `C:\Tools\KeePassXC.exe`, Title: "Notes"},
			want: true,
		},
		{
			name: "window title",
			info: windowutil.ForegroundInfo{ProcessName: "notepad.exe", Title: "Secret Vault - Notepad"},
			want: true,
		},
		{
			name: "no match",
			info: windowutil.ForegroundInfo{ProcessName: "notepad.exe", Title: "Shopping list"},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldSkipForeground(settings, tc.info); got != tc.want {
				t.Fatalf("shouldSkipForeground() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestShouldCaptureUsesForegroundExclusion(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	app := NewApp(dir, false)
	app.store = store
	app.foregroundInfo = func() windowutil.ForegroundInfo {
		return windowutil.ForegroundInfo{
			ProcessName: "1Password.exe",
			ProcessPath: `C:\Program Files\1Password\1Password.exe`,
			Title:       "1Password",
		}
	}

	if err := store.SetSetting("excluded_apps", "1password.exe"); err != nil {
		t.Fatalf("SetSetting() error = %v", err)
	}

	if app.shouldCapture(clipboard.ClipEntry{Type: "text", Content: "ordinary text"}) {
		t.Fatal("shouldCapture() = true, want false for excluded foreground app")
	}
}

func TestPrepareClipEntryAddsSourceInfo(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	app := NewApp(dir, false)
	app.store = store
	app.foregroundInfo = func() windowutil.ForegroundInfo {
		return windowutil.ForegroundInfo{
			ProcessName: "Code.exe",
			ProcessPath: `C:\Users\QGS\AppData\Local\Programs\Microsoft VS Code\Code.exe`,
			Title:       "settings.go - ClipBox",
		}
	}

	entry, ok := app.prepareClipEntry(clipboard.ClipEntry{Type: "text", Content: "ordinary text", Preview: "ordinary text"})
	if !ok {
		t.Fatal("prepareClipEntry() ok = false, want true")
	}
	if entry.SourceApp != "Code.exe" || entry.SourceTitle != "settings.go - ClipBox" || entry.SourcePath == "" {
		t.Fatalf("prepareClipEntry() source = %+v", entry)
	}
}

func TestPrepareClipEntrySkipsSourceInfoWhenDisabled(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()
	if err := store.SetSetting("record_source_info", "false"); err != nil {
		t.Fatalf("SetSetting() error = %v", err)
	}

	app := NewApp(dir, false)
	app.store = store
	app.foregroundInfo = func() windowutil.ForegroundInfo {
		return windowutil.ForegroundInfo{
			ProcessName: "notepad.exe",
			ProcessPath: `C:\Windows\System32\notepad.exe`,
			Title:       "Notes",
		}
	}

	entry, ok := app.prepareClipEntry(clipboard.ClipEntry{Type: "text", Content: "ordinary text", Preview: "ordinary text"})
	if !ok {
		t.Fatal("prepareClipEntry() ok = false, want true")
	}
	if entry.SourceApp != "" || entry.SourceTitle != "" || entry.SourcePath != "" {
		t.Fatalf("prepareClipEntry() source = %+v, want empty", entry)
	}
}

func TestUpdateAppSettingsFailureKeepsPersistedState(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	app := NewApp(dir, false)
	app.store = store

	original := app.loadAppSettings()
	next := original
	next.Theme = "light"
	next.MinTextLength = 5

	// 关闭数据库后保存必然失败：不能广播未持久化的值，也不更新内存缓存。
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	returned, err := app.UpdateAppSettings(next)
	if err == nil {
		t.Fatal("UpdateAppSettings() error = nil, want persistence failure")
	}
	if returned.Theme == next.Theme || returned.MinTextLength == next.MinTextLength {
		t.Fatalf("UpdateAppSettings() returned %+v, want persisted values instead of unsaved input", returned)
	}
	if cached := app.loadAppSettings(); cached.Theme == next.Theme || cached.MinTextLength == next.MinTextLength {
		t.Fatalf("settings cache = %+v, want persisted values after failure", cached)
	}
}

func TestSetCapturePausedPersistsSetting(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	app := NewApp(dir, false)
	app.store = store

	if err := app.setCapturePaused(true); err != nil {
		t.Fatalf("setCapturePaused(true) error = %v", err)
	}
	if !app.loadAppSettings().CapturePaused {
		t.Fatal("CapturePaused = false, want true")
	}

	if err := app.setCapturePaused(false); err != nil {
		t.Fatalf("setCapturePaused(false) error = %v", err)
	}
	if app.loadAppSettings().CapturePaused {
		t.Fatal("CapturePaused = true, want false")
	}
}

func TestGetLastTargetInfo(t *testing.T) {
	app := NewApp(t.TempDir(), false)
	if info := app.GetLastTargetInfo(); info.Available {
		t.Fatal("GetLastTargetInfo().Available = true, want false without a target")
	}

	app.windowInfo = func(hwnd uintptr) windowutil.ForegroundInfo {
		if hwnd != 12345 {
			t.Fatalf("windowInfo hwnd = %d, want 12345", hwnd)
		}
		return windowutil.ForegroundInfo{
			ProcessName: "KeePassXC.exe",
			ProcessPath: `C:\Tools\KeePassXC.exe`,
			Title:       "Database - KeePassXC",
		}
	}
	app.mu.Lock()
	app.lastTarget = 12345
	app.mu.Unlock()

	info := app.GetLastTargetInfo()
	if !info.Available {
		t.Fatal("GetLastTargetInfo().Available = false, want true")
	}
	if info.ProcessName != "KeePassXC.exe" || info.ProcessPath != `C:\Tools\KeePassXC.exe` || info.Title != "Database - KeePassXC" {
		t.Fatalf("GetLastTargetInfo() = %+v", info)
	}
}

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
