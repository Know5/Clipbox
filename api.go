package main

import (
	"clipbox/internal/clipboard"
	"clipbox/internal/hotkey"
	"clipbox/internal/storage"
	"clipbox/internal/windowutil"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type HotkeyConfig struct {
	Modifiers int    `json:"modifiers"`
	KeyCode   int    `json:"keyCode"`
	Display   string `json:"display"`
}

type AppInfo struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	DataDir      string `json:"dataDir"`
	DatabasePath string `json:"databasePath"`
	ImageDir     string `json:"imageDir"`
	ExePath      string `json:"exePath"`
	StartHidden  bool   `json:"startHidden"`
}

type TargetWindowInfo struct {
	Available   bool   `json:"available"`
	ProcessName string `json:"processName"`
	ProcessPath string `json:"processPath"`
	Title       string `json:"title"`
}

type ClipPage struct {
	Items   []clipboard.ClipEntry `json:"items"`
	Total   int                   `json:"total"`
	Limit   int                   `json:"limit"`
	Offset  int                   `json:"offset"`
	HasMore bool                  `json:"hasMore"`
}

type ClipExportResult struct {
	Path      string `json:"path"`
	Type      string `json:"type"`
	Bytes     int64  `json:"bytes"`
	Cancelled bool   `json:"cancelled"`
}

type ClipTag struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func hotkeyDisplayName(modifiers, keyCode int) string {
	result := ""
	if modifiers&hotkey.ModControl != 0 {
		result += "Ctrl + "
	}
	if modifiers&hotkey.ModAlt != 0 {
		result += "Alt + "
	}
	if modifiers&hotkey.ModShift != 0 {
		result += "Shift + "
	}
	if modifiers&hotkey.ModWin != 0 {
		result += "Win + "
	}
	keyName := ""
	switch {
	case keyCode >= 0x41 && keyCode <= 0x5A:
		keyName = string(rune('A' + keyCode - 0x41))
	case keyCode >= 0x30 && keyCode <= 0x39:
		keyName = string(rune('0' + keyCode - 0x30))
	case keyCode >= 0x70 && keyCode <= 0x7B:
		keyName = fmt.Sprintf("F%d", keyCode-0x70+1)
	case keyCode == 0x20:
		keyName = "Space"
	case keyCode == 0x0D:
		keyName = "Enter"
	case keyCode == 0xBE:
		keyName = "."
	case keyCode == 0xBC:
		keyName = ","
	case keyCode == 0xBF:
		keyName = "/"
	case keyCode == 0xBA:
		keyName = ";"
	case keyCode == 0xDE:
		keyName = "'"
	case keyCode == 0xDB:
		keyName = "["
	case keyCode == 0xDD:
		keyName = "]"
	case keyCode == 0xDC:
		keyName = "\\"
	case keyCode == 0xC0:
		keyName = "`"
	case keyCode == 0xBD:
		keyName = "-"
	case keyCode == 0xBB:
		keyName = "="
	default:
		keyName = fmt.Sprintf("0x%02X", keyCode)
	}
	return result + keyName
}

func (a *App) GetClips(limit, offset int) ([]clipboard.ClipEntry, error) {
	return a.store.GetAll(limit, offset)
}

func (a *App) GetClipPage(limit, offset int) (ClipPage, error) {
	items, err := a.store.GetAll(limit, offset)
	if err != nil {
		return ClipPage{}, err
	}
	total, err := a.store.CountAll()
	if err != nil {
		return ClipPage{}, err
	}
	return clipPage(items, total, limit, offset), nil
}

func (a *App) GetClipPageByType(clipType string, limit, offset int) (ClipPage, error) {
	items, err := a.store.GetByType(clipType, limit, offset)
	if err != nil {
		return ClipPage{}, err
	}
	total, err := a.store.CountByType(clipType)
	if err != nil {
		return ClipPage{}, err
	}
	return clipPage(items, total, limit, offset), nil
}

func (a *App) GetClipPageByTypeAndTag(clipType, tag string, limit, offset int) (ClipPage, error) {
	items, err := a.store.GetByTypeAndTag(clipType, tag, limit, offset)
	if err != nil {
		return ClipPage{}, err
	}
	total, err := a.store.CountByTypeAndTag(clipType, tag)
	if err != nil {
		return ClipPage{}, err
	}
	return clipPage(items, total, limit, offset), nil
}

