package storage

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportRejectsOversizedPinnedHistoryWithoutReplacingBackup(t *testing.T) {
	store := newTestStore(t)
	tx, err := store.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	statement, err := tx.Prepare(`INSERT INTO clips(type, content, preview, timestamp, pinned, content_hash) VALUES ('text', ?, 'pinned', ?, 1, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index <= maxBackupClips; index++ {
		content := fmt.Sprintf("pinned-%d", index)
		if _, err := statement.Exec(content, index, hashContent(content)); err != nil {
			t.Fatal(err)
		}
	}
	statement.Close()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := store.Cleanup(CleanupPolicy{MaxClips: maxBackupClips}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "previous.clipbox-backup")
	previous := []byte("previous backup")
	if err := os.WriteFile(path, previous, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExportBackupFile(path, "test"); !errors.Is(err, errBackupLimit) {
		t.Fatalf("export error = %v, want a format limit error", err)
	}
	actual, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(actual, previous) {
		t.Fatalf("previous backup was changed: %q, err=%v", actual, err)
	}
}

func TestExportJSONLimitPreservesPreviousBackupAndRemovesTemporaryFile(t *testing.T) {
	store := newTestStore(t)
	path := filepath.Join(t.TempDir(), "previous.clipbox-backup")
	if err := os.WriteFile(path, []byte("previous"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExportBackupFile(path, strings.Repeat("version", maxBackupManifestBytes)); !errors.Is(err, errBackupLimit) {
		t.Fatalf("export error = %v, want JSON limit error", err)
	}
	actual, err := os.ReadFile(path)
	if err != nil || string(actual) != "previous" {
		t.Fatalf("previous backup was changed: %q, err=%v", actual, err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("failed export left a temporary file: %v", err)
	}
}

func TestBackupClipArrayEnforcesEncodedBytesIncludingEscapes(t *testing.T) {
	clips := []backupClip{{Type: "text", Content: "quotes \"\n<&> λ"}, {Type: "text", Content: "second"}}
	var complete bytes.Buffer
	if err := writeBackupClipArray(&complete, clips, 10000); err != nil {
		t.Fatal(err)
	}
	var decoded []backupClip
	if err := json.Unmarshal(complete.Bytes(), &decoded); err != nil || len(decoded) != 2 || decoded[0].Content != clips[0].Content {
		t.Fatalf("encoded records do not round trip: %+v err=%v", decoded, err)
	}
	var limited bytes.Buffer
	if err := writeBackupClipArray(&limited, clips, int64(complete.Len()-1)); !errors.Is(err, errBackupLimit) {
		t.Fatalf("expected a byte budget error, got %v", err)
	}
	limited.Reset()
	if err := writeBackupClipArray(&limited, clips, int64(complete.Len())); err != nil {
		t.Fatalf("an exact-fit record array was rejected: %v", err)
	}
}

func TestBackupImageEnforcesRemainingTotalBudget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.dib")
	if err := os.WriteFile(path, []byte("12345"), 0644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	defer archive.Close()
	if _, _, err := writeBackupImage(archive, path, 1, map[string]bool{}, 4); !errors.Is(err, errBackupLimit) {
		t.Fatalf("expected the remaining image budget to be enforced, got %v", err)
	}
	if _, written, err := writeBackupImage(archive, path, 1, map[string]bool{}, 5); err != nil || written != 5 {
		t.Fatalf("exact-fit image failed: written=%d err=%v", written, err)
	}
}

func TestRestoreCorruptImageRemovesPartialFile(t *testing.T) {
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	writer, err := archive.Create("images/image.dib")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("image bytes")); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(output.Bytes()), int64(output.Len()))
	if err != nil {
		t.Fatal(err)
	}
	reader.File[0].CRC32 ^= 1
	dir := t.TempDir()
	if _, _, err := restoreBackupImage(reader.File[0], dir, 0, maxBackupTotalImageBytes); err == nil {
		t.Fatal("corrupt image was accepted")
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 0 {
		t.Fatalf("partial image files remain: %v err=%v", files, err)
	}
}
