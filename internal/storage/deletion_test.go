package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"clipbox/internal/clipboard"
)

func saveDeletionTestImage(t *testing.T, store *Store, name string, timestamp int64) (int64, string) {
	t.Helper()
	path := filepath.Join(store.dataDir, "images", name+".dib")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(name), 0644); err != nil {
		t.Fatal(err)
	}
	id, err := store.SaveOrUpdate(clipboard.ClipEntry{Type: "image", Content: path, Preview: name, Timestamp: timestamp})
	if err != nil {
		t.Fatal(err)
	}
	return id, path
}

func TestFailedDeletionPreservesRecordsAndImages(t *testing.T) {
	for _, operation := range []string{"delete", "cleanup", "clear", "queue"} {
		t.Run(operation, func(t *testing.T) {
			store := newTestStore(t)
			id, path := saveDeletionTestImage(t, store, "original", 1000)
			if _, err := store.SaveOrUpdate(clipboard.ClipEntry{Type: "text", Content: "newer", Preview: "newer", Timestamp: 2000}); err != nil {
				t.Fatal(err)
			}
			trigger := `CREATE TRIGGER fail_delete BEFORE DELETE ON clips BEGIN SELECT RAISE(ABORT, 'write failure'); END`
			if operation == "queue" {
				trigger = `CREATE TRIGGER fail_queue BEFORE INSERT ON pending_image_deletes BEGIN SELECT RAISE(ABORT, 'queue write failure'); END`
			}
			if _, err := store.db.Exec(trigger); err != nil {
				t.Fatal(err)
			}
			var err error
			switch operation {
			case "cleanup":
				err = store.Cleanup(CleanupPolicy{MaxClips: 1})
			case "clear":
				_, err = store.Clear()
			default:
				err = store.Delete(id)
			}
			if err == nil {
				t.Fatal("expected the injected write failure")
			}
			entry, err := store.GetByID(id)
			if err != nil || entry == nil {
				t.Fatalf("original record was not preserved: entry=%+v err=%v", entry, err)
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("original image was not preserved: %v", err)
			}
		})
	}
}

func TestBatchDeletionRollsBackEarlierRecords(t *testing.T) {
	store := newTestStore(t)
	first, firstPath := saveDeletionTestImage(t, store, "first", 1)
	second, secondPath := saveDeletionTestImage(t, store, "second", 2)
	trigger := fmt.Sprintf(`CREATE TRIGGER fail_second BEFORE DELETE ON clips WHEN old.id = %d BEGIN SELECT RAISE(ABORT, 'second delete failed'); END`, second)
	if _, err := store.db.Exec(trigger); err != nil {
		t.Fatal(err)
	}
	if err := store.deleteIDs([]int64{first, second}); err == nil {
		t.Fatal("expected the second deletion to fail")
	}
	for index, id := range []int64{first, second} {
		entry, err := store.GetByID(id)
		if err != nil || entry == nil {
			t.Fatalf("record %d was not rolled back: %v", id, err)
		}
		if _, err := os.Stat([]string{firstPath, secondPath}[index]); err != nil {
			t.Fatal(err)
		}
	}
}

func TestImageReclamationRetriesAfterRestart(t *testing.T) {
	store := newTestStore(t)
	id, path := saveDeletionTestImage(t, store, "locked", 1)
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := store.Delete(id); err != nil {
		t.Fatal(err)
	}
	var pending int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM pending_image_deletes`).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 1 {
		t.Fatalf("locked image queue size = %d, want 1", pending)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(store.dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("image was not reclaimed on restart: %v", err)
	}
	if err := reopened.db.QueryRow(`SELECT COUNT(*) FROM pending_image_deletes`).Scan(&pending); err != nil || pending != 0 {
		t.Fatalf("pending queue after restart = %d, err=%v", pending, err)
	}
}

func TestDiscardDoesNotRemoveAReferencedImage(t *testing.T) {
	store := newTestStore(t)
	id, path := saveDeletionTestImage(t, store, "referenced", 1)
	store.DiscardEntryFiles(clipboard.ClipEntry{Type: "image", Content: path})
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("discard removed a referenced image: %v", err)
	}
	if err := store.Delete(id); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("image was not reclaimed after its last reference was deleted: %v", err)
	}
}