func (a *App) GetClipTags(query, clipType string) ([]ClipTag, error) {
	tags, err := a.store.ListTags(query, clipType)
	if err != nil {
		return nil, err
	}
	result := make([]ClipTag, 0, len(tags))
	for _, tag := range tags {
		result = append(result, ClipTag{Name: tag.Name, Count: tag.Count})
	}
	return result, nil
}

func (a *App) SearchClips(query string) ([]clipboard.ClipEntry, error) {
	return a.store.Search(query, 50, 0)
}

func (a *App) SearchClipPage(query string, limit, offset int) (ClipPage, error) {
	items, err := a.store.Search(query, limit, offset)
	if err != nil {
		return ClipPage{}, err
	}
	total, err := a.store.CountSearch(query)
	if err != nil {
		return ClipPage{}, err
	}
	return clipPage(items, total, limit, offset), nil
}

func (a *App) SearchClipPageByType(query, clipType string, limit, offset int) (ClipPage, error) {
	items, err := a.store.SearchByType(query, clipType, limit, offset)
	if err != nil {
		return ClipPage{}, err
	}
	total, err := a.store.CountSearchByType(query, clipType)
	if err != nil {
		return ClipPage{}, err
	}
	return clipPage(items, total, limit, offset), nil
}

func (a *App) SearchClipPageByTypeAndTag(query, clipType, tag string, limit, offset int) (ClipPage, error) {
	items, err := a.store.SearchByTypeAndTag(query, clipType, tag, limit, offset)
	if err != nil {
		return ClipPage{}, err
	}
	total, err := a.store.CountSearchByTypeAndTag(query, clipType, tag)
	if err != nil {
		return ClipPage{}, err
	}
	return clipPage(items, total, limit, offset), nil
}

func (a *App) GetClipDetails(id int64) (clipboard.ClipEntry, error) {
	entry, err := a.store.GetByID(id)
	if err != nil {
		return clipboard.ClipEntry{}, err
	}
	if entry == nil {
		return clipboard.ClipEntry{}, fmt.Errorf("clip %d not found", id)
	}
	return sanitizeClipDetails(*entry), nil
}

func (a *App) UpdateClipMetadata(id int64, note, tags string) (clipboard.ClipEntry, error) {
	entry, err := a.store.UpdateMetadata(id, note, tags)
	if err != nil {
		return clipboard.ClipEntry{}, err
	}
	if entry == nil {
		return clipboard.ClipEntry{}, fmt.Errorf("clip %d not found", id)
	}
	// 前端拿到返回值后本地更新对应条目，无需广播 clips:changed 触发整页重载。
	a.logInfof("updated clip metadata id=%d noteLen=%d tagsLen=%d", id, len([]rune(entry.Note)), len([]rune(entry.Tags)))
	return sanitizeClipDetails(*entry), nil
}

func sanitizeClipDetails(entry clipboard.ClipEntry) clipboard.ClipEntry {
	if entry.Type == "image" {
		entry.Content = ""
	}
	return entry
}

func clipPage(items []clipboard.ClipEntry, total, limit, offset int) ClipPage {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return ClipPage{
		Items:   items,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
		HasMore: offset+len(items) < total,
	}
}

func (a *App) GetImageDataURL(id int64) (string, error) {
	entry, err := a.store.GetByID(id)
	if err != nil || entry == nil {
		return "", err
	}
	if entry.Type != "image" {
		return "", nil
	}

	if entry.Content == "" {
		return "", nil
	}

	// Try as a file path first (new format). If the file exists, use it.
	if _, err := os.Stat(entry.Content); err == nil {
		return clipboard.GetImageDataURL(entry.Content)
	}

	// Fallback: old entries stored as base64-encoded BMP data in the DB.
	return "data:image/bmp;base64," + entry.Content, nil
}

func (a *App) ExportClip(id int64) (ClipExportResult, error) {
	if a.ctx == nil {
		return ClipExportResult{}, fmt.Errorf("app is not ready")
	}
	entry, err := a.store.GetByID(id)
	if err != nil {
		return ClipExportResult{}, err
	}
	if entry == nil {
		return ClipExportResult{}, fmt.Errorf("clip %d not found", id)
	}

	defaultName, extension, filters := exportDialogDefaults(*entry)
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "导出剪贴板记录",
		DefaultFilename: defaultName,
		Filters:         filters,
	})
	if err != nil {
		a.logErrorf("export clip dialog failed id=%d: %v", id, err)
		return ClipExportResult{}, err
	}
	if path == "" {
		return ClipExportResult{Type: entry.Type, Cancelled: true}, nil
	}
	path = ensureExtension(path, extension)
	bytes, err := exportClipToPath(*entry, path)
	if err != nil {
		a.logErrorf("export clip failed id=%d path=%s: %v", id, path, err)
		return ClipExportResult{}, err
	}
	a.logInfof("exported clip id=%d type=%s bytes=%d path=%s", id, entry.Type, bytes, path)
	return ClipExportResult{Path: path, Type: entry.Type, Bytes: bytes}, nil
}

