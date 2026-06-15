package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"clipbox/internal/clipboard"
	"clipbox/internal/storage"
	"clipbox/internal/windowutil"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type AppSettings struct {
	CapturePaused          bool     `json:"capturePaused"`
	RecordImages           bool     `json:"recordImages"`
	SkipSensitiveText      bool     `json:"skipSensitiveText"`
	RecordSourceInfo       bool     `json:"recordSourceInfo"`
	StartAtLogin           bool     `json:"startAtLogin"`
	AutoPaste              bool     `json:"autoPaste"`
	MinTextLength          int      `json:"minTextLength"`
	MaxClips               int      `json:"maxClips"`
	RetentionDays          int      `json:"retentionDays"`
	MaxImageStorageMB      int      `json:"maxImageStorageMB"`
	UpdateManifestURL      string   `json:"updateManifestURL"`
	AutoBackupEnabled      bool     `json:"autoBackupEnabled"`
	AutoBackupDir          string   `json:"autoBackupDir"`
	AutoBackupIntervalDays int      `json:"autoBackupIntervalDays"`
	AutoBackupMaxFiles     int      `json:"autoBackupMaxFiles"`
	ExcludedApps           []string `json:"excludedApps"`
	ExcludedWindowTitles   []string `json:"excludedWindowTitles"`
}

var sensitiveTextPattern = regexp.MustCompile(`(?i)\b(password|passwd|pwd|token|secret|api[_-]?key|authorization|bearer|access[_-]?key)\b\s*[:=]`)

const (
	maxExclusionRules   = 100
	maxExclusionRuleLen = 200
	maxUpdateURLLen     = 500
	maxBackupDirLen     = 1000
	maxSourceAppLen     = 160
	maxSourceTitleLen   = 300
	maxSourcePathLen    = 500
)

func defaultAppSettings() AppSettings {
	return AppSettings{
		CapturePaused:          false,
		RecordImages:           true,
		SkipSensitiveText:      false,
		RecordSourceInfo:       true,
		StartAtLogin:           false,
		AutoPaste:              true,
		MinTextLength:          1,
		MaxClips:               500,
		RetentionDays:          0,
		MaxImageStorageMB:      1024,
		AutoBackupEnabled:      false,
		AutoBackupIntervalDays: 1,
		AutoBackupMaxFiles:     10,
	}
}

func normalizeAppSettings(settings AppSettings) AppSettings {
	if settings.MinTextLength < 1 {
		settings.MinTextLength = 1
	}
	if settings.MinTextLength > 5000 {
		settings.MinTextLength = 5000
	}
	if settings.MaxClips < 50 {
		settings.MaxClips = 50
	}
	if settings.MaxClips > 10000 {
		settings.MaxClips = 10000
	}
	if settings.RetentionDays < 0 {
		settings.RetentionDays = 0
	}
	if settings.RetentionDays > 3650 {
		settings.RetentionDays = 3650
	}
	if settings.MaxImageStorageMB < 32 {
		settings.MaxImageStorageMB = 32
	}
	if settings.MaxImageStorageMB > 102400 {
		settings.MaxImageStorageMB = 102400
	}
	if settings.AutoBackupIntervalDays < 1 {
		settings.AutoBackupIntervalDays = 1
	}
	if settings.AutoBackupIntervalDays > 365 {
		settings.AutoBackupIntervalDays = 365
	}
	if settings.AutoBackupMaxFiles < 1 {
		settings.AutoBackupMaxFiles = 1
	}
	if settings.AutoBackupMaxFiles > 100 {
		settings.AutoBackupMaxFiles = 100
	}
	settings.ExcludedApps = normalizeRuleList(settings.ExcludedApps)
	settings.ExcludedWindowTitles = normalizeRuleList(settings.ExcludedWindowTitles)
	settings.UpdateManifestURL = truncateSettingRunes(strings.TrimSpace(settings.UpdateManifestURL), maxUpdateURLLen)
	settings.AutoBackupDir = truncateSettingRunes(strings.TrimSpace(settings.AutoBackupDir), maxBackupDirLen)
	return settings
}

func (a *App) GetAppSettings() AppSettings {
	return a.loadAppSettings()
}

