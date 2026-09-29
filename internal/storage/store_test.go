package storage

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clipbox/internal/clipboard"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()

	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func writeTestBackup(t *testing.T, path string, clips []backupClip, settings []backupSetting, images map[string][]byte) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create backup: %v", err)
	}
	zipWriter := zip.NewWriter(file)
	if err := writeZipJSON(zipWriter, "manifest.json", backupManifest{
		Format:     backupFormat,
		Version:    backupVersion,
		AppVersion: "test",
		ExportedAt: "2026-06-09T00:00:00Z",
	}); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := writeZipJSON(zipWriter, "settings.json", settings); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	if err := writeZipJSON(zipWriter, "clips.json", clips); err != nil {
		t.Fatalf("write clips: %v", err)
	}
	for name, data := range images {
		writer, err := zipWriter.Create("images/" + name)
		if err != nil {
			t.Fatalf("create image %s: %v", name, err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatalf("write image %s: %v", name, err)
		}
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close backup: %v", err)
	}
}

func TestClearKeepsPinnedClips(t *testing.T) {
	store := newTestStore(t)

	pinnedID, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "text",
		Content:   "keep me",
		Preview:   "keep me",
		Timestamp: 1000,
	})
	if err != nil {
		t.Fatalf("SaveOrUpdate pinned clip: %v", err)
	}
	if _, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "text",
		Content:   "delete me",
		Preview:   "delete me",
		Timestamp: 2000,
	}); err != nil {
		t.Fatalf("SaveOrUpdate unpinned clip: %v", err)
	}
	if err := store.TogglePin(pinnedID); err != nil {
		t.Fatalf("TogglePin() error = %v", err)
	}

	result, err := store.Clear()
	if err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if result.DeletedClips != 1 {
		t.Fatalf("Clear() DeletedClips = %d, want 1", result.DeletedClips)
	}
	if result.PinnedKept != 1 {
		t.Fatalf("Clear() PinnedKept = %d, want 1", result.PinnedKept)
	}
	if result.Stats.TotalClips != 1 || result.Stats.PinnedClips != 1 {
		t.Fatalf("Clear() Stats = %+v, want only one pinned clip", result.Stats)
	}

	clips, err := store.GetAll(50, 0)
	if err != nil {
		t.Fatalf("GetAll() error = %v", err)
	}
	if len(clips) != 1 || clips[0].ID != pinnedID || !clips[0].Pinned {
		t.Fatalf("Clear() clips = %+v, want only pinned clip %d", clips, pinnedID)
	}
}

func TestClearReportsAndRemovesNonPinnedImages(t *testing.T) {
	store := newTestStore(t)
	imageDir := filepath.Join(store.dataDir, "images")
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	deletePath := filepath.Join(imageDir, "delete.dib")
	pinnedPath := filepath.Join(imageDir, "keep.dib")
	if err := os.WriteFile(deletePath, []byte("delete image bytes"), 0644); err != nil {
		t.Fatalf("WriteFile delete image: %v", err)
	}
	if err := os.WriteFile(pinnedPath, []byte("keep image bytes"), 0644); err != nil {
		t.Fatalf("WriteFile pinned image: %v", err)
	}

	if _, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "text",
		Content:   "delete text",
		Preview:   "delete text",
		Timestamp: 1000,
	}); err != nil {
		t.Fatalf("SaveOrUpdate unpinned text: %v", err)
	}
	if _, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "image",
		Content:   deletePath,
		Preview:   "[image]",
		Timestamp: 2000,
	}); err != nil {
		t.Fatalf("SaveOrUpdate unpinned image: %v", err)
	}
	pinnedID, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "image",
		Content:   pinnedPath,
		Preview:   "[image]",
		Timestamp: 3000,
	})
	if err != nil {
		t.Fatalf("SaveOrUpdate pinned image: %v", err)
	}
	if err := store.TogglePin(pinnedID); err != nil {
		t.Fatalf("TogglePin() error = %v", err)
	}

	result, err := store.Clear()
	if err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if result.DeletedClips != 2 {
		t.Fatalf("Clear() DeletedClips = %d, want 2", result.DeletedClips)
	}
	if result.DeletedImages != 1 {
		t.Fatalf("Clear() DeletedImages = %d, want 1", result.DeletedImages)
	}
	if result.PinnedKept != 1 {
		t.Fatalf("Clear() PinnedKept = %d, want 1", result.PinnedKept)
	}
	if _, err := os.Stat(deletePath); !os.IsNotExist(err) {
		t.Fatalf("deleted image file still exists, stat error = %v", err)
	}
	if _, err := os.Stat(pinnedPath); err != nil {
		t.Fatalf("pinned image file was removed: %v", err)
	}
	if result.Stats.TotalClips != 1 || result.Stats.PinnedClips != 1 || result.Stats.UnpinnedClips != 0 {
		t.Fatalf("Clear() Stats = %+v, want only pinned image", result.Stats)
	}
}

