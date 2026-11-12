package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clipbox/internal/clipboard"
)

func TestTextToolsSecretIsProtectedAndOmittedFromBackup(t *testing.T) {
	app := newTestAppWithStore(t)
	settings, err := app.SaveTextToolsSettings(TextToolsSettings{
		Endpoint: "https://example.com/v1", Model: "model", APIKey: "local-test-secret",
	})
	if err != nil || !settings.HasAPIKey || settings.APIKey != "" {
		t.Fatalf("save returned %+v, %v", settings, err)
	}
	protected, _ := app.store.GetSetting(translationSecretKey)
	if protected == "" || strings.Contains(protected, "local-test-secret") {
		t.Fatal("secret was not protected")
	}
	key, err := unprotectTranslationKey(protected)
	if err != nil || key != "local-test-secret" {
		t.Fatalf("unprotect = %q, %v", key, err)
	}
	settings.TargetLanguage = "English"
	if _, err := app.SaveTextToolsSettings(settings); err != nil {
		t.Fatal(err)
	}
	stillProtected, _ := app.store.GetSetting(translationSecretKey)
	if protected != stillProtected {
		t.Fatal("saving blank key replaced existing secret")
	}
	path := filepath.Join(t.TempDir(), "backup.clipbox-backup")
	if _, err := app.store.ExportBackupFile(path, "test"); err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	for _, file := range z.File {
		if file.Name != "settings.json" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil || strings.Contains(string(data), translationSecretKey) || strings.Contains(string(data), "local-test-secret") || strings.Contains(string(data), protected) {
			t.Fatalf("secret exported: %v", err)
		}
	}
	settings.ClearAPIKey = true
	settings, err = app.SaveTextToolsSettings(settings)
	if err != nil || settings.HasAPIKey {
		t.Fatalf("clear returned %+v, %v", settings, err)
	}
}

func TestTextTaskCancellationAndDuplicateID(t *testing.T) {
	app := newTestAppWithStore(t)
	ctx, done, err := app.beginTextTask("one")
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	if _, _, err := app.beginTextTask("one"); err == nil {
		t.Fatal("duplicate task ID accepted")
	}
	app.CancelTextTask("one")
	if ctx.Err() != context.Canceled {
		t.Fatalf("ctx.Err() = %v", ctx.Err())
	}
}

func TestBackupImportCannotReplaceLocalTranslationKey(t *testing.T) {
	app := newTestAppWithStore(t)
	if err := app.store.SetSetting(translationSecretKey, "local-encrypted-value"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "injected.clipbox-backup")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(file)
	for name, value := range map[string]any{
		"manifest.json": map[string]any{"format": "clipbox-backup", "version": 1},
		"clips.json":    []any{},
		"settings.json": []map[string]string{{"key": translationSecretKey, "value": "foreign-secret"}},
	} {
		writer, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.NewEncoder(writer).Encode(value); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if _, err := app.store.ImportBackupFile(path); err != nil {
		t.Fatal(err)
	}
	value, _ := app.store.GetSetting(translationSecretKey)
	if value != "local-encrypted-value" {
		t.Fatal("backup import overwrote the local key")
	}
}

func TestRecognizeStoredImageIntegration(t *testing.T) {
	path := os.Getenv("CLIPBOX_OCR_TEST_IMAGE")
	if path == "" {
		t.Skip("set CLIPBOX_OCR_TEST_IMAGE to exercise stored-image OCR")
	}
	app := newTestAppWithStore(t)
	bmp, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	imageDir := filepath.Join(app.dataDir, "images")
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		t.Fatal(err)
	}
	dibPath := filepath.Join(imageDir, "ocr.dib")
	if err := os.WriteFile(dibPath, clipboard.BmpToDib(bmp), 0600); err != nil {
		t.Fatal(err)
	}
	id, err := app.store.SaveOrUpdate(clipboard.ClipEntry{Type: "image", Content: dibPath, Preview: "test image", Timestamp: time.Now().UnixMilli()})
	if err != nil {
		t.Fatal(err)
	}
	result, err := app.RecognizeClipText(id, "", "stored-image-test")
	if err != nil || !strings.Contains(strings.ToLower(result.Text), "hello") {
		t.Fatalf("stored OCR = %+v, %v", result, err)
	}
	entry, _ := app.store.GetByID(id)
	if entry.Content != dibPath || entry.Type != "image" {
		t.Fatal("OCR modified the source record")
	}
}
