package main

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clipbox/internal/clipboard"
	"clipbox/internal/storage"
)

func TestImageAssetServesBinaryAndRangesWithoutExposingPaths(t *testing.T) {
	app := newTestAppWithStore(t)
	path := filepath.Join(app.dataDir, "image.dib")
	dib := minimalDib()
	if err := os.WriteFile(path, dib, 0600); err != nil {
		t.Fatal(err)
	}
	id, err := app.store.SaveOrUpdate(clipboard.ClipEntry{Type: "image", Content: path, Preview: "image", Timestamp: 1})
	if err != nil {
		t.Fatal(err)
	}
	url, err := app.GetImageURL(id)
	if err != nil || strings.Contains(url, app.dataDir) {
		t.Fatalf("image URL = %q, %v", url, err)
	}
	url = strings.TrimSuffix(url, ".png") + ".bmp"
	bmp := clipboard.DibToBmp(dib)
	for _, test := range []struct {
		method, rangeHeader string
		status              int
		body                []byte
	}{
		{"GET", "", 200, bmp}, {"HEAD", "", 200, nil},
		{"GET", "bytes=0-13", 206, bmp[:14]},
		{"GET", "bytes=10-20", 206, bmp[10:21]},
	} {
		r := httptest.NewRequest(test.method, url, nil)
		r.Header.Set("Range", test.rangeHeader)
		w := httptest.NewRecorder()
		app.serveImage(w, r)
		if w.Code != test.status || !bytes.Equal(w.Body.Bytes(), test.body) || w.Header().Get("Content-Type") != "image/bmp" {
			t.Fatalf("%s %s: status=%d bytes=%d", test.method, test.rangeHeader, w.Code, w.Body.Len())
		}
	}
	for _, invalid := range []string{"/clip-image/wrong/1.bmp", "/clip-image/" + app.imageSession + "/../../clipbox.db", "/clip-image/" + app.imageSession + "/999999.bmp"} {
		w := httptest.NewRecorder()
		app.serveImage(w, httptest.NewRequest("GET", invalid, nil))
		if w.Code != 404 {
			t.Fatalf("unexpected asset access for %s: %d", invalid, w.Code)
		}
	}
}

func TestImageAssetLegacyBMPAndMissingFile(t *testing.T) {
	app := newTestAppWithStore(t)
	bmp := clipboard.DibToBmp(minimalDib())
	for _, content := range []string{base64.StdEncoding.EncodeToString(bmp), filepath.Join(app.dataDir, "missing.dib")} {
		id, err := app.store.SaveOrUpdate(clipboard.ClipEntry{Type: "image", Content: content, Preview: "image", Timestamp: 1})
		if err != nil {
			t.Fatal(err)
		}
		url, _ := app.GetImageURL(id)
		w := httptest.NewRecorder()
		app.serveImage(w, httptest.NewRequest("GET", url, nil))
		if strings.HasSuffix(content, ".dib") {
			if w.Code != 404 {
				t.Fatal("missing image must return 404")
			}
		} else if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), bmp) {
			t.Fatal("legacy BMP changed during delivery")
		}
	}
}

func TestCaptureEventDoesNotTransferLargeText(t *testing.T) {
	app := newTestAppWithStore(t)
	body := strings.Repeat("large source ", 100000)
	change, err := app.persistCapturedClip(clipboard.ClipEntry{Type: "text", Content: body, Preview: "short preview", Timestamp: 1})
	if err != nil || change.Entry == nil || change.Entry.Content != "" {
		t.Fatalf("capture event includes the full body: %v", err)
	}
	detail, err := app.GetClipDetails(change.Entry.ID)
	if err != nil || detail.Content != body {
		t.Fatal("full text no longer available on demand")
	}
}

func TestImagePreviewIsBoundedAndOriginalUnchanged(t *testing.T) {
	app := newTestAppWithStore(t)
	dib := sizedDib(3840, 2160)
	path := filepath.Join(app.dataDir, "4k.dib")
	if err := os.WriteFile(path, dib, 0600); err != nil {
		t.Fatal(err)
	}
	id, err := app.store.SaveOrUpdate(clipboard.ClipEntry{Type: "image", Content: path, Preview: "4K", Timestamp: 1})
	if err != nil {
		t.Fatal(err)
	}
	url, _ := app.GetImageURL(id)
	w := httptest.NewRecorder()
	app.serveImage(w, httptest.NewRequest("GET", url, nil))
	preview, err := png.DecodeConfig(bytes.NewReader(w.Body.Bytes()))
	if err != nil || w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || preview.Width != 1280 || preview.Height != 720 {
		t.Fatalf("preview = %+v, status=%d, err=%v", preview, w.Code, err)
	}
	info, _ := os.Stat(path)
	if info.Size() != int64(len(dib)) {
		t.Fatal("preview changed original data")
	}
}

func BenchmarkImageDelivery(b *testing.B) {
	dataDir := b.TempDir()
	store, err := storage.NewStore(dataDir)
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	app := NewApp(dataDir, false)
	app.store = store
	dib := sizedDib(3840, 2160)
	path := filepath.Join(dataDir, "4k.dib")
	if err := os.WriteFile(path, dib, 0600); err != nil {
		b.Fatal(err)
	}
	id, err := store.SaveOrUpdate(clipboard.ClipEntry{Type: "image", Content: path, Preview: "4K", Timestamp: 1})
	if err != nil {
		b.Fatal(err)
	}
	url, _ := app.GetImageURL(id)
	rawURL := strings.TrimSuffix(url, ".png") + ".bmp"
	dib = nil
	b.Run("LegacyDataURL", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := app.GetImageDataURL(id); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("BinaryBufferedLikeWebView", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			w := httptest.NewRecorder()
			app.serveImage(w, httptest.NewRequest("GET", rawURL, nil))
			if w.Code != 200 {
				b.Fatal(w.Code)
			}
		}
	})
	b.Run("BinaryHandlerOnly", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			w := &discardImageResponse{header: make(http.Header)}
			app.serveImage(w, httptest.NewRequest("GET", rawURL, nil))
		}
	})
	b.Run("BoundedPreviewBufferedLikeWebView", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			w := httptest.NewRecorder()
			app.serveImage(w, httptest.NewRequest("GET", url, nil))
			if w.Code != 200 {
				b.Fatal(w.Code)
			}
			b.ReportMetric(float64(w.Body.Len()), "response_bytes")
		}
	})
}

type discardImageResponse struct{ header http.Header }

func (w *discardImageResponse) Header() http.Header         { return w.header }
func (w *discardImageResponse) WriteHeader(int)             {}
func (w *discardImageResponse) Write(p []byte) (int, error) { return io.Discard.Write(p) }