func (a *App) UpdateAppSettings(settings AppSettings) (AppSettings, error) {
	settings = normalizeAppSettings(settings)
	saveErr := a.saveAppSettings(settings)
	cleanupErr := a.applyCleanup(settings)
	if cleanupErr != nil {
		return settings, cleanupErr
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "settings:updated", settings)
		runtime.EventsEmit(a.ctx, "clips:changed")
	}
	a.updateTrayState()
	if saveErr != nil {
		return settings, saveErr
	}
	return settings, nil
}

func (a *App) loadAppSettings() AppSettings {
	settings := defaultAppSettings()
	if a.store == nil {
		return settings
	}
	settings.CapturePaused = a.getBoolSetting("capture_paused", settings.CapturePaused)
	settings.RecordImages = a.getBoolSetting("record_images", settings.RecordImages)
	settings.SkipSensitiveText = a.getBoolSetting("skip_sensitive_text", settings.SkipSensitiveText)
	settings.RecordSourceInfo = a.getBoolSetting("record_source_info", settings.RecordSourceInfo)
	settings.StartAtLogin = getStartAtLogin()
	settings.AutoPaste = a.getBoolSetting("auto_paste", settings.AutoPaste)
	settings.MinTextLength = a.getIntSetting("min_text_length", settings.MinTextLength)
	settings.MaxClips = a.getIntSetting("max_clips", settings.MaxClips)
	settings.RetentionDays = a.getIntSetting("retention_days", settings.RetentionDays)
	settings.MaxImageStorageMB = a.getIntSetting("max_image_storage_mb", settings.MaxImageStorageMB)
	settings.UpdateManifestURL = a.getStringSetting("update_manifest_url")
	settings.AutoBackupEnabled = a.getBoolSetting("auto_backup_enabled", settings.AutoBackupEnabled)
	settings.AutoBackupDir = a.getStringSetting("auto_backup_dir")
	settings.AutoBackupIntervalDays = a.getIntSetting("auto_backup_interval_days", settings.AutoBackupIntervalDays)
	settings.AutoBackupMaxFiles = a.getIntSetting("auto_backup_max_files", settings.AutoBackupMaxFiles)
	settings.ExcludedApps = splitSettingList(a.getStringSetting("excluded_apps"))
	settings.ExcludedWindowTitles = splitSettingList(a.getStringSetting("excluded_window_titles"))
	return normalizeAppSettings(settings)
}

func (a *App) saveAppSettings(settings AppSettings) error {
	if a.store == nil {
		return nil
	}
	values := map[string]string{
		"capture_paused":            strconv.FormatBool(settings.CapturePaused),
		"record_images":             strconv.FormatBool(settings.RecordImages),
		"skip_sensitive_text":       strconv.FormatBool(settings.SkipSensitiveText),
		"record_source_info":        strconv.FormatBool(settings.RecordSourceInfo),
		"auto_paste":                strconv.FormatBool(settings.AutoPaste),
		"min_text_length":           strconv.Itoa(settings.MinTextLength),
		"max_clips":                 strconv.Itoa(settings.MaxClips),
		"retention_days":            strconv.Itoa(settings.RetentionDays),
		"max_image_storage_mb":      strconv.Itoa(settings.MaxImageStorageMB),
		"update_manifest_url":       settings.UpdateManifestURL,
		"auto_backup_enabled":       strconv.FormatBool(settings.AutoBackupEnabled),
		"auto_backup_dir":           settings.AutoBackupDir,
		"auto_backup_interval_days": strconv.Itoa(settings.AutoBackupIntervalDays),
		"auto_backup_max_files":     strconv.Itoa(settings.AutoBackupMaxFiles),
		"excluded_apps":             strings.Join(settings.ExcludedApps, "\n"),
		"excluded_window_titles":    strings.Join(settings.ExcludedWindowTitles, "\n"),
	}
	for key, value := range values {
		if err := a.store.SetSetting(key, value); err != nil {
			return err
		}
	}
	if err := setStartAtLogin(settings.StartAtLogin); err != nil {
		return fmt.Errorf("其他设置已保存，但开机自启更新失败: %w", err)
	}
	return nil
}

