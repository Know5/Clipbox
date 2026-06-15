package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clipbox/internal/clipboard"
	"clipbox/internal/storage"
)

func newTestAppWithStore(t *testing.T) *App {
	t.Helper()

	dir := t.TempDir()
	store, err := storage.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	app := NewApp(dir, false)
	app.store = store
	return app
}

func TestGetClipDetailsReturnsFullText(t *testing.T) {
	app := newTestAppWithStore(t)
	content := strings.Repeat("full text ", 80)
	id, err := app.store.SaveOrUpdate(clipboard.ClipEntry{
		Type:        "text",
		Content:     content,
		Preview:     content[:120],
		Timestamp:   1000,
		SourceApp:   "notepad.exe",
		SourceTitle: "Notes",
	})
	if err != nil {
		t.Fatalf("SaveOrUpdate text: %v", err)
	}

	detail, err := app.GetClipDetails(id)
	if err != nil {
		t.Fatalf("GetClipDetails() error = %v", err)
	}
	if detail.Content != content {
		t.Fatalf("detail.Content length = %d, want %d", len(detail.Content), len(content))
	}
	if detail.SourceApp != "notepad.exe" || detail.SourceTitle != "Notes" {
		t.Fatalf("detail source = %+v", detail)
	}
}

func TestGetClipDetailsSanitizesImageContent(t *testing.T) {
	app := newTestAppWithStore(t)
	imageDir := filepath.Join(app.dataDir, "images")
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		t.Fatalf("MkdirAll image dir: %v", err)
	}
	imagePath := filepath.Join(imageDir, "detail.dib")
	if err := os.WriteFile(imagePath, []byte("image bytes"), 0644); err != nil {
		t.Fatalf("WriteFile image: %v", err)
	}
	id, err := app.store.SaveOrUpdate(clipboard.ClipEntry{
		Type:        "image",
		Content:     imagePath,
		Preview:     "[图片]",
		Timestamp:   1000,
		SourceApp:   "SnippingTool.exe",
		SourceTitle: "截图工具",
	})
	if err != nil {
		t.Fatalf("SaveOrUpdate image: %v", err)
	}

	detail, err := app.GetClipDetails(id)
	if err != nil {
		t.Fatalf("GetClipDetails() error = %v", err)
	}
	if detail.Content != "" {
		t.Fatalf("image detail Content = %q, want empty", detail.Content)
	}
	if detail.SourceApp != "SnippingTool.exe" || detail.SourceTitle != "截图工具" {
		t.Fatalf("image detail source = %+v", detail)
	}
}

func TestUpdateClipMetadataReturnsSanitizedImageDetails(t *testing.T) {
	app := newTestAppWithStore(t)
	imageDir := filepath.Join(app.dataDir, "images")
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		t.Fatalf("MkdirAll image dir: %v", err)
	}
	imagePath := filepath.Join(imageDir, "metadata.dib")
	if err := os.WriteFile(imagePath, []byte("image bytes"), 0644); err != nil {
		t.Fatalf("WriteFile image: %v", err)
	}
	id, err := app.store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "image",
		Content:   imagePath,
		Preview:   "[图片]",
		Timestamp: 1000,
	})
	if err != nil {
		t.Fatalf("SaveOrUpdate image: %v", err)
	}

	detail, err := app.UpdateClipMetadata(id, "  image memo  ", "shot, review, shot")
	if err != nil {
		t.Fatalf("UpdateClipMetadata() error = %v", err)
	}
	if detail.Content != "" {
		t.Fatalf("updated image Content = %q, want empty", detail.Content)
	}
	if detail.Note != "image memo" || detail.Tags != "shot, review" {
		t.Fatalf("updated metadata = %+v", detail)
	}
}

func TestExportClipToPathWritesText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "export.txt")
	bytes, err := exportClipToPath(clipboard.ClipEntry{
		Type:    "text",
		Content: "hello export",
	}, path)
	if err != nil {
		t.Fatalf("exportClipToPath text: %v", err)
	}
	if bytes != int64(len("hello export")) {
		t.Fatalf("exported text bytes = %d, want %d", bytes, len("hello export"))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile text export: %v", err)
	}
	if string(data) != "hello export" {
		t.Fatalf("text export = %q, want original content", data)
	}
}

func TestExportClipToPathWritesImageAsBmp(t *testing.T) {
	dir := t.TempDir()
	imagePath := filepath.Join(dir, "source.dib")
	if err := os.WriteFile(imagePath, minimalDib(), 0644); err != nil {
		t.Fatalf("WriteFile DIB: %v", err)
	}
	exportPath := filepath.Join(dir, "export.bmp")
	bytes, err := exportClipToPath(clipboard.ClipEntry{
		Type:    "image",
		Content: imagePath,
	}, exportPath)
	if err != nil {
		t.Fatalf("exportClipToPath image: %v", err)
	}
	data, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("ReadFile image export: %v", err)
	}
	if bytes != int64(len(data)) {
		t.Fatalf("exported image bytes = %d, file bytes = %d", bytes, len(data))
	}
	if len(data) < 14 || data[0] != 'B' || data[1] != 'M' {
		t.Fatalf("image export header = %q, want BMP", data[:2])
	}
}

func TestExportFilenameHelpers(t *testing.T) {
	if got := ensureExtension(`C:\tmp\clip`, ".txt"); got != `C:\tmp\clip.txt` {
		t.Fatalf("ensureExtension no ext = %q", got)
	}
	if got := ensureExtension(`C:\tmp\clip.md`, ".txt"); got != `C:\tmp\clip.md` {
		t.Fatalf("ensureExtension keeps ext = %q", got)
	}
	if got := safeFilenamePart(`  a/b:c*中文  `, 20); got != "a-b-c-中文" {
		t.Fatalf("safeFilenamePart = %q", got)
	}
}

func minimalDib() []byte {
	dib := make([]byte, 44)
	dib[0] = 40 // BITMAPINFOHEADER size.
	dib[4] = 1  // width
	dib[8] = 1  // height
	dib[12] = 1 // planes
	dib[14] = 24
	dib[20] = 4 // image size, including row padding.
	dib[40] = 0xff
	dib[41] = 0xff
	dib[42] = 0xff
	return dib
}
