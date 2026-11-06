package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"clipbox/internal/clipboard"
	"clipbox/internal/hotkey"
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

// sizedDib builds a 24-bit DIB with the given dimensions and blank pixels.
func sizedDib(width, height int) []byte {
	stride := (width*3 + 3) &^ 3
	pixels := stride * height
	dib := make([]byte, 40+pixels)
	binary.LittleEndian.PutUint32(dib[0:], 40)
	binary.LittleEndian.PutUint32(dib[4:], uint32(int32(width)))
	binary.LittleEndian.PutUint32(dib[8:], uint32(int32(height)))
	dib[12] = 1 // planes
	dib[14] = 24
	binary.LittleEndian.PutUint32(dib[20:], uint32(pixels))
	return dib
}

func TestGetClipImageMeta(t *testing.T) {
	app := newTestAppWithStore(t)

	dib := sizedDib(7, 3)
	imagePath := filepath.Join(t.TempDir(), "clip.dib")
	if err := os.WriteFile(imagePath, dib, 0644); err != nil {
		t.Fatalf("WriteFile DIB: %v", err)
	}
	imageID, err := app.store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "image",
		Content:   imagePath,
		Preview:   "[图片]",
		Timestamp: 1000,
	})
	if err != nil {
		t.Fatalf("SaveOrUpdate image: %v", err)
	}
	textID, err := app.store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "text",
		Content:   "plain text",
		Preview:   "plain text",
		Timestamp: 1001,
	})
	if err != nil {
		t.Fatalf("SaveOrUpdate text: %v", err)
	}
	legacyID, err := app.store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "image",
		Content:   base64.StdEncoding.EncodeToString([]byte("legacy bmp bytes")),
		Preview:   "[图片]",
		Timestamp: 1002,
	})
	if err != nil {
		t.Fatalf("SaveOrUpdate legacy image: %v", err)
	}

	metas, err := app.GetClipImageMeta([]int64{imageID, textID, legacyID, 999999})
	if err != nil {
		t.Fatalf("GetClipImageMeta() error = %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("GetClipImageMeta() returned %d items, want 1: %+v", len(metas), metas)
	}
	got := metas[0]
	want := ClipImageMeta{ID: imageID, Width: 7, Height: 3, Bytes: int64(len(dib))}
	if got != want {
		t.Fatalf("GetClipImageMeta() = %+v, want %+v", got, want)
	}
}

func TestGetSourceIconNotepad(t *testing.T) {
	const notepad = `C:\Windows\System32\notepad.exe`
	if _, err := os.Stat(notepad); err != nil {
		t.Skipf("notepad.exe not available: %v", err)
	}

	app := NewApp(t.TempDir(), false)
	url, err := app.GetSourceIcon(notepad)
	if err != nil {
		t.Fatalf("GetSourceIcon() error = %v", err)
	}
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(url, prefix) {
		t.Fatalf("GetSourceIcon() = %.40q, want PNG data URL", url)
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(url, prefix))
	if err != nil {
		t.Fatalf("decode icon payload: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode icon PNG: %v", err)
	}
	if bounds := img.Bounds(); bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		t.Fatalf("icon bounds = %v, want non-zero", bounds)
	}
}

func TestGetSourceIconConcurrent(t *testing.T) {
	const notepad = `C:\Windows\System32\notepad.exe`
	if _, err := os.Stat(notepad); err != nil {
		t.Skipf("notepad.exe not available: %v", err)
	}

	app := NewApp(t.TempDir(), false)
	const callers = 8
	var wg sync.WaitGroup
	errs := make([]error, callers)
	urls := make([]string, callers)
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			urls[i], errs[i] = app.GetSourceIcon(notepad)
		}()
	}
	wg.Wait()

	for i := range callers {
		if errs[i] != nil {
			t.Fatalf("caller %d error = %v", i, errs[i])
		}
		if urls[i] != urls[0] {
			t.Fatalf("caller %d returned a different icon", i)
		}
	}
}

func TestReservedHotkeyReason(t *testing.T) {
	const (
		ctrl  = hotkey.ModControl
		alt   = hotkey.ModAlt
		shift = hotkey.ModShift
	)
	cases := []struct {
		name      string
		modifiers int
		keyCode   int
		reserved  bool
	}{
		{"Ctrl+1 copies the first clip", ctrl, '1', true},
		{"Alt+9 pastes the ninth clip", alt, '9', true},
		{"Ctrl+Alt+3 also reaches the list shortcut", ctrl | alt, '3', true},
		{"Ctrl+Numpad5 triggers the same list shortcut", ctrl, 0x65, true},
		{"Ctrl+Enter copies only", ctrl, 0x0D, true},
		{"Ctrl+V is the paste ClipBox sends", ctrl, 'V', true},
		{"Ctrl+0 is not a list shortcut", ctrl, '0', false},
		{"Ctrl+Shift+1 types a symbol, not a digit", ctrl | shift, '1', false},
		{"default Ctrl+Alt+V", ctrl | alt, 'V', false},
		{"Ctrl+Shift+V", ctrl | shift, 'V', false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := reservedHotkeyReason(tc.modifiers, tc.keyCode) != ""; got != tc.reserved {
				t.Fatalf("reservedHotkeyReason(%#x, %#x) reserved = %v, want %v", tc.modifiers, tc.keyCode, got, tc.reserved)
			}
		})
	}
}