func TestSaveOrUpdateDeduplicatesImageFiles(t *testing.T) {
	store := newTestStore(t)
	imageDir := filepath.Join(store.dataDir, "images")
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	firstPath := filepath.Join(imageDir, "first.dib")
	secondPath := filepath.Join(imageDir, "second.dib")
	data := []byte("same image bytes")
	if err := os.WriteFile(firstPath, data, 0644); err != nil {
		t.Fatalf("WriteFile first: %v", err)
	}
	if err := os.WriteFile(secondPath, data, 0644); err != nil {
		t.Fatalf("WriteFile second: %v", err)
	}

	firstID, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "image",
		Content:   firstPath,
		Preview:   "[图片]",
		Thumbnail: "thumb",
		Timestamp: 1000,
	})
	if err != nil {
		t.Fatalf("SaveOrUpdate first image: %v", err)
	}
	secondID, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "image",
		Content:   secondPath,
		Preview:   "[图片]",
		Thumbnail: "thumb",
		Timestamp: 2000,
	})
	if err != nil {
		t.Fatalf("SaveOrUpdate duplicate image: %v", err)
	}
	if firstID != secondID {
		t.Fatalf("duplicate image got id %d, want %d", secondID, firstID)
	}
	if _, err := os.Stat(secondPath); !os.IsNotExist(err) {
		t.Fatalf("duplicate file still exists, stat error = %v", err)
	}
}

func TestSaveOrUpdateStoresAndRefreshesSourceInfo(t *testing.T) {
	store := newTestStore(t)

	firstID, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:        "text",
		Content:     "same text",
		Preview:     "same text",
		Timestamp:   1000,
		SourceApp:   "notepad.exe",
		SourceTitle: "Notes",
		SourcePath:  `C:\Windows\System32\notepad.exe`,
	})
	if err != nil {
		t.Fatalf("SaveOrUpdate first: %v", err)
	}
	clip, err := store.GetByID(firstID)
	if err != nil {
		t.Fatalf("GetByID first: %v", err)
	}
	if clip.SourceApp != "notepad.exe" || clip.SourceTitle != "Notes" || clip.SourcePath != `C:\Windows\System32\notepad.exe` {
		t.Fatalf("source info = %+v", clip)
	}

	secondID, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:        "text",
		Content:     "same text",
		Preview:     "same text",
		Timestamp:   2000,
		SourceApp:   "Code.exe",
		SourceTitle: "main.go - ClipBox",
		SourcePath:  `C:\Users\QGS\AppData\Local\Programs\Microsoft VS Code\Code.exe`,
	})
	if err != nil {
		t.Fatalf("SaveOrUpdate second: %v", err)
	}
	if secondID != firstID {
		t.Fatalf("duplicate text id = %d, want %d", secondID, firstID)
	}
	clip, err = store.GetByID(firstID)
	if err != nil {
		t.Fatalf("GetByID second: %v", err)
	}
	if clip.SourceApp != "Code.exe" || clip.SourceTitle != "main.go - ClipBox" {
		t.Fatalf("refreshed source info = %+v", clip)
	}

	if _, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "text",
		Content:   "same text",
		Preview:   "same text",
		Timestamp: 3000,
	}); err != nil {
		t.Fatalf("SaveOrUpdate without source: %v", err)
	}
	clip, err = store.GetByID(firstID)
	if err != nil {
		t.Fatalf("GetByID without source: %v", err)
	}
	if clip.SourceApp != "Code.exe" || clip.SourceTitle != "main.go - ClipBox" {
		t.Fatalf("source info after empty update = %+v, want preserved", clip)
	}

	clips, err := store.GetAll(10, 0)
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(clips) != 1 || clips[0].SourceApp != "Code.exe" {
		t.Fatalf("GetAll source info = %+v", clips)
	}
}

