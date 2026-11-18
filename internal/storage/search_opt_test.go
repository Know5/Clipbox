package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"clipbox/internal/clipboard"
)

func TestSearchMultiTokenHybridAndPage(t *testing.T) {
	store := newTestStore(t)

	_, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "text",
		Content:   "react 微 服务 架构",
		Preview:   "react 微 服务 架构",
		Timestamp: 100,
		Tags:      "Work",
	})
	if err != nil {
		t.Fatalf("SaveOrUpdate hybrid match: %v", err)
	}
	_, err = store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "text",
		Content:   "react only without cjk",
		Preview:   "react only without cjk",
		Timestamp: 200,
		Tags:      "Work",
	})
	if err != nil {
		t.Fatalf("SaveOrUpdate react-only: %v", err)
	}
	pinnedID, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "text",
		Content:   "micro react guide",
		Preview:   "micro react guide",
		Timestamp: 150,
		Pinned:    true,
		Tags:      "Work, Docs",
	})
	if err != nil {
		t.Fatalf("SaveOrUpdate pinned match: %v", err)
	}

	// Hybrid FTS (>=3 rune token "react") + LIKE (<3 rune token "微").
	items, total, err := store.SearchPageByTypeAndTag("react 微", "text", "Work", 50, 0)
	if err != nil {
		t.Fatalf("SearchPageByTypeAndTag hybrid: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].Preview != "react 微 服务 架构" {
		t.Fatalf("hybrid search got %d items (total=%d): %+v", len(items), total, items)
	}

	// Multi-token FTS respects pinned ordering and beyond-last-page offset preserves total.
	page1, total1, err := store.SearchPageByTypeAndTag("react micro", "text", "Work", 1, 0)
	if err != nil {
		t.Fatalf("SearchPageByTypeAndTag multi-token: %v", err)
	}
	if total1 != 1 || len(page1) != 1 || page1[0].ID != pinnedID {
		t.Fatalf("multi-token page got %+v total=%d", page1, total1)
	}

	emptyPage, total2, err := store.SearchPageByTypeAndTag("react", "text", "Work", 10, 50)
	if err != nil {
		t.Fatalf("SearchPageByTypeAndTag empty offset: %v", err)
	}
	wantTotal, err := store.CountSearchByTypeAndTag("react", "text", "Work")
	if err != nil {
		t.Fatalf("CountSearchByTypeAndTag: %v", err)
	}
	if len(emptyPage) != 0 || total2 != wantTotal || wantTotal != 3 {
		t.Fatalf("empty offset page: len=%d total=%d want=%d", len(emptyPage), total2, wantTotal)
	}
}

func TestHasReadableImageHashAndMissingPayload(t *testing.T) {
	store := newTestStore(t)

	if _, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "image",
		Preview:   "empty image",
		Timestamp: 100,
	}); !errors.Is(err, ErrMissingImagePayload) {
		t.Fatalf("SaveOrUpdate empty image error = %v, want ErrMissingImagePayload", err)
	}

	imgDir := filepath.Join(t.TempDir(), "images")
	if err := os.MkdirAll(imgDir, 0755); err != nil {
		t.Fatal(err)
	}
	imgPath := filepath.Join(imgDir, "sample.dib")
	payload := []byte("fake-dib-payload")
	if err := os.WriteFile(imgPath, payload, 0644); err != nil {
		t.Fatal(err)
	}
	digest := hashBytes(payload)

	if _, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:        "image",
		Content:     imgPath,
		ContentHash: digest,
		Preview:     "sample",
		Timestamp:   200,
	}); err != nil {
		t.Fatalf("SaveOrUpdate image: %v", err)
	}

	ok, err := store.HasReadableImageHash(digest)
	if err != nil || !ok {
		t.Fatalf("HasReadableImageHash(present) = (%v, %v), want (true, nil)", ok, err)
	}

	if err := os.Remove(imgPath); err != nil {
		t.Fatal(err)
	}
	ok, err = store.HasReadableImageHash(digest)
	if err != nil || ok {
		t.Fatalf("HasReadableImageHash(removed) = (%v, %v), want (false, nil)", ok, err)
	}
}

func TestGetImagePathsByIDsAndStats(t *testing.T) {
	store := newTestStore(t)

	textID, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "text",
		Content:   "hello",
		Preview:   "hello",
		Timestamp: 100,
		Pinned:    true,
	})
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	livePath := filepath.Join(dir, "live.dib")
	if err := os.WriteFile(livePath, []byte("12345678"), 0644); err != nil {
		t.Fatal(err)
	}
	missingPath := filepath.Join(dir, "missing.dib")

	img1, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "image",
		Content:   livePath,
		Preview:   "live",
		Thumbnail: "thumb-a",
		Timestamp: 200,
	})
	if err != nil {
		t.Fatal(err)
	}
	img2, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "image",
		Content:   missingPath,
		Preview:   "missing",
		Thumbnail: "thumb-bb",
		Timestamp: 300,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetSetting("theme", "dark"); err != nil {
		t.Fatal(err)
	}

	paths, err := store.GetImagePathsByIDs([]int64{0, -1, textID, img1, img1, img2, 9999})
	if err != nil {
		t.Fatalf("GetImagePathsByIDs: %v", err)
	}
	if len(paths) != 2 || paths[img1] != livePath || paths[img2] != missingPath {
		t.Fatalf("GetImagePathsByIDs = %+v", paths)
	}

	stats, err := store.Stats()
	if err != nil {
		t.Fatalf("Stats(): %v", err)
	}
	wantThumbBytes := int64(len("thumb-a") + len("thumb-bb"))
	if stats.TotalClips != 3 || stats.TextClips != 1 || stats.ImageClips != 2 ||
		stats.PinnedClips != 1 || stats.UnpinnedClips != 2 ||
		stats.ThumbnailBytes != wantThumbBytes || stats.SettingsEntries != 1 ||
		stats.ImageBytes != 8 || stats.MissingImages != 1 {
		t.Fatalf("Stats() = %+v", stats)
	}
}
