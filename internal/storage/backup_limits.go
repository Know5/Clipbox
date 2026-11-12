package storage

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

var errBackupLimit = errors.New("backup exceeds format limits")

// Reject obviously oversized histories before loading their text into memory.
// The encoded byte budgets below also account for JSON escaping and metadata.
func (s *Store) checkBackupExportLimits() error {
	var clips int
	var contentBytes int64
	if err := s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN type != 'image' THEN LENGTH(CAST(content AS BLOB)) ELSE 0 END), 0) FROM clips`).Scan(&clips, &contentBytes); err != nil {
		return err
	}
	if clips > maxBackupClips {
		return fmt.Errorf("%w: too many clips (%d, maximum %d)", errBackupLimit, clips, maxBackupClips)
	}
	if contentBytes > maxBackupClipsBytes {
		return fmt.Errorf("%w: clipboard text exceeds %d MiB", errBackupLimit, maxBackupClipsBytes/(1024*1024))
	}
	return nil
}

type backupBudgetWriter struct {
	writer    io.Writer
	remaining int64
}

func (writer *backupBudgetWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > writer.remaining {
		return 0, fmt.Errorf("%w: encoded JSON exceeds its byte budget", errBackupLimit)
	}
	written, err := writer.writer.Write(data)
	writer.remaining -= int64(written)
	if err == nil && written != len(data) {
		err = io.ErrShortWrite
	}
	return written, err
}

func writeZipJSONLimited(archive *zip.Writer, name string, value interface{}, maxBytes int64) error {
	writer, err := archive.Create(name)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(&backupBudgetWriter{writer: writer, remaining: maxBytes})
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("backup %s: %w", name, err)
	}
	return nil
}

// Encode one record at a time so checking the 64 MiB limit does not first
// allocate another complete JSON copy of the entire clipboard history.
func writeBackupClipArray(destination io.Writer, clips []backupClip, maxBytes int64) error {
	writer := &backupBudgetWriter{writer: destination, remaining: maxBytes}
	if _, err := io.WriteString(writer, "["); err != nil {
		return err
	}
	encoder := json.NewEncoder(writer)
	for index, clip := range clips {
		if index > 0 {
			if _, err := io.WriteString(writer, ","); err != nil {
				return err
			}
		}
		if err := encoder.Encode(clip); err != nil {
			return err
		}
	}
	_, err := io.WriteString(writer, "]\n")
	return err
}