func (a *App) getBoolSetting(key string, fallback bool) bool {
	value, err := a.store.GetSetting(key)
	if err != nil || value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func (a *App) getIntSetting(key string, fallback int) int {
	value, err := a.store.GetSetting(key)
	if err != nil || value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func (a *App) getStringSetting(key string) string {
	value, err := a.store.GetSetting(key)
	if err != nil {
		return ""
	}
	return value
}

func (a *App) shouldCapture(entry clipboard.ClipEntry) bool {
	_, ok := a.prepareClipEntry(entry)
	return ok
}

func (a *App) prepareClipEntry(entry clipboard.ClipEntry) (clipboard.ClipEntry, bool) {
	settings := a.loadAppSettings()
	if settings.CapturePaused {
		return entry, false
	}

	info := a.currentForegroundInfo()
	if shouldSkipForeground(settings, info) {
		return entry, false
	}

	switch entry.Type {
	case "text":
		text := strings.TrimSpace(entry.Content)
		if len([]rune(text)) < settings.MinTextLength {
			return entry, false
		}
		if settings.SkipSensitiveText && looksSensitiveText(text) {
			return entry, false
		}
	case "image":
		if !settings.RecordImages {
			return entry, false
		}
	default:
		return entry, false
	}

	if settings.RecordSourceInfo {
		entry = enrichSourceInfo(entry, info)
	}
	return entry, true
}

func enrichSourceInfo(entry clipboard.ClipEntry, info windowutil.ForegroundInfo) clipboard.ClipEntry {
	sourceApp := info.ProcessName
	if sourceApp == "" && info.ProcessPath != "" {
		sourceApp = filepath.Base(info.ProcessPath)
	}
	entry.SourceApp = truncateSettingRunes(sourceApp, maxSourceAppLen)
	entry.SourceTitle = truncateSettingRunes(info.Title, maxSourceTitleLen)
	entry.SourcePath = truncateSettingRunes(info.ProcessPath, maxSourcePathLen)
	return entry
}

func (a *App) currentForegroundInfo() windowutil.ForegroundInfo {
	if a.foregroundInfo != nil {
		return a.foregroundInfo()
	}
	return windowutil.CurrentForegroundInfo()
}

func shouldSkipForeground(settings AppSettings, info windowutil.ForegroundInfo) bool {
	if matchesAnyRule(settings.ExcludedApps, info.ProcessName, info.ProcessPath) {
		return true
	}
	return matchesAnyRule(settings.ExcludedWindowTitles, info.Title)
}

func normalizeRuleList(rules []string) []string {
	normalized := make([]string, 0, len(rules))
	seen := make(map[string]struct{})
	for _, rule := range rules {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		rule = truncateSettingRunes(rule, maxExclusionRuleLen)
		key := strings.ToLower(rule)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, rule)
		if len(normalized) >= maxExclusionRules {
			break
		}
	}
	return normalized
}

func splitSettingList(value string) []string {
	if value == "" {
		return nil
	}
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return normalizeRuleList(strings.Split(value, "\n"))
}

func matchesAnyRule(rules []string, values ...string) bool {
	for _, rule := range rules {
		needle := strings.ToLower(strings.TrimSpace(rule))
		if needle == "" {
			continue
		}
		for _, value := range values {
			haystack := strings.ToLower(value)
			if haystack != "" && strings.Contains(haystack, needle) {
				return true
			}
		}
	}
	return false
}

func truncateSettingRunes(text string, max int) string {
	if max <= 0 {
		return ""
	}
	count := 0
	for i := range text {
		if count == max {
			return text[:i]
		}
		count++
	}
	return text
}

func (a *App) applyCurrentCleanup() error {
	return a.applyCleanup(a.loadAppSettings())
}

func (a *App) applyCleanup(settings AppSettings) error {
	if a.store == nil {
		return nil
	}
	return a.store.Cleanup(storage.CleanupPolicy{
		MaxClips:   settings.MaxClips,
		MaxDays:    settings.RetentionDays,
		MaxImageMB: settings.MaxImageStorageMB,
	})
}

func looksSensitiveText(text string) bool {
	if sensitiveTextPattern.MatchString(text) {
		return true
	}

	trimmed := strings.TrimSpace(text)
	if len([]rune(trimmed)) < 24 || strings.ContainsAny(trimmed, " \t\r\n") {
		return false
	}

	hasLetter := false
	hasDigit := false
	hasSymbol := false
	for _, r := range trimmed {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		default:
			hasSymbol = true
		}
	}
	return hasLetter && hasDigit && hasSymbol
}
