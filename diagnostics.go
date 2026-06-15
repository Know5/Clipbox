package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"sync"
	"time"

	"clipbox/internal/storage"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const maxDiagnosticLogBytes = 2 * 1024 * 1024

type DiagnosticStats struct {
	Path      string `json:"path"`
	LogBytes  int64  `json:"logBytes"`
	Files     int    `json:"files"`
	Cancelled bool   `json:"cancelled"`
}

type diagnosticManifest struct {
	Format     string `json:"format"`
	Version    int    `json:"version"`
	AppName    string `json:"appName"`
	AppVersion string `json:"appVersion"`
	ExportedAt string `json:"exportedAt"`
}

type diagnosticAppSnapshot struct {
	AppInfo  AppInfo               `json:"appInfo"`
	Settings AppSettings           `json:"settings"`
	Storage  storage.Stats         `json:"storage"`
	Runtime  diagnosticRuntimeInfo `json:"runtime"`
}

type diagnosticRuntimeInfo struct {
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
	GoVersion string `json:"goVersion"`
}

var diagnosticsWriteMu sync.Mutex

func storageLogPath(dataDir string) string {
	if dataDir == "" {
		return ""
	}
	return filepath.Join(dataDir, "clipbox.log")
}

func (a *App) logInfof(format string, args ...interface{}) {
	a.appendLog("INFO", format, args...)
}

func (a *App) logErrorf(format string, args ...interface{}) {
	a.appendLog("ERROR", format, args...)
}

func (a *App) appendLog(level, format string, args ...interface{}) {
	if a == nil || a.logPath == "" {
		return
	}
	diagnosticsWriteMu.Lock()
	defer diagnosticsWriteMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(a.logPath), 0755); err != nil {
		return
	}
	file, err := os.OpenFile(a.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer file.Close()

	message := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(file, "%s [%s] %s\n", time.Now().UTC().Format(time.RFC3339), level, message)
}

func (a *App) ExportDiagnostics() (DiagnosticStats, error) {
	if a.ctx == nil {
		return DiagnosticStats{}, fmt.Errorf("app is not ready")
	}
	defaultName := "clipbox-diagnostics-" + time.Now().Format("20060102-150405") + ".zip"
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "导出 ClipBox 诊断包",
		DefaultFilename: defaultName,
		Filters: []runtime.FileFilter{
			{DisplayName: "Zip Archive (*.zip)", Pattern: "*.zip"},
		},
	})
	if err != nil {
		a.logErrorf("export diagnostics dialog failed: %v", err)
		return DiagnosticStats{}, err
	}
	stats, err := a.exportDiagnosticsFile(path)
	if err != nil {
		a.logErrorf("export diagnostics failed: %v", err)
		return DiagnosticStats{}, err
	}
	if !stats.Cancelled {
		a.logInfof("exported diagnostics path=%s files=%d logBytes=%d", stats.Path, stats.Files, stats.LogBytes)
	}
	return stats, nil
}

func (a *App) exportDiagnosticsFile(path string) (DiagnosticStats, error) {
	if path == "" {
		return DiagnosticStats{Cancelled: true}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return DiagnosticStats{}, err
	}

	tmp := path + ".tmp"
	_ = os.Remove(tmp)
	file, err := os.Create(tmp)
	if err != nil {
		return DiagnosticStats{}, err
	}
	zipWriter := zip.NewWriter(file)
	stats := DiagnosticStats{Path: path}
	cleanup := true
	defer func() {
		_ = zipWriter.Close()
		_ = file.Close()
		if cleanup {
			_ = os.Remove(tmp)
		}
	}()

	manifest := diagnosticManifest{
		Format:     "clipbox-diagnostics",
		Version:    1,
		AppName:    appName,
		AppVersion: appVersion,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := addDiagnosticJSON(zipWriter, "manifest.json", manifest); err != nil {
		return DiagnosticStats{}, err
	}
	stats.Files++

	snapshot, err := a.diagnosticSnapshot()
	if err != nil {
		return DiagnosticStats{}, err
	}
	if err := addDiagnosticJSON(zipWriter, "app.json", snapshot); err != nil {
		return DiagnosticStats{}, err
	}
	stats.Files++

	logBytes, err := addDiagnosticLog(zipWriter, a.logPath)
	if err != nil {
		return DiagnosticStats{}, err
	}
	stats.LogBytes = logBytes
	stats.Files++

	if err := zipWriter.Close(); err != nil {
		return DiagnosticStats{}, err
	}
	if err := file.Close(); err != nil {
		return DiagnosticStats{}, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return DiagnosticStats{}, err
	}
	cleanup = false
	return stats, nil
}

func (a *App) diagnosticSnapshot() (diagnosticAppSnapshot, error) {
	info, err := a.GetAppInfo()
	if err != nil {
		return diagnosticAppSnapshot{}, err
	}
	var stats storage.Stats
	if a.store != nil {
		stats, err = a.store.Stats()
		if err != nil {
			return diagnosticAppSnapshot{}, err
		}
	}
	return diagnosticAppSnapshot{
		AppInfo:  info,
		Settings: a.loadAppSettings(),
		Storage:  stats,
		Runtime: diagnosticRuntimeInfo{
			GOOS:      stdruntime.GOOS,
			GOARCH:    stdruntime.GOARCH,
			GoVersion: stdruntime.Version(),
		},
	}, nil
}

func addDiagnosticJSON(zipWriter *zip.Writer, name string, value interface{}) error {
	writer, err := zipWriter.Create(name)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func addDiagnosticLog(zipWriter *zip.Writer, logPath string) (int64, error) {
	writer, err := zipWriter.Create("clipbox.log")
	if err != nil {
		return 0, err
	}
	if logPath == "" {
		_, err := io.WriteString(writer, "log path is not configured\n")
		return 0, err
	}
	data, err := tailFile(logPath, maxDiagnosticLogBytes)
	if os.IsNotExist(err) {
		_, err := io.WriteString(writer, "log file does not exist yet\n")
		return 0, err
	}
	if err != nil {
		return 0, err
	}
	_, err = writer.Write(data)
	return int64(len(data)), err
}

func tailFile(path string, maxBytes int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	offset := int64(0)
	if size > maxBytes {
		offset = size - maxBytes
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	return io.ReadAll(file)
}