func exportDialogDefaults(entry clipboard.ClipEntry) (string, string, []runtime.FileFilter) {
	stamp := time.UnixMilli(entry.Timestamp).Format("20060102-150405")
	if entry.Timestamp <= 0 {
		stamp = time.Now().Format("20060102-150405")
	}
	switch entry.Type {
	case "text":
		name := "clipbox-text-" + stamp + "-" + safeFilenamePart(entry.Preview, 32) + ".txt"
		return name, ".txt", []runtime.FileFilter{
			{DisplayName: "Text File (*.txt)", Pattern: "*.txt"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		}
	case "image":
		name := "clipbox-image-" + stamp + ".bmp"
		return name, ".bmp", []runtime.FileFilter{
			{DisplayName: "Bitmap Image (*.bmp)", Pattern: "*.bmp"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		}
	default:
		name := "clipbox-clip-" + stamp + ".txt"
		return name, ".txt", []runtime.FileFilter{
			{DisplayName: "Text File (*.txt)", Pattern: "*.txt"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		}
	}
}

func exportClipToPath(entry clipboard.ClipEntry, path string) (int64, error) {
	if path == "" {
		return 0, os.ErrInvalid
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return 0, err
	}
	var data []byte
	switch entry.Type {
	case "text":
		data = []byte(entry.Content)
	case "image":
		dib, err := loadImageDib(entry.Content)
		if err != nil {
			return 0, err
		}
		data = clipboard.DibToBmp(dib)
		if len(data) == 0 {
			return 0, fmt.Errorf("image data is not a valid DIB")
		}
	default:
		return 0, fmt.Errorf("unsupported clip type: %s", entry.Type)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return 0, err
	}
	return int64(len(data)), nil
}

func ensureExtension(path, extension string) string {
	if path == "" || extension == "" {
		return path
	}
	if filepath.Ext(path) != "" {
		return path
	}
	return path + extension
}

func safeFilenamePart(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "clip"
	}
	var builder strings.Builder
	lastDash := false
	count := 0
	for _, r := range text {
		if maxRunes > 0 && count >= maxRunes {
			break
		}
		if isFilenameSafeRune(r) {
			builder.WriteRune(r)
			lastDash = false
			count++
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
			count++
		}
	}
	result := strings.Trim(builder.String(), "-. ")
	if result == "" {
		return "clip"
	}
	return result
}

func isFilenameSafeRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	case r >= '0' && r <= '9':
		return true
	case r == '-', r == '_':
		return true
	case r >= 0x4e00 && r <= 0x9fff:
		return true
	default:
		return false
	}
}

// copyEntryToClipboard writes a clip entry's content to the Windows clipboard.
// Returns the entry so callers can use it for further actions (paste, etc.).
func (a *App) copyEntryToClipboard(id int64) (*clipboard.ClipEntry, error) {
	entry, err := a.store.GetByID(id)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, fmt.Errorf("clip %d not found", id)
	}

	clipboard.MarkSelfWrite()

	if entry.Type == "text" {
		if err := clipboard.WriteTextToClipboard(entry.Content); err != nil {
			clipboard.ClearSelfWrite()
			return nil, err
		}
	} else if entry.Type == "image" {
		dibData, err := loadImageDib(entry.Content)
		if err != nil {
			clipboard.ClearSelfWrite()
			return nil, err
		}
		if err := clipboard.WriteDibToClipboard(dibData); err != nil {
			clipboard.ClearSelfWrite()
			return nil, err
		}
	} else {
		clipboard.ClearSelfWrite()
		return nil, fmt.Errorf("unsupported clip type: %s", entry.Type)
	}
	return entry, nil
}

func (a *App) CopyToClipboard(id int64) error {
	if _, err := a.copyEntryToClipboard(id); err != nil {
		return err
	}

	autoPaste := a.loadAppSettings().AutoPaste
	go func() {
		a.mu.Lock()
		pinned := a.windowPinned
		a.mu.Unlock()
		if !pinned {
			a.hideWindow()
		}
		if autoPaste {
			a.pasteToLastTarget()
		}
	}()
	return nil
}

