package storage

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"clipbox/internal/clipboard"
)

func TestMaintenancePreservesLegacyAndRecaptureRepairsMissingImage(t *testing.T) {
	store := newTestStore(t)
	dib := make([]byte, 44)
	dib[0] = 40
	dib[4] = 1
	dib[8] = 1
	dib[12] = 1
	dib[14] = 24
	legacy := base64.StdEncoding.EncodeToString(clipboard.DibToBmp(dib))
	legacyID, err := store.SaveOrUpdate(clipboard.ClipEntry{Type: "image", Content: legacy, Preview: "legacy", Timestamp: 1, Pinned: true, Note: "keep"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Maintain(); err != nil {
		t.Fatal(err)
	}
	if entry, _ := store.GetByID(legacyID); entry == nil || entry.Note != "keep" {
		t.Fatal("maintenance deleted a legacy record")
	}
	dir := filepath.Join(store.dataDir, "images")
	_ = os.MkdirAll(dir, 0755)
	old := filepath.Join(dir, "old.dib")
	fresh := filepath.Join(dir, "new.dib")
	_ = os.WriteFile(old, dib, 0600)
	id, err := store.SaveOrUpdate(clipboard.ClipEntry{Type: "image", Content: old, Preview: "image", Timestamp: 2, Pinned: true})
	if err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(old)
	_ = os.WriteFile(fresh, dib, 0600)
	got, err := store.SaveOrUpdate(clipboard.ClipEntry{Type: "image", Content: fresh, Preview: "image", Timestamp: 3})
	if err != nil || got != id {
		t.Fatalf("recapture=%d,%v", got, err)
	}
	entry, _ := store.GetByID(id)
	if entry.Content != fresh || !entry.Pinned {
		t.Fatalf("repair did not preserve identity and pin: %+v", entry)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatal("repaired file was reclaimed")
	}
}

func TestUIReaderCannotWriteOrMigrate(t *testing.T) {
	store := newTestStore(t)
	reader, err := OpenReader(store.dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := reader.SetSetting("theme", "light"); err == nil {
		t.Fatal("read-only UI connection accepted a write")
	}
}