func TestCleanupRespectsMaxClipsAndPinnedClips(t *testing.T) {
	store := newTestStore(t)

	oldID, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "text",
		Content:   "old",
		Preview:   "old",
		Timestamp: 1000,
	})
	if err != nil {
		t.Fatalf("SaveOrUpdate old: %v", err)
	}
	pinnedID, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "text",
		Content:   "pinned",
		Preview:   "pinned",
		Timestamp: 2000,
	})
	if err != nil {
		t.Fatalf("SaveOrUpdate pinned: %v", err)
	}
	newID, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "text",
		Content:   "new",
		Preview:   "new",
		Timestamp: 3000,
	})
	if err != nil {
		t.Fatalf("SaveOrUpdate new: %v", err)
	}
	if err := store.TogglePin(pinnedID); err != nil {
		t.Fatalf("TogglePin() error = %v", err)
	}

	if err := store.Cleanup(CleanupPolicy{MaxClips: 2}); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	if clip, err := store.GetByID(oldID); err != nil || clip != nil {
		t.Fatalf("old clip = %+v, err = %v, want deleted", clip, err)
	}
	if clip, err := store.GetByID(pinnedID); err != nil || clip == nil || !clip.Pinned {
		t.Fatalf("pinned clip = %+v, err = %v, want kept pinned", clip, err)
	}
	if clip, err := store.GetByID(newID); err != nil || clip == nil {
		t.Fatalf("new clip = %+v, err = %v, want kept", clip, err)
	}
}

func TestStatsCountsClipsAndStorage(t *testing.T) {
	store := newTestStore(t)
	imageDir := filepath.Join(store.dataDir, "images")
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	imagePath := filepath.Join(imageDir, "image.dib")
	imageBytes := []byte("image bytes")
	if err := os.WriteFile(imagePath, imageBytes, 0644); err != nil {
		t.Fatalf("WriteFile image: %v", err)
	}

	textID, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "text",
		Content:   "hello",
		Preview:   "hello",
		Timestamp: 1000,
	})
	if err != nil {
		t.Fatalf("SaveOrUpdate text: %v", err)
	}
	if _, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "image",
		Content:   imagePath,
		Preview:   "[图片]",
		Thumbnail: "thumbnail-data",
		Timestamp: 2000,
	}); err != nil {
		t.Fatalf("SaveOrUpdate image: %v", err)
	}
	if err := store.TogglePin(textID); err != nil {
		t.Fatalf("TogglePin() error = %v", err)
	}

	stats, err := store.Stats()
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if stats.TotalClips != 2 || stats.TextClips != 1 || stats.ImageClips != 1 || stats.PinnedClips != 1 {
		t.Fatalf("Stats() counts = %+v, want 2 total, 1 text, 1 image, 1 pinned", stats)
	}
	if stats.ImageBytes != int64(len(imageBytes)) {
		t.Fatalf("Stats().ImageBytes = %d, want %d", stats.ImageBytes, len(imageBytes))
	}
	if stats.ThumbnailBytes != int64(len("thumbnail-data")) {
		t.Fatalf("Stats().ThumbnailBytes = %d", stats.ThumbnailBytes)
	}
	if stats.DatabaseBytes <= 0 {
		t.Fatalf("Stats().DatabaseBytes = %d, want > 0", stats.DatabaseBytes)
	}
}

func TestMaintainRemovesMissingImagesAndKeepsValidClips(t *testing.T) {
	store := newTestStore(t)
	imageDir := filepath.Join(store.dataDir, "images")
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		t.Fatalf("MkdirAll image dir: %v", err)
	}
	validImage := filepath.Join(imageDir, "valid.dib")
	if err := os.WriteFile(validImage, []byte("valid image"), 0644); err != nil {
		t.Fatalf("WriteFile valid image: %v", err)
	}

	if _, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "text",
		Content:   "keep text",
		Preview:   "keep text",
		Timestamp: 1000,
	}); err != nil {
		t.Fatalf("SaveOrUpdate text: %v", err)
	}
	if _, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "image",
		Content:   validImage,
		Preview:   "[图片]",
		Timestamp: 2000,
	}); err != nil {
		t.Fatalf("SaveOrUpdate valid image: %v", err)
	}
	if _, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "image",
		Content:   filepath.Join(imageDir, "missing.dib"),
		Preview:   "[图片]",
		Timestamp: 3000,
	}); err != nil {
		t.Fatalf("SaveOrUpdate missing image: %v", err)
	}

	before, err := store.Stats()
	if err != nil {
		t.Fatalf("Stats before: %v", err)
	}
	if before.MissingImages != 1 {
		t.Fatalf("MissingImages before = %d, want 1", before.MissingImages)
	}

	result, err := store.Maintain()
	if err != nil {
		t.Fatalf("Maintain() error = %v", err)
	}
	if result.MissingImagesRemoved != 1 || !result.Optimized || !result.Vacuumed {
		t.Fatalf("Maintain() result = %+v, want one removed and optimized/vacuumed", result)
	}
	if result.Stats.MissingImages != 0 || result.Stats.TotalClips != 2 || result.Stats.ImageClips != 1 {
		t.Fatalf("stats after maintenance = %+v, want no missing, 2 total, 1 image", result.Stats)
	}
}

