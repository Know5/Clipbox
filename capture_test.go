package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"clipbox/internal/clipboard"
)

func TestCaptureEventPreservesPersistedMetadata(t *testing.T) {
	app := newTestAppWithStore(t)
	entry := clipboard.ClipEntry{Type: "text", Content: "saved text", Preview: "saved text", Timestamp: 1, Pinned: true, Note: "saved note", Tags: "Work"}
	id, err := app.store.SaveOrUpdate(entry)
	if err != nil {
		t.Fatal(err)
	}
	change, err := app.persistCapturedClip(clipboard.ClipEntry{Type: "text", Content: entry.Content, Preview: entry.Preview, Timestamp: 2})
	if err != nil {
		t.Fatal(err)
	}
	if change.Entry == nil || change.Entry.ID != id || !change.Entry.Pinned || change.Entry.Note != entry.Note || change.Entry.Tags != entry.Tags || change.Total != 1 {
		t.Fatalf("capture event did not reflect the persisted record: %+v entry=%+v", change, change.Entry)
	}
}

func TestCaptureEventIncludesCleanupDeletionsAndActualTotal(t *testing.T) {
	for _, pinned := range []bool{false, true} {
		t.Run(fmt.Sprintf("pinned=%v", pinned), func(t *testing.T) {
			app := newTestAppWithStore(t)
			settings := defaultAppSettings()
			settings.MaxClips = 50
			if err := app.saveAppSettings(settings); err != nil {
				t.Fatal(err)
			}
			var oldest int64
			for index := 0; index < 50; index++ {
				id, err := app.store.SaveOrUpdate(clipboard.ClipEntry{Type: "text", Content: fmt.Sprintf("old-%d", index), Preview: "old", Timestamp: int64(index + 1), Pinned: pinned})
				if err != nil {
					t.Fatal(err)
				}
				if index == 0 {
					oldest = id
				}
			}
			change, err := app.persistCapturedClip(clipboard.ClipEntry{Type: "text", Content: "new entry", Preview: "new entry", Timestamp: 100})
			if err != nil {
				t.Fatal(err)
			}
			if change.Total != 50 || len(change.DeletedIDs) != 1 {
				t.Fatalf("cleanup change = %+v, want one deletion and total 50", change)
			}
			if pinned {
				if change.Entry != nil {
					t.Fatal("must not emit an entry immediately removed by the pinned capacity limit")
				}
			} else if change.Entry == nil || change.DeletedIDs[0] != oldest {
				t.Fatalf("oldest deletion or final new entry missing: %+v", change)
			}
		})
	}
}

func TestCaptureEventKeepsImagePathPrivate(t *testing.T) {
	app := newTestAppWithStore(t)
	path := filepath.Join(app.dataDir, "images", "captured.dib")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, minimalDib(), 0644); err != nil {
		t.Fatal(err)
	}
	change, err := app.persistCapturedClip(clipboard.ClipEntry{Type: "image", Content: path, Preview: "image", Timestamp: 1})
	if err != nil || change.Entry == nil || change.Entry.Content != "" {
		t.Fatalf("image capture event = %+v, err=%v", change, err)
	}
}

func TestFailedCaptureReclaimsUnreferencedImage(t *testing.T) {
	app := newTestAppWithStore(t)
	db, err := sql.Open("sqlite", filepath.Join(app.dataDir, "clipbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER fail_capture BEFORE INSERT ON clips BEGIN SELECT RAISE(ABORT, 'write failure'); END`); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(app.dataDir, "images", "failed.dib")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, minimalDib(), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.persistCapturedClip(clipboard.ClipEntry{Type: "image", Content: path, Preview: "image", Timestamp: 1}); err == nil {
		t.Fatal("expected the injected write error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("failed capture left an unreferenced image: %v", err)
	}
}