// CopyAndPaste copies the clip AND immediately pastes it into the
// previously active window. Unlike CopyToClipboard, this always pastes
// regardless of the auto-paste setting and does not hide the window.
func (a *App) CopyAndPaste(id int64) error {
	if _, err := a.copyEntryToClipboard(id); err != nil {
		return err
	}
	go a.pasteToLastTarget()
	return nil
}

// loadImageDib reads DIB data for an image entry.
// New entries have a file path in Content; old entries have base64 BMP data.
func loadImageDib(content string) ([]byte, error) {
	if content == "" {
		return nil, os.ErrNotExist
	}

	// Try as a file path first (new format).
	if _, err := os.Stat(content); err == nil {
		return clipboard.LoadDibFromFile(content)
	}

	// Fallback: old format — base64-encoded BMP in the DB.
	bmpData, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		return nil, fmt.Errorf("image not found on disk and not valid base64: %w", err)
	}
	return clipboard.BmpToDib(bmpData), nil
}

func (a *App) HideWindow() {
	a.hideWindow()
}

func (a *App) ToggleWindowPin() bool {
	a.mu.Lock()
	a.windowPinned = !a.windowPinned
	pinned := a.windowPinned
	a.mu.Unlock()
	return pinned
}

func (a *App) GetWindowPinned() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.windowPinned
}

func (a *App) TogglePin(id int64) error {
	return a.store.TogglePin(id)
}

func (a *App) DeleteClip(id int64) error {
	return a.store.Delete(id)
}

func (a *App) ClearAll() (storage.ClearResult, error) {
	result, err := a.store.Clear()
	if err != nil {
		return storage.ClearResult{}, err
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "clips:changed")
	}
	a.logInfof("cleared clips deleted=%d images=%d pinnedKept=%d", result.DeletedClips, result.DeletedImages, result.PinnedKept)
	return result, nil
}

func (a *App) GetStorageStats() (storage.Stats, error) {
	return a.store.Stats()
}

func (a *App) RunCleanupNow() (storage.Stats, error) {
	if err := a.applyCurrentCleanup(); err != nil {
		return storage.Stats{}, err
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "clips:changed")
	}
	return a.store.Stats()
}

func (a *App) RunStorageMaintenance() (storage.MaintenanceResult, error) {
	result, err := a.store.Maintain()
	if err != nil {
		a.logErrorf("storage maintenance failed: %v", err)
		return storage.MaintenanceResult{}, err
	}
	a.logInfof(
		"storage maintenance removedMissingImages=%d dbBefore=%d dbAfter=%d totalBefore=%d totalAfter=%d optimized=%v vacuumed=%v",
		result.MissingImagesRemoved,
		result.DatabaseBytesBefore,
		result.DatabaseBytesAfter,
		result.TotalClipsBefore,
		result.TotalClipsAfter,
		result.Optimized,
		result.Vacuumed,
	)
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "clips:changed")
	}
	return result, nil
}

func (a *App) GetAppInfo() (AppInfo, error) {
	exePath, _ := os.Executable()
	dataDir := a.dataDir
	return AppInfo{
		Name:         appName,
		Version:      appVersion,
		DataDir:      dataDir,
		DatabasePath: filepath.Join(dataDir, "clipbox.db"),
		ImageDir:     filepath.Join(dataDir, "images"),
		ExePath:      exePath,
		StartHidden:  a.startHidden,
	}, nil
}

func (a *App) GetLastTargetInfo() TargetWindowInfo {
	a.mu.Lock()
	target := a.lastTarget
	a.mu.Unlock()
	if target == 0 {
		return TargetWindowInfo{}
	}
	info := a.infoForWindow(target)
	return TargetWindowInfo{
		Available:   info.ProcessName != "" || info.ProcessPath != "" || info.Title != "",
		ProcessName: info.ProcessName,
		ProcessPath: info.ProcessPath,
		Title:       info.Title,
	}
}

func (a *App) infoForWindow(hwnd uintptr) windowutil.ForegroundInfo {
	if a.windowInfo != nil {
		return a.windowInfo(hwnd)
	}
	return windowutil.WindowInfo(hwnd)
}

func (a *App) OpenDataDir() error {
	if a.dataDir == "" {
		return fmt.Errorf("data directory is not configured")
	}
	if err := os.MkdirAll(a.dataDir, 0755); err != nil {
		return err
	}
	return exec.Command("explorer.exe", a.dataDir).Start()
}