func TestSearchSupportsPaginationAndCount(t *testing.T) {
	store := newTestStore(t)

	for i := 0; i < 5; i++ {
		if _, err := store.SaveOrUpdate(clipboard.ClipEntry{
			Type:      "text",
			Content:   "match item " + string(rune('a'+i)),
			Preview:   "match",
			Timestamp: int64(1000 + i),
		}); err != nil {
			t.Fatalf("SaveOrUpdate match %d: %v", i, err)
		}
	}
	if _, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "text",
		Content:   "other item",
		Preview:   "other",
		Timestamp: 2000,
	}); err != nil {
		t.Fatalf("SaveOrUpdate other: %v", err)
	}

	count, err := store.CountSearch("match")
	if err != nil {
		t.Fatalf("CountSearch() error = %v", err)
	}
	if count != 5 {
		t.Fatalf("CountSearch() = %d, want 5", count)
	}

	firstPage, err := store.Search("match", 2, 0)
	if err != nil {
		t.Fatalf("Search first page: %v", err)
	}
	secondPage, err := store.Search("match", 2, 2)
	if err != nil {
		t.Fatalf("Search second page: %v", err)
	}
	if len(firstPage) != 2 || len(secondPage) != 2 {
		t.Fatalf("page lengths = %d/%d, want 2/2", len(firstPage), len(secondPage))
	}
	if firstPage[0].ID == secondPage[0].ID {
		t.Fatalf("pagination returned duplicate first ids: %d", firstPage[0].ID)
	}
}

func TestSearchUsesLiteralWildcardsAndConsistentCounts(t *testing.T) {
	store := newTestStore(t)
	clips := []clipboard.ClipEntry{
		{Type: "text", Content: "50% done", Preview: "50% done", Timestamp: 1000, Tags: "percent"},
		{Type: "text", Content: "100 percent", Preview: "100 percent", Timestamp: 2000, Tags: "ordinary"},
		{Type: "text", Content: "under_score", Preview: "under_score", Timestamp: 3000, Tags: "underscore"},
		{Type: "text", Content: "under score", Preview: "under score", Timestamp: 4000, Tags: "ordinary"},
	}
	for _, clip := range clips {
		if _, err := store.SaveOrUpdate(clip); err != nil {
			t.Fatalf("SaveOrUpdate(%q): %v", clip.Content, err)
		}
	}

	for _, tc := range []struct {
		query string
		want  string
	}{
		{query: "%", want: "50% done"},
		{query: "_", want: "under_score"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			items, err := store.SearchByType(tc.query, "all", 10, 0)
			if err != nil {
				t.Fatalf("SearchByType(%q): %v", tc.query, err)
			}
			if len(items) != 1 || items[0].Content != tc.want {
				t.Fatalf("SearchByType(%q) = %+v, want only %q", tc.query, items, tc.want)
			}

			count, err := store.CountSearchByType(tc.query, "all")
			if err != nil {
				t.Fatalf("CountSearchByType(%q): %v", tc.query, err)
			}
			if count != len(items) {
				t.Fatalf("CountSearchByType(%q) = %d, page length = %d", tc.query, count, len(items))
			}

			tags, err := store.ListTags(tc.query, "all")
			if err != nil {
				t.Fatalf("ListTags(%q): %v", tc.query, err)
			}
			if len(tags) != 1 {
				t.Fatalf("ListTags(%q) = %+v, want only tags on literal matches", tc.query, tags)
			}
			wantTag := "percent"
			if tc.query == "_" {
				wantTag = "underscore"
			}
			if tags[0].Name != wantTag || tags[0].Count != 1 {
				t.Fatalf("ListTags(%q) = %+v, want %s count 1", tc.query, tags, wantTag)
			}
		})
	}
}

func TestChineseSubstringSearchAndCounts(t *testing.T) {
	store := newTestStore(t)
	for i, content := range []string{"微信聊天记录", "微信支付凭证", "周会记录"} {
		if _, err := store.SaveOrUpdate(clipboard.ClipEntry{
			Type:      "text",
			Content:   content,
			Preview:   content,
			Timestamp: int64(1000 + i),
		}); err != nil {
			t.Fatalf("SaveOrUpdate(%q): %v", content, err)
		}
	}
	for _, tc := range []struct {
		query string
		want  int
	}{
		{query: "微", want: 2},
		{query: "微信", want: 2},
		{query: "微信聊", want: 1},
	} {
		items, err := store.SearchByType(tc.query, "all", 1, 0)
		if err != nil {
			t.Fatalf("SearchByType(%q): %v", tc.query, err)
		}
		count, err := store.CountSearchByType(tc.query, "all")
		if err != nil {
			t.Fatalf("CountSearchByType(%q): %v", tc.query, err)
		}
		if len(items) != 1 || count != tc.want {
			t.Fatalf("query %q: page length=%d count=%d, want page length 1 count %d", tc.query, len(items), count, tc.want)
		}
		if count2, err := store.CountSearchByTypeAndTag(tc.query, "all", ""); err != nil || count2 != count {
			t.Fatalf("CountSearchByTypeAndTag(%q, empty tag) = %d, %v; want %d", tc.query, count2, err, count)
		}
	}
}

