package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (s *Store) managedImagePath(content string) (string, bool) {
	if content == "" {
		return "", false
	}
	imageDir, err := filepath.Abs(filepath.Join(s.dataDir, "images"))
	if err != nil {
		return "", false
	}
	path, err := filepath.Abs(content)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(imageDir, path)
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return path, true
}

// queueImageDeletes belongs to the same transaction that deletes the records.
// A rollback must leave both the records and their files intact.
func (s *Store) queueImageDeletes(tx *sql.Tx, paths []string) error {
	for _, path := range paths {
		if _, managed := s.managedImagePath(path); !managed {
			continue
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO pending_image_deletes(path) VALUES (?)`, path); err != nil {
			return err
		}
	}
	return nil
}

// RetryImageDeletes finishes file reclamation after the database has committed.
// Locked files stay queued across restarts. A remaining reference protects a
// shared image, including a pinned record that was excluded from a clear.
func (s *Store) RetryImageDeletes() error {
	s.imageGCMu.Lock()
	defer s.imageGCMu.Unlock()
	rows, err := s.db.Query(`SELECT path FROM pending_image_deletes`)
	if err != nil {
		return err
	}
	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			rows.Close()
			return err
		}
		paths = append(paths, path)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var failures []error
	for _, content := range paths {
		path, managed := s.managedImagePath(content)
		if managed {
			var references int
			if err := s.db.QueryRow(`SELECT COUNT(*) FROM clips WHERE type = 'image' AND content = ? COLLATE NOCASE`, content).Scan(&references); err != nil {
				failures = append(failures, err)
				continue
			}
			if references == 0 {
				if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
					failures = append(failures, fmt.Errorf("reclaim image %s: %w", path, err))
					continue
				}
			}
		}
		if _, err := s.db.Exec(`DELETE FROM pending_image_deletes WHERE path = ?`, content); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
