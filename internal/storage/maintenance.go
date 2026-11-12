package storage

import (
	"database/sql"
	"os"
	"path/filepath"

	"clipbox/internal/clipboard"

	_ "modernc.org/sqlite"
)

func (s *Store) TogglePin(id int64) error {
	_, err := s.db.Exec(`UPDATE clips SET pinned = CASE WHEN pinned = 1 THEN 0 ELSE 1 END WHERE id = ?`, id)
	return err
}

func (s *Store) Delete(id int64) error {
	return s.deleteIDs([]int64{id})
}

func (s *Store) Stats() (Stats, error) {
	var stats Stats
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM clips`).Scan(&stats.TotalClips); err != nil {
		return stats, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM clips WHERE type = 'text'`).Scan(&stats.TextClips); err != nil {
		return stats, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM clips WHERE type = 'image'`).Scan(&stats.ImageClips); err != nil {
		return stats, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM clips WHERE pinned = 1`).Scan(&stats.PinnedClips); err != nil {
		return stats, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM clips WHERE pinned = 0`).Scan(&stats.UnpinnedClips); err != nil {
		return stats, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM settings`).Scan(&stats.SettingsEntries); err != nil {
		return stats, err
	}
	if err := s.db.QueryRow(`SELECT COALESCE(SUM(LENGTH(thumbnail)), 0) FROM clips`).Scan(&stats.ThumbnailBytes); err != nil {
		return stats, err
	}

	dbPath := filepath.Join(s.dataDir, "clipbox.db")
	if info, err := os.Stat(dbPath); err == nil {
		stats.DatabaseBytes = info.Size()
	}

	rows, err := s.db.Query(`SELECT content FROM clips WHERE type = 'image' AND content != ''`)
	if err != nil {
		return stats, err
	}
	defer rows.Close()

	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return stats, err
		}
		if info, err := os.Stat(path); err == nil {
			stats.ImageBytes += info.Size()
		} else if missingImageFile(path) {
			stats.MissingImages++
		}
	}
	if err := rows.Err(); err != nil {
		return stats, err
	}
	return stats, nil
}

func (s *Store) Maintain() (MaintenanceResult, error) {
	before, err := s.Stats()
	if err != nil {
		return MaintenanceResult{}, err
	}

	missingIDs, err := s.missingImageIDs()
	if err != nil {
		return MaintenanceResult{}, err
	}
	if err := s.deleteIDs(missingIDs); err != nil {
		return MaintenanceResult{}, err
	}
	if s.ftsAvailable {
		if err := s.rebuildSearchIndex(); err != nil {
			return MaintenanceResult{}, err
		}
	}

	result := MaintenanceResult{
		MissingImagesRemoved: len(missingIDs),
		DatabaseBytesBefore:  before.DatabaseBytes,
		TotalClipsBefore:     before.TotalClips,
	}
	if _, err := s.db.Exec(`PRAGMA optimize`); err != nil {
		return MaintenanceResult{}, err
	}
	result.Optimized = true
	if _, err := s.db.Exec(`VACUUM`); err != nil {
		return MaintenanceResult{}, err
	}
	result.Vacuumed = true

	after, err := s.Stats()
	if err != nil {
		return MaintenanceResult{}, err
	}
	result.Stats = after
	result.DatabaseBytesAfter = after.DatabaseBytes
	result.TotalClipsAfter = after.TotalClips
	return result, nil
}

func (s *Store) missingImageIDs() ([]int64, error) {
	rows, err := s.db.Query(`SELECT id, content FROM clips WHERE type = 'image' AND content != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		var path string
		if err := rows.Scan(&id, &path); err != nil {
			return nil, err
		}
		if missingImageFile(path) {
			ids = append(ids, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *Store) Clear() (ClearResult, error) {
	var result ClearResult

	tx, err := s.db.Begin()
	if err != nil {
		return result, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := tx.QueryRow(`SELECT COUNT(*) FROM clips WHERE pinned = 1`).Scan(&result.PinnedKept); err != nil {
		return result, err
	}

	rows, err := tx.Query(`SELECT content FROM clips WHERE pinned = 0 AND type = 'image' AND content != ''`)
	if err != nil {
		return result, err
	}

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			rows.Close()
			return result, err
		}
		paths = append(paths, path)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()
	result.DeletedImages = len(paths)

	deleteResult, err := tx.Exec(`DELETE FROM clips WHERE pinned = 0`)
	if err != nil {
		return result, err
	}
	if affected, err := deleteResult.RowsAffected(); err == nil {
		result.DeletedClips = int(affected)
	}
	if err := s.queueImageDeletes(tx, paths); err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	committed = true

	_ = s.RetryImageDeletes()

	stats, err := s.Stats()
	if err != nil {
		return result, err
	}
	result.Stats = stats
	return result, nil
}

func (s *Store) DiscardEntryFiles(entry clipboard.ClipEntry) {
	if entry.Type != "image" {
		return
	}
	if _, managed := s.managedImagePath(entry.Content); !managed {
		return
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO pending_image_deletes(path) VALUES (?)`, entry.Content); err == nil {
		_ = s.RetryImageDeletes()
	}
}

func (s *Store) idsWhere(where string, args ...interface{}) ([]int64, error) {
	rows, err := s.db.Query(`SELECT id FROM clips WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	return ids, nil
}

func (s *Store) idsBeyondLimit(limit int) ([]int64, error) {
	rows, err := s.db.Query(
		`SELECT id FROM clips
		 WHERE pinned = 0
		   AND id NOT IN (
		     SELECT id FROM clips ORDER BY pinned DESC, timestamp DESC LIMIT ?
		   )
		 ORDER BY timestamp ASC`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *Store) imageIDsOverLimit(maxBytes int64) ([]int64, error) {
	rows, err := s.db.Query(`SELECT id, content, pinned FROM clips WHERE type = 'image' AND content != '' ORDER BY pinned DESC, timestamp DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type imageClip struct {
		id     int64
		path   string
		size   int64
		pinned bool
	}
	var clips []imageClip
	var total int64
	for rows.Next() {
		var c imageClip
		if err := rows.Scan(&c.id, &c.path, &c.pinned); err != nil {
			return nil, err
		}
		if info, err := os.Stat(c.path); err == nil {
			c.size = info.Size()
			total += c.size
		}
		clips = append(clips, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var ids []int64
	for i := len(clips) - 1; i >= 0 && total > maxBytes; i-- {
		if clips[i].pinned {
			continue
		}
		ids = append(ids, clips[i].id)
		total -= clips[i].size
	}
	return ids, nil
}

func (s *Store) deleteIDs(ids []int64) error {
	_, err := s.deleteIDsWithResult(ids)
	return err
}

func (s *Store) deleteIDsWithResult(ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var paths []string
	var deleted []int64
	for _, id := range ids {
		var clipType, content string
		err := tx.QueryRow(`SELECT type, content FROM clips WHERE id = ?`, id).Scan(&clipType, &content)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`DELETE FROM clips WHERE id = ?`, id); err != nil {
			return nil, err
		}
		deleted = append(deleted, id)
		if clipType == "image" && content != "" {
			paths = append(paths, content)
		}
	}
	if err := s.queueImageDeletes(tx, paths); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	_ = s.RetryImageDeletes()
	return deleted, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}