func TestSearchIndexRebuildsAndTracksClipChanges(t *testing.T) {
	store := newTestStore(t)
	if !store.ftsAvailable {
		t.Fatal("FTS search index is not available")
	}

	id, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "text",
		Content:   "release package alpha",
		Preview:   "release package alpha",
		Timestamp: 1000,
	})
	if err != nil {
		t.Fatalf("SaveOrUpdate text: %v", err)
	}
	if count := countSearchIndexRows(t, store); count != 1 {
		t.Fatalf("FTS row count after insert = %d, want 1", count)
	}

	results, err := store.SearchByType("package", "all", 50, 0)
	if err != nil {
		t.Fatalf("SearchByType package: %v", err)
	}
	if len(results) != 1 || results[0].ID != id {
		t.Fatalf("FTS search results = %+v, want clip %d", results, id)
	}

	if _, err := store.UpdateMetadata(id, "signed installer", "release"); err != nil {
		t.Fatalf("UpdateMetadata: %v", err)
	}
	results, err = store.SearchByType("signed", "all", 50, 0)
	if err != nil {
		t.Fatalf("SearchByType signed: %v", err)
	}
	if len(results) != 1 || results[0].ID != id {
		t.Fatalf("FTS metadata results = %+v, want clip %d", results, id)
	}

	if err := store.Delete(id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if count := countSearchIndexRows(t, store); count != 0 {
		t.Fatalf("FTS row count after delete = %d, want 0", count)
	}
}

func TestMaintainRebuildsSearchIndex(t *testing.T) {
	store := newTestStore(t)
	if !store.ftsAvailable {
		t.Fatal("FTS search index is not available")
	}
	if _, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "text",
		Content:   "maintenance searchable text",
		Preview:   "maintenance searchable text",
		Timestamp: 1000,
	}); err != nil {
		t.Fatalf("SaveOrUpdate text: %v", err)
	}
	if _, err := store.db.Exec(`DELETE FROM clips_fts`); err != nil {
		t.Fatalf("clear clips_fts: %v", err)
	}
	if count := countSearchIndexRows(t, store); count != 0 {
		t.Fatalf("FTS row count after manual clear = %d, want 0", count)
	}

	if _, err := store.Maintain(); err != nil {
		t.Fatalf("Maintain: %v", err)
	}
	if count := countSearchIndexRows(t, store); count != 1 {
		t.Fatalf("FTS row count after Maintain = %d, want 1", count)
	}
	results, err := store.SearchByType("searchable", "text", 50, 0)
	if err != nil {
		t.Fatalf("SearchByType searchable: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("search results after Maintain = %+v, want one clip", results)
	}
}

func TestGetByTypePaginatesFilteredClips(t *testing.T) {
	store := newTestStore(t)
	imageDir := filepath.Join(store.dataDir, "images")
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		t.Fatalf("MkdirAll image dir: %v", err)
	}

	for i := 0; i < 3; i++ {
		if _, err := store.SaveOrUpdate(clipboard.ClipEntry{
			Type:      "text",
			Content:   "text item " + string(rune('a'+i)),
			Preview:   "text",
			Timestamp: int64(1000 + i),
		}); err != nil {
			t.Fatalf("SaveOrUpdate text %d: %v", i, err)
		}
	}
	for i := 0; i < 2; i++ {
		path := filepath.Join(imageDir, "image-"+string(rune('a'+i))+".dib")
		if err := os.WriteFile(path, []byte{byte(i + 1)}, 0644); err != nil {
			t.Fatalf("WriteFile image %d: %v", i, err)
		}
		if _, err := store.SaveOrUpdate(clipboard.ClipEntry{
			Type:      "image",
			Content:   path,
			Preview:   "[图片]",
			Timestamp: int64(2000 + i),
		}); err != nil {
			t.Fatalf("SaveOrUpdate image %d: %v", i, err)
		}
	}

	imageCount, err := store.CountByType("image")
	if err != nil {
		t.Fatalf("CountByType image: %v", err)
	}
	if imageCount != 2 {
		t.Fatalf("image count = %d, want 2", imageCount)
	}
	images, err := store.GetByType("image", 1, 0)
	if err != nil {
		t.Fatalf("GetByType image: %v", err)
	}
	if len(images) != 1 || images[0].Type != "image" || images[0].Content != "" {
		t.Fatalf("images page = %+v, want one sanitized image", images)
	}
	textCount, err := store.CountByType("text")
	if err != nil {
		t.Fatalf("CountByType text: %v", err)
	}
	if textCount != 3 {
		t.Fatalf("text count = %d, want 3", textCount)
	}
}

