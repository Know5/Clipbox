package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"clipbox/internal/storage"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const autoBackupSettingLastAt = "auto_backup_last_at"

func (a *App) SelectAutoBackupDir() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("app is not ready")
	}
	path, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "\u9009\u62e9\u81ea\u52a8\u5907\u4efd\u76ee\u5f55",
	})
	if err != nil {
		a.logErrorf("select auto backup dir failed: %v", err)
		return "", err
	}
	return path, nil
}

func (a *App) RunAutoBackupNow() (storage.BackupStats, error) {
	settings := a.loadAppSettings()
	if strings.TrimSpace(settings.AutoBackupDir) == "" {
		return storage.BackupStats{}, fmt.Errorf("auto backup directory is not configured")
	}
	stats, err := a.runAutoBackup(settings, time.Now())
	if err != nil {
		a.logErrorf("manual auto backup failed: %v", err)
		return stats, err
	}
	a.logInfof("manual auto backup path=%s clips=%d images=%d settings=%d skippedImages=%d", stats.Path, stats.Clips, stats.Images, stats.Settings, stats.SkippedImages)
	return stats, nil
}

func (a *App) maybeRunAutoBackup() {
	settings := a.loadAppSettings()
	if !settings.AutoBackupEnabled || strings.TrimSpace(settings.AutoBackupDir) == "" {
		return
	}
	now := time.Now()
	if !a.autoBackupDue(settings, now) {
		return
	}
	stats, err := a.runAutoBackup(settings, now)
	if err != nil {
		a.logErrorf("scheduled auto backup failed: %v", err)
		return
	}
	a.logInfof("scheduled auto backup path=%s clips=%d images=%d settings=%d skippedImages=%d", stats.Path, stats.Clips, stats.Images, stats.Settings, stats.SkippedImages)
}

func (a *App) autoBackupDue(settings AppSettings, now time.Time) bool {
	if a.store == nil {
		return false
	}
	lastRaw, err := a.store.GetSetting(autoBackupSettingLastAt)
	if err != nil || strings.TrimSpace(lastRaw) == "" {
		return true
	}
	lastMs, err := strconv.ParseInt(lastRaw, 10, 64)
	if err != nil || lastMs <= 0 {
		return true
	}
	interval := time.Duration(settings.AutoBackupIntervalDays) * 24 * time.Hour
	return now.Sub(time.UnixMilli(lastMs)) >= interval
}

func (a *App) runAutoBackup(settings AppSettings, now time.Time) (storage.BackupStats, error) {
	if a.store == nil {
		return storage.BackupStats{}, fmt.Errorf("app is not ready")
	}
	dir := strings.TrimSpace(settings.AutoBackupDir)
	if dir == "" {
		return storage.BackupStats{}, fmt.Errorf("auto backup directory is not configured")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return storage.BackupStats{}, err
	}
	path := nextAutoBackupPath(dir, now)
	stats, err := a.store.ExportBackupFile(path, appVersion)
	if err != nil {
		return stats, err
	}
	if !stats.Cancelled {
		if err := pruneAutoBackups(dir, settings.AutoBackupMaxFiles); err != nil {
			return stats, err
		}
		if err := a.store.SetSetting(autoBackupSettingLastAt, strconv.FormatInt(now.UnixMilli(), 10)); err != nil {
			return stats, err
		}
	}
	return stats, nil
}

func nextAutoBackupPath(dir string, now time.Time) string {
	stem := "clipbox-auto-" + now.Format("20060102-150405")
	path := filepath.Join(dir, stem+".clipbox-backup")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	for i := 1; i < 1000; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s-%d.clipbox-backup", stem, i))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	return filepath.Join(dir, stem+"-"+strconv.FormatInt(now.UnixMilli(), 10)+".clipbox-backup")
}

func pruneAutoBackups(dir string, maxFiles int) error {
	if maxFiles < 1 {
		maxFiles = 1
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	type backupFile struct {
		path    string
		modTime time.Time
	}
	files := []backupFile{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "clipbox-auto-") || !strings.HasSuffix(name, ".clipbox-backup") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, backupFile{
			path:    filepath.Join(dir, name),
			modTime: info.ModTime(),
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})
	for i := maxFiles; i < len(files); i++ {
		if err := os.Remove(files[i].path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
