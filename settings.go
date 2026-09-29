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
	Theme                  string   `json:"theme"`
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
		Theme:                  "dark",
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
	switch settings.Theme {
	case "dark", "light", "system":
	default:
		settings.Theme = "dark"
	}
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
	prev := a.loadAppSettings()
	settings = normalizeAppSettings(settings)
	if err := a.saveAppSettings(settings); err != nil {
		// 保存失败时不广播未持久化的状态，返回 DB 真值让前端与后端保持一致。
		a.invalidateSettingsCache()
		return a.loadAppSettings(), err
	}

	// 先广播设置变更，让主题等 UI 状态立即生效，再做可能耗时的清理。
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "settings:updated", settings)
	}
	a.updateTrayState()

	// 只有保留策略变化时才需要清理（含图片目录扫描，开销大），
	// 拨动普通开关不应触发清理和列表重载。
	policyChanged := prev.MaxClips != settings.MaxClips ||
		prev.RetentionDays != settings.RetentionDays ||
		prev.MaxImageStorageMB != settings.MaxImageStorageMB
	if policyChanged {
		if err := a.applyCleanup(settings); err != nil {
			return settings, err
		}
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "clips:changed")
		}
	}
	return settings, nil
}

func (a *App) loadAppSettings() AppSettings {
	settings := defaultAppSettings()
	if a.store == nil {
		return settings
	}
	a.settingsMu.Lock()
	if a.settingsCache != nil {
		cached := *a.settingsCache
		a.settingsMu.Unlock()
		return cached
	}
	a.settingsMu.Unlock()

	settings.CapturePaused = a.getBoolSetting("capture_paused", settings.CapturePaused)
	settings.RecordImages = a.getBoolSetting("record_images", settings.RecordImages)
	settings.SkipSensitiveText = a.getBoolSetting("skip_sensitive_text", settings.SkipSensitiveText)
	settings.RecordSourceInfo = a.getBoolSetting("record_source_info", settings.RecordSourceInfo)
	settings.StartAtLogin = a.loadStartAtLoginIntent()
	settings.AutoPaste = a.getBoolSetting("auto_paste", settings.AutoPaste)
	settings.Theme = a.getStringSetting("theme")
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
	settings = normalizeAppSettings(settings)

	a.settingsMu.Lock()
	cached := settings
	a.settingsCache = &cached
	a.settingsMu.Unlock()
	return settings
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
		"start_at_login":            strconv.FormatBool(settings.StartAtLogin),
		"auto_paste":                strconv.FormatBool(settings.AutoPaste),
		"theme":                     settings.Theme,
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

	// 开机自启：先对齐注册表，再落盘 DB 意图。注册表失败时其它设置不再写入，
	// 缓存也不更新，避免 DB 显示开启而注册表实际没有写入。
	prevIntent := a.loadStartAtLoginIntent()
	if err := reconcileStartAtLogin(settings.StartAtLogin); err != nil {
		a.invalidateSettingsCache()
		return fmt.Errorf("开机自启更新失败: %w", err)
	}
	if err := a.store.SetSettings(values); err != nil {
		// DB 写入失败时把注册表回滚到此前意图，保持两者一致。
		if rollbackErr := reconcileStartAtLogin(prevIntent); rollbackErr != nil {
			a.logErrorf("failed to roll back start-at-login after settings save failure: %v", rollbackErr)
			a.invalidateSettingsCache()
			return fmt.Errorf("设置保存失败，且开机自启回滚失败，请重开设置页确认自启开关状态: %w", err)
		}
		a.invalidateSettingsCache()
		return err
	}
	a.settingsMu.Lock()
	cached := settings
	a.settingsCache = &cached
	a.settingsMu.Unlock()
	return nil
}

// loadStartAtLoginIntent 以 DB 记录的意图为准；DB 未记录过时（老用户或全新安装）
// 回退到读注册表，作为一次性迁移，之后写回 DB。
func (a *App) loadStartAtLoginIntent() bool {
	raw, err := a.store.GetSetting("start_at_login")
	if err == nil && raw != "" {
		if parsed, perr := strconv.ParseBool(raw); perr == nil {
			return parsed
		}
	}
	inferred := registryHasStartupEntry()
	_ = a.store.SetSetting("start_at_login", strconv.FormatBool(inferred))
	return inferred
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
