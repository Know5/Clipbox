package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"clipbox/internal/clipboard"
	"clipbox/internal/storage"
)

func TestExportDiagnosticsFileContainsSnapshotAndLog(t *testing.T) {
	dataDir := t.TempDir()
	store, err := storage.NewStore(dataDir)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	if _, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "text",
		Content:   "diagnostic text",
		Preview:   "diagnostic text",
		Timestamp: 1000,
	}); err != nil {
		t.Fatalf("SaveOrUpdate() error = %v", err)
	}
	app := NewApp(dataDir, false)
	app.store = store
	app.logInfof("diagnostic log entry")

	outPath := filepath.Join(t.TempDir(), "diagnostics.zip")
	stats, err := app.exportDiagnosticsFile(outPath)
	if err != nil {
		t.Fatalf("exportDiagnosticsFile() error = %v", err)
	}
	if stats.Cancelled || stats.Files != 3 || stats.LogBytes <= 0 {
		t.Fatalf("diagnostic stats = %+v, want 3 files and log bytes", stats)
	}

	reader, err := zip.OpenReader(outPath)
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	defer reader.Close()

	names := map[string]bool{}
	for _, file := range reader.File {
		names[file.Name] = true
	}
	for _, name := range []string{"manifest.json", "app.json", "clipbox.log"} {
		if !names[name] {
			t.Fatalf("diagnostic zip missing %s; names=%v", name, names)
		}
	}
}

func TestTailFileReturnsLastBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clipbox.log")
	if err := os.WriteFile(path, []byte("0123456789"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	data, err := tailFile(path, 4)
	if err != nil {
		t.Fatalf("tailFile() error = %v", err)
	}
	if string(data) != "6789" {
		t.Fatalf("tailFile() = %q, want 6789", string(data))
	}
}
