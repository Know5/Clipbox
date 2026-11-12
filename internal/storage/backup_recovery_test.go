package storage

import (
	"os"
	"path/filepath"
	"testing"

	"clipbox/internal/clipboard"
)

func TestImportBackupRepairsMissingImageWithoutDuplicatingRecord(t *testing.T) {
	store := newTestStore(t)
	id, originalPath := saveDeletionTestImage(t, store, "restore-missing", 1)
	if err := store.TogglePin(id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateMetadata(id, "preserved note", "Work"); err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(t.TempDir(), "image.clipbox-backup")
	if _, err := store.ExportBackupFile(backupPath, "test"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(originalPath); err != nil {
		t.Fatal(err)
	}
	stats, err := store.ImportBackupFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Clips != 0 {
		t.Fatalf("restore duplicated an existing record: %+v", stats)
	}
	entry, err := store.GetByID(id)
	if err != nil || entry == nil {
		t.Fatalf("original record was not preserved: %+v, %v", entry, err)
	}
	if !entry.Pinned || entry.Note != "preserved note" || entry.Tags != "Work" {
		t.Fatalf("repair changed the record's metadata: %+v", entry)
	}
	data, err := os.ReadFile(entry.Content)
	if err != nil || string(data) != "restore-missing" {
		t.Fatalf("backup did not repair the image: %q, %v", data, err)
	}
}

func TestDuplicateImportedImageIsNotDeletedBeforeCommit(t *testing.T) {
	store := newTestStore(t)
	_, originalPath := saveDeletionTestImage(t, store, "original", 1)
	duplicatePath := filepath.Join(store.dataDir, "images", "duplicate.dib")
	if err := os.WriteFile(duplicatePath, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	tx, err := store.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := store.saveOrUpdateTx(tx, clipboard.ClipEntry{Type: "image", Content: duplicatePath, Timestamp: 2}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(duplicatePath); err != nil {
		t.Fatalf("duplicate image was removed before commit: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := store.RetryImageDeletes(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{originalPath, duplicatePath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("rollback did not preserve image %s: %v", path, err)
		}
	}
}

func TestDuplicateImportedImageReclamationSurvivesFileLock(t *testing.T) {
	store := newTestStore(t)
	_, originalPath := saveDeletionTestImage(t, store, "original", 1)
	duplicatePath := filepath.Join(store.dataDir, "images", "duplicate.dib")
	if err := os.WriteFile(duplicatePath, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	locked, err := os.Open(duplicatePath)
	if err != nil {
		t.Fatal(err)
	}
	defer locked.Close()
	tx, err := store.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	inserted, err := store.saveOrUpdateTx(tx, clipboard.ClipEntry{Type: "image", Content: duplicatePath, Timestamp: 2}, "")
	if err != nil || inserted {
		t.Fatalf("deduplication failed: inserted=%v, err=%v", inserted, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := locked.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.RetryImageDeletes(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(duplicatePath); !os.IsNotExist(err) {
		t.Fatalf("duplicate image was not reclaimed after unlock: %v", err)
	}
	if _, err := os.Stat(originalPath); err != nil {
		t.Fatalf("referenced original image was removed: %v", err)
	}
}
