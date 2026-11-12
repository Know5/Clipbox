package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"clipbox/internal/clipboard"
)

// The URL is scoped to this app session and addresses a stored clip ID, never
// an arbitrary filesystem path. Binary delivery avoids base64/JSON/JS copies.
func (a *App) GetImageURL(id int64) (string, error) {
	if a.store == nil {
		return "", fmt.Errorf("存储尚未就绪")
	}
	entry, err := a.store.GetListEntry(id)
	if err != nil {
		return "", err
	}
	if entry == nil || entry.Type != "image" {
		return "", fmt.Errorf("图片记录不存在")
	}
	return "/clip-image/" + a.imageSession + "/" + strconv.FormatInt(id, 10) + ".png", nil
}

func (a *App) serveImage(w http.ResponseWriter, r *http.Request) {
	prefix := "/clip-image/" + a.imageSession + "/"
	isPreview := strings.HasSuffix(r.URL.Path, ".png")
	if a.imageSession == "" || !strings.HasPrefix(r.URL.Path, prefix) || (!isPreview && !strings.HasSuffix(r.URL.Path, ".bmp")) || a.store == nil {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), ".bmp"), ".png"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	entry, err := a.store.GetByID(id)
	if err != nil || entry == nil || entry.Type != "image" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/bmp")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	file, err := os.Open(entry.Content)
	if err == nil {
		defer file.Close()
		info, err := file.Stat()
		if err != nil || info.Size() > 50*1024*1024 {
			http.NotFound(w, r)
			return
		}
		if isPreview {
			// 1280 pixels is ample for the 400px panel including high-DPI
			// displays. Original data remains available for copy/export/OCR.
			preview, previewErr := clipboard.DIBPreview(file, info.Size(), 1280)
			if previewErr == nil {
				w.Header().Set("Content-Type", "image/png")
				if r.Method != http.MethodHead {
					encoder := png.Encoder{CompressionLevel: png.BestSpeed}
					_ = encoder.Encode(w, preview)
				}
				return
			}
			// Keep unusual bitmap formats viewable through the original BMP.
		}
		var dibHeader [124]byte
		n, _ := file.ReadAt(dibHeader[:], 0)
		header, err := clipboard.BMPFileHeader(dibHeader[:n], info.Size())
		if err != nil {
			http.NotFound(w, r)
			return
		}
		reader := &bmpReaderAt{header: header[:], file: file}
		http.ServeContent(w, r, "clip.bmp", info.ModTime(), io.NewSectionReader(reader, 0, info.Size()+14))
		return
	}
	// Old records contain a base64 BMP. Decode directly to the response rather
	// than assembling a second base64 data URL and serializing it over Wails.
	if len(entry.Content) > 70*1024*1024 {
		http.NotFound(w, r)
		return
	}
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(entry.Content))
	var signature [2]byte
	if _, err := io.ReadFull(decoder, signature[:]); err != nil || signature != [2]byte{'B', 'M'} {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.Copy(w, io.MultiReader(bytes.NewReader(signature[:]), io.LimitReader(decoder, 50*1024*1024)))
}

// Present a BMP header followed by the existing DIB file as one seekable
// resource. ServeContent handles HEAD/ranges; the full image is never read
// into an application-owned byte slice.
type bmpReaderAt struct {
	header []byte
	file   io.ReaderAt
}

func (b *bmpReaderAt) ReadAt(p []byte, offset int64) (int, error) {
	if offset < 0 {
		return 0, os.ErrInvalid
	}
	n := 0
	if offset < int64(len(b.header)) {
		n = copy(p, b.header[offset:])
		p, offset = p[n:], offset+int64(n)
	}
	if len(p) == 0 {
		return n, nil
	}
	m, err := b.file.ReadAt(p, offset-int64(len(b.header)))
	return n + m, err
}