func (a *App) ExportBackup() (storage.BackupStats, error) {
	if a.ctx == nil {
		return storage.BackupStats{}, fmt.Errorf("app is not ready")
	}
	defaultName := "clipbox-backup-" + time.Now().Format("20060102-150405") + ".clipbox-backup"
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "导出 ClipBox 备份",
		DefaultFilename: defaultName,
		Filters: []runtime.FileFilter{
			{DisplayName: "ClipBox Backup (*.clipbox-backup)", Pattern: "*.clipbox-backup"},
			{DisplayName: "Zip Archive (*.zip)", Pattern: "*.zip"},
		},
	})
	if err != nil {
		a.logErrorf("export backup dialog failed: %v", err)
		return storage.BackupStats{}, err
	}
	stats, err := a.store.ExportBackupFile(path, appVersion)
	if err != nil {
		a.logErrorf("export backup failed: %v", err)
		return storage.BackupStats{}, err
	}
	if !stats.Cancelled {
		a.logInfof("exported backup path=%s clips=%d images=%d settings=%d skippedImages=%d", stats.Path, stats.Clips, stats.Images, stats.Settings, stats.SkippedImages)
	}
	return stats, nil
}

func (a *App) ImportBackup() (storage.BackupStats, error) {
	if a.ctx == nil {
		return storage.BackupStats{}, fmt.Errorf("app is not ready")
	}
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "导入 ClipBox 备份",
		Filters: []runtime.FileFilter{
			{DisplayName: "ClipBox Backup (*.clipbox-backup)", Pattern: "*.clipbox-backup"},
			{DisplayName: "Zip Archive (*.zip)", Pattern: "*.zip"},
		},
	})
	if err != nil {
		a.logErrorf("import backup dialog failed: %v", err)
		return storage.BackupStats{}, err
	}
	stats, err := a.store.ImportBackupFile(path)
	if err != nil {
		a.logErrorf("import backup failed: %v", err)
		return storage.BackupStats{}, err
	}
	a.applyImportedRuntimeSettings()
	if a.ctx != nil && !stats.Cancelled {
		runtime.EventsEmit(a.ctx, "settings:updated", a.loadAppSettings())
		runtime.EventsEmit(a.ctx, "clips:changed")
	}
	if !stats.Cancelled {
		a.logInfof("imported backup path=%s clips=%d images=%d settings=%d skippedImages=%d", stats.Path, stats.Clips, stats.Images, stats.Settings, stats.SkippedImages)
	}
	return stats, nil
}

func (a *App) applyImportedRuntimeSettings() {
	// 备份导入直接写了 settings 表，缓存必须失效重建。
	a.invalidateSettingsCache()
	if a.hotkeyManager != nil {
		config := a.GetHotkeySettings()
		_ = a.hotkeyManager.Rebind(uint32(config.Modifiers), uint32(config.KeyCode))
	}
	_ = a.applyCurrentCleanup()
	a.updateTrayState()
}

func (a *App) QuitApp() {
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
}

func (a *App) GetHotkeySettings() HotkeyConfig {
	modStr, _ := a.store.GetSetting("hotkey_modifiers")
	vkStr, _ := a.store.GetSetting("hotkey_key")
	mod := hotkey.ModControl | hotkey.ModAlt
	vk := 0x56
	if modStr != "" {
		if v, err := strconv.ParseUint(modStr, 10, 32); err == nil {
			mod = int(v)
		}
	}
	if vkStr != "" {
		if v, err := strconv.ParseUint(vkStr, 10, 32); err == nil {
			vk = int(v)
		}
	}
	return HotkeyConfig{
		Modifiers: mod,
		KeyCode:   vk,
		Display:   hotkeyDisplayName(mod, vk),
	}
}

func (a *App) UpdateHotkey(modifiers, keyCode int) error {
	if modifiers == 0 {
		return fmt.Errorf("必须包含至少一个修饰键(Ctrl/Alt/Shift/Win)")
	}
	if keyCode == 0 {
		return fmt.Errorf("必须指定一个按键")
	}

	err := a.hotkeyManager.Rebind(uint32(modifiers), uint32(keyCode))
	if err != nil {
		a.logErrorf("update hotkey failed modifiers=%d key=%d: %v", modifiers, keyCode, err)
		if hotkey.IsHotkeyOccupiedError(err) {
			return fmt.Errorf("%v\n当前热键保持不变，请换一个组合键后重试", err)
		}
		return err
	}

	_ = a.store.SetSetting("hotkey_modifiers", strconv.Itoa(modifiers))
	_ = a.store.SetSetting("hotkey_key", strconv.Itoa(keyCode))
	a.logInfof("updated hotkey modifiers=%d key=%d", modifiers, keyCode)
	return nil
}