func countSearchIndexRows(t *testing.T, store *Store) int {
	t.Helper()
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM clips_fts`).Scan(&count); err != nil {
		t.Fatalf("count clips_fts: %v", err)
	}
	return count
}

func TestSearchByTypeReturnsEmptyForImages(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "text",
		Content:   "needle",
		Preview:   "needle",
		Timestamp: 1000,
	}); err != nil {
		t.Fatalf("SaveOrUpdate text: %v", err)
	}

	count, err := store.CountSearchByType("needle", "image")
	if err != nil {
		t.Fatalf("CountSearchByType image: %v", err)
	}
	if count != 0 {
		t.Fatalf("image search count = %d, want 0", count)
	}
	results, err := store.SearchByType("needle", "image", 50, 0)
	if err != nil {
		t.Fatalf("SearchByType image: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("image search results = %+v, want empty", results)
	}
}

func TestSearchByTypeMatchesSourceInfoForImages(t *testing.T) {
	store := newTestStore(t)
	imageDir := filepath.Join(store.dataDir, "images")
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		t.Fatalf("MkdirAll image dir: %v", err)
	}
	imagePath := filepath.Join(imageDir, "screenshot.dib")
	if err := os.WriteFile(imagePath, []byte("screenshot bytes"), 0644); err != nil {
		t.Fatalf("WriteFile image: %v", err)
	}

	if _, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:        "image",
		Content:     imagePath,
		Preview:     "[图片]",
		Timestamp:   1000,
		SourceApp:   "SnippingTool.exe",
		SourceTitle: "截图工具",
	}); err != nil {
		t.Fatalf("SaveOrUpdate image: %v", err)
	}
	if _, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:        "text",
		Content:     "plain text",
		Preview:     "plain text",
		Timestamp:   2000,
		SourceApp:   "notepad.exe",
		SourceTitle: "Notes",
	}); err != nil {
		t.Fatalf("SaveOrUpdate text: %v", err)
	}

	count, err := store.CountSearchByType("snipping", "image")
	if err != nil {
		t.Fatalf("CountSearchByType image source: %v", err)
	}
	if count != 1 {
		t.Fatalf("image source count = %d, want 1", count)
	}
	results, err := store.SearchByType("截图", "all", 50, 0)
	if err != nil {
		t.Fatalf("SearchByType all source title: %v", err)
	}
	if len(results) != 1 || results[0].Type != "image" || results[0].Content != "" || results[0].SourceApp != "SnippingTool.exe" {
		t.Fatalf("source search results = %+v, want sanitized image from SnippingTool", results)
	}
}

func TestUpdateMetadataNormalizesAndSearchesNoteAndTags(t *testing.T) {
	store := newTestStore(t)
	id, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "text",
		Content:   "plain clipboard text",
		Preview:   "plain clipboard text",
		Timestamp: 1000,
	})
	if err != nil {
		t.Fatalf("SaveOrUpdate text: %v", err)
	}

	updated, err := store.UpdateMetadata(id, "  invoice follow-up  ", "work, urgent，work\nfinance")
	if err != nil {
		t.Fatalf("UpdateMetadata: %v", err)
	}
	if updated == nil {
		t.Fatal("UpdateMetadata returned nil, want updated clip")
	}
	if updated.Note != "invoice follow-up" {
		t.Fatalf("updated note = %q, want normalized note", updated.Note)
	}
	if updated.Tags != "work, urgent, finance" {
		t.Fatalf("updated tags = %q, want normalized tags", updated.Tags)
	}

	count, err := store.CountSearchByType("finance", "text")
	if err != nil {
		t.Fatalf("CountSearchByType tag: %v", err)
	}
	if count != 1 {
		t.Fatalf("metadata search count = %d, want 1", count)
	}
	results, err := store.SearchByType("follow-up", "all", 50, 0)
	if err != nil {
		t.Fatalf("SearchByType note: %v", err)
	}
	if len(results) != 1 || results[0].ID != id || results[0].Note != "invoice follow-up" {
		t.Fatalf("metadata search results = %+v, want clip with note", results)
	}
}

func TestListTagsCountsAndFiltersByTypeAndSearch(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "text",
		Content:   "alpha note",
		Preview:   "alpha note",
		Timestamp: 1000,
		Tags:      "work, alpha",
	}); err != nil {
		t.Fatalf("SaveOrUpdate alpha text: %v", err)
	}
	if _, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "text",
		Content:   "beta note",
		Preview:   "beta note",
		Timestamp: 2000,
		Tags:      "work, beta",
	}); err != nil {
		t.Fatalf("SaveOrUpdate beta text: %v", err)
	}
	if _, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "image",
		Content:   filepath.Join(store.dataDir, "missing.dib"),
		Preview:   "[图片]",
		Timestamp: 3000,
		Tags:      "visual, work",
	}); err != nil {
		t.Fatalf("SaveOrUpdate image: %v", err)
	}

	tags, err := store.ListTags("", "all")
	if err != nil {
		t.Fatalf("ListTags all: %v", err)
	}
	if len(tags) < 4 || tags[0].Name != "work" || tags[0].Count != 3 {
		t.Fatalf("tags all = %+v, want work count first", tags)
	}
	textTags, err := store.ListTags("", "text")
	if err != nil {
		t.Fatalf("ListTags text: %v", err)
	}
	if len(textTags) == 0 || textTags[0].Name != "work" || textTags[0].Count != 2 {
		t.Fatalf("text tags = %+v, want work count 2", textTags)
	}
	searchTags, err := store.ListTags("alpha", "all")
	if err != nil {
		t.Fatalf("ListTags search: %v", err)
	}
	if len(searchTags) != 2 || searchTags[0].Name != "alpha" && searchTags[1].Name != "alpha" {
		t.Fatalf("search tags = %+v, want alpha and work", searchTags)
	}
}

func TestGetByTypeAndTagUsesExactTagMatch(t *testing.T) {
	store := newTestStore(t)
	workID, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "text",
		Content:   "work item",
		Preview:   "work item",
		Timestamp: 1000,
		Tags:      "work",
	})
	if err != nil {
		t.Fatalf("SaveOrUpdate work: %v", err)
	}
	if _, err := store.SaveOrUpdate(clipboard.ClipEntry{
		Type:      "text",
		Content:   "workshop item",
		Preview:   "workshop item",
		Timestamp: 2000,
		Tags:      "workshop",
	}); err != nil {
		t.Fatalf("SaveOrUpdate workshop: %v", err)
	}

	count, err := store.CountByTypeAndTag("text", "work")
	if err != nil {
		t.Fatalf("CountByTypeAndTag: %v", err)
	}
	if count != 1 {
		t.Fatalf("tag count = %d, want 1", count)
	}
	results, err := store.GetByTypeAndTag("text", "work", 50, 0)
	if err != nil {
		t.Fatalf("GetByTypeAndTag: %v", err)
	}
	if len(results) != 1 || results[0].ID != workID {
		t.Fatalf("tag results = %+v, want only work clip %d", results, workID)
	}
	searchResults, err := store.SearchByTypeAndTag("item", "all", "work", 50, 0)
	if err != nil {
		t.Fatalf("SearchByTypeAndTag: %v", err)
	}
	if len(searchResults) != 1 || searchResults[0].ID != workID {
		t.Fatalf("tag search results = %+v, want only work clip %d", searchResults, workID)
	}
}

func TestBackupRoundTripIncludesSettingsTextAndImages(t *testing.T) {
	source := newTestStore(t)
	imageDir := filepath.Join(source.dataDir, "images")
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		t.Fatalf("MkdirAll image dir: %v", err)
	}
	imagePath := filepath.Join(imageDir, "image.dib")
	imageBytes := []byte("backup image bytes")
	if err := os.WriteFile(imagePath, imageBytes, 0644); err != nil {
		t.Fatalf("WriteFile image: %v", err)
	}
	if err := source.SetSetting("auto_paste", "false"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	textID, err := source.SaveOrUpdate(clipboard.ClipEntry{
		Type:        "text",
		Content:     "backup text",
		Preview:     "backup text",
		Timestamp:   1000,
		SourceApp:   "notepad.exe",
		SourceTitle: "Backup Notes",
		SourcePath:  `C:\Windows\System32\notepad.exe`,
		Note:        "remember this text",
		Tags:        "docs, important",
	})
	if err != nil {
		t.Fatalf("SaveOrUpdate text: %v", err)
	}
	if err := source.TogglePin(textID); err != nil {
		t.Fatalf("TogglePin text: %v", err)
	}
	if _, err := source.SaveOrUpdate(clipboard.ClipEntry{
		Type:        "image",
		Content:     imagePath,
		Preview:     "[图片]",
		Thumbnail:   "thumb",
		Timestamp:   2000,
		SourceApp:   "SnippingTool.exe",
		SourceTitle: "截图工具",
		SourcePath:  `C:\Windows\System32\SnippingTool.exe`,
		Note:        "image note",
		Tags:        "visual, review",
	}); err != nil {
		t.Fatalf("SaveOrUpdate image: %v", err)
	}

	backupPath := filepath.Join(t.TempDir(), "clipbox.clipbox-backup")
	exportStats, err := source.ExportBackupFile(backupPath, "test")
	if err != nil {
		t.Fatalf("ExportBackupFile: %v", err)
	}
	if exportStats.Clips != 2 || exportStats.Images != 1 || exportStats.Settings != 1 {
		t.Fatalf("export stats = %+v, want 2 clips, 1 image, 1 setting", exportStats)
	}

	target := newTestStore(t)
	importStats, err := target.ImportBackupFile(backupPath)
	if err != nil {
		t.Fatalf("ImportBackupFile: %v", err)
	}
	if importStats.Clips != 2 || importStats.Images != 1 || importStats.Settings != 1 {
		t.Fatalf("import stats = %+v, want 2 clips, 1 image, 1 setting", importStats)
	}
	value, err := target.GetSetting("auto_paste")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if value != "false" {
		t.Fatalf("auto_paste = %q, want false", value)
	}
	clips, err := target.GetAll(10, 0)
	if err != nil {
		t.Fatalf("GetAll target: %v", err)
	}
	if len(clips) != 2 {
		t.Fatalf("imported clips length = %d, want 2", len(clips))
	}
	if !clips[0].Pinned && !clips[1].Pinned {
		t.Fatalf("imported clips = %+v, want one pinned", clips)
	}
	var textClip *clipboard.ClipEntry
	var imageClip *clipboard.ClipEntry
	for i := range clips {
		if clips[i].Type == "text" {
			textClip = &clips[i]
		}
		if clips[i].Type == "image" {
			imageClip = &clips[i]
		}
	}
	if textClip == nil || textClip.SourceApp != "notepad.exe" || textClip.SourceTitle != "Backup Notes" {
		t.Fatalf("imported text source = %+v", textClip)
	}
	if textClip.Note != "remember this text" || textClip.Tags != "docs, important" {
		t.Fatalf("imported text metadata = %+v", textClip)
	}
	if imageClip == nil || imageClip.SourceApp != "SnippingTool.exe" || imageClip.SourceTitle != "截图工具" {
		t.Fatalf("imported image source = %+v", imageClip)
	}
	if imageClip.Note != "image note" || imageClip.Tags != "visual, review" {
		t.Fatalf("imported image metadata = %+v", imageClip)
	}

	stats, err := target.Stats()
	if err != nil {
		t.Fatalf("Stats target: %v", err)
	}
	if stats.ImageBytes != int64(len(imageBytes)) {
		t.Fatalf("imported image bytes = %d, want %d", stats.ImageBytes, len(imageBytes))
	}
}

func TestImportBackupRejectsTooManyClips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "too-many.clipbox-backup")
	clips := make([]backupClip, maxBackupClips+1)
	for i := range clips {
		clips[i] = backupClip{
			Type:      "text",
			Content:   "item",
			Preview:   "item",
			Timestamp: int64(i + 1),
		}
	}
	writeTestBackup(t, path, clips, nil, nil)

	store := newTestStore(t)
	if _, err := store.ImportBackupFile(path); err == nil || !strings.Contains(err.Error(), "too many clips") {
		t.Fatalf("ImportBackupFile err = %v, want too many clips", err)
	}
}

func TestRestoreBackupImageRejectsLimitOverflow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image-limit.clipbox-backup")
	writeTestBackup(t, path, nil, nil, map[string][]byte{
		"large.dib": []byte("12345"),
	})
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer reader.Close()

	var imageFile *zip.File
	for _, file := range reader.File {
		if file.Name == "images/large.dib" {
			imageFile = file
			break
		}
	}
	if imageFile == nil {
		t.Fatal("image file not found in test backup")
	}
	if _, _, err := restoreBackupImage(imageFile, t.TempDir(), 0, 4); err == nil {
		t.Fatal("restoreBackupImage succeeded, want limit error")
	}
}

func TestImportBackupSkipsUnsafeImageReference(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unsafe-image.clipbox-backup")
	writeTestBackup(t, path, []backupClip{{
		Type:      "image",
		Preview:   "[图片]",
		ImageFile: "../unsafe.dib",
		Timestamp: 1000,
	}}, nil, map[string][]byte{
		"safe.dib": []byte("image"),
	})

	store := newTestStore(t)
	stats, err := store.ImportBackupFile(path)
	if err != nil {
		t.Fatalf("ImportBackupFile: %v", err)
	}
	if stats.SkippedImages != 1 || stats.Clips != 0 || stats.Images != 0 {
		t.Fatalf("import stats = %+v, want unsafe image skipped", stats)
	}
	count, err := store.CountAll()
	if err != nil {
		t.Fatalf("CountAll: %v", err)
	}
	if count != 0 {
		t.Fatalf("imported count = %d, want 0", count)
	}
}
