package storage

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"clipbox/internal/clipboard"

	_ "modernc.org/sqlite"
)

type Store struct {
	db           *sql.DB
	dataDir      string
	ftsAvailable bool
	imageGCMu    sync.Mutex
}

type CleanupPolicy struct {
	MaxClips   int
	MaxDays    int
	MaxImageMB int
}

type Stats struct {
	TotalClips      int   `json:"totalClips"`
	TextClips       int   `json:"textClips"`
	ImageClips      int   `json:"imageClips"`
	PinnedClips     int   `json:"pinnedClips"`
	ImageBytes      int64 `json:"imageBytes"`
	DatabaseBytes   int64 `json:"databaseBytes"`
	MissingImages   int   `json:"missingImages"`
	UnpinnedClips   int   `json:"unpinnedClips"`
	ThumbnailBytes  int64 `json:"thumbnailBytes"`
	SettingsEntries int   `json:"settingsEntries"`
}

type MaintenanceResult struct {
	MissingImagesRemoved int   `json:"missingImagesRemoved"`
	DatabaseBytesBefore  int64 `json:"databaseBytesBefore"`
	DatabaseBytesAfter   int64 `json:"databaseBytesAfter"`
	TotalClipsBefore     int   `json:"totalClipsBefore"`
	TotalClipsAfter      int   `json:"totalClipsAfter"`
	Vacuumed             bool  `json:"vacuumed"`
	Optimized            bool  `json:"optimized"`
	Stats                Stats `json:"stats"`
}

type ClearResult struct {
	DeletedClips  int   `json:"deletedClips"`
	DeletedImages int   `json:"deletedImages"`
	PinnedKept    int   `json:"pinnedKept"`
	Stats         Stats `json:"stats"`
}

type TagCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

const clipColumns = `id, type, content, preview, thumbnail, timestamp, pinned, source_app, source_title, source_path, note, tags`

// Lists never need the full body. Read it only for copy, details or export.
// Projecting an empty value in SQL also avoids allocating it in the driver.
const clipListColumns = `id, type, '' AS content, preview, thumbnail, timestamp, pinned, source_app, source_title, source_path, note, tags`

const (
	maxNoteRunes = 2000
	maxTags      = 20
	maxTagRunes  = 40
)

var tagSeparators = regexp.MustCompile(`[,\n\r;，；]+`)

func NewStore(dataDir string) (*Store, error) {
	if tagKeyRegistrationError != nil {
		return nil, fmt.Errorf("register tag key function: %w", tagKeyRegistrationError)
	}
	dbPath := filepath.Join(dataDir, "clipbox.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	s := &Store{db: db, dataDir: dataDir}
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		db.Close()
		return nil, fmt.Errorf("set sqlite busy_timeout: %w", err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		db.Close()
		return nil, fmt.Errorf("set sqlite journal_mode: %w", err)
	}
	if _, err := db.Exec(`PRAGMA synchronous = NORMAL`); err != nil {
		db.Close()
		return nil, fmt.Errorf("set sqlite synchronous: %w", err)
	}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	// A previous exit or a locked file can leave committed image deletions to
	// finish. Keep failed attempts queued without preventing history startup.
	_ = s.RetryImageDeletes()

	return s, nil
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS clips (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			content TEXT NOT NULL,
			preview TEXT NOT NULL,
			timestamp INTEGER NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create clips table: %w", err)
	}

	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("create settings table: %w", err)
	}
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS pending_image_deletes (path TEXT PRIMARY KEY)`); err != nil {
		return fmt.Errorf("create image deletion queue: %w", err)
	}

	if err := s.addColumnIfMissing("clips", "pinned", "INTEGER DEFAULT 0"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("clips", "content_hash", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("clips", "thumbnail", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("clips", "source_app", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("clips", "source_title", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("clips", "source_path", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("clips", "note", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("clips", "tags", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := s.backfillContentHashes(); err != nil {
		return err
	}

	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_clips_timestamp ON clips(timestamp DESC)`); err != nil {
		return fmt.Errorf("create timestamp index: %w", err)
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_clips_type ON clips(type)`); err != nil {
		return fmt.Errorf("create type index: %w", err)
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_clips_hash ON clips(content_hash)`); err != nil {
		return fmt.Errorf("create content hash index: %w", err)
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_clips_list_order ON clips(pinned DESC, timestamp DESC)`); err != nil {
		return fmt.Errorf("create list order index: %w", err)
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_clips_type_order ON clips(type, pinned DESC, timestamp DESC)`); err != nil {
		return fmt.Errorf("create type order index: %w", err)
	}

	s.ensureSearchIndex()
	return nil
}

func (s *Store) addColumnIfMissing(table, column, colDef string) error {
	rows, err := s.db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return fmt.Errorf("inspect table %s: %w", table, err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("scan table info for %s: %w", table, err)
		}
		if name == column {
			found = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate table info for %s: %w", table, err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close table info for %s: %w", table, err)
	}

	if !found {
		if _, err := s.db.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + colDef); err != nil {
			return fmt.Errorf("add column %s.%s: %w", table, column, err)
		}
	}
	return nil
}

func (s *Store) GetSetting(key string) (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(
		"INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		key, value,
	)
	return err
}

// SetSettings writes multiple settings in a single transaction.
func (s *Store) SetSettings(values map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	for key, value := range values {
		if _, err := tx.Exec(
			"INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
			key, value,
		); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func hashContent(content string) string {
	return hashBytes([]byte(content))
}

func hashBytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func (s *Store) hashEntryContent(entry clipboard.ClipEntry) (string, error) {
	if entry.Type == "image" && entry.Content != "" {
		if file, err := os.Open(entry.Content); err == nil {
			defer file.Close()
			hash := sha256.New()
			if _, err := io.Copy(hash, file); err != nil {
				return "", err
			}
			return hex.EncodeToString(hash.Sum(nil)), nil
		}
	}
	return hashContent(entry.Content), nil
}

func (s *Store) backfillContentHashes() error {
	rows, err := s.db.Query(`SELECT id, type, content, preview, thumbnail, timestamp, pinned FROM clips WHERE content_hash = '' OR content_hash IS NULL`)
	if err != nil {
		return fmt.Errorf("query clips needing content_hash: %w", err)
	}
	defer rows.Close()

	var entries []clipboard.ClipEntry
	for rows.Next() {
		var e clipboard.ClipEntry
		if err := rows.Scan(&e.ID, &e.Type, &e.Content, &e.Preview, &e.Thumbnail, &e.Timestamp, &e.Pinned); err != nil {
			return fmt.Errorf("scan clip needing content_hash: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate clips needing content_hash: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close clips needing content_hash: %w", err)
	}

	for _, e := range entries {
		h, err := s.hashEntryContent(e)
		if err != nil {
			return err
		}
		if _, err := s.db.Exec(`UPDATE clips SET content_hash = ? WHERE id = ?`, h, e.ID); err != nil {
			return fmt.Errorf("backfill content_hash for clip %d: %w", e.ID, err)
		}
	}
	return nil
}

func (s *Store) SaveOrUpdate(entry clipboard.ClipEntry) (int64, error) {
	entry = normalizeClipMetadata(entry)
	h, err := s.hashEntryContent(entry)
	if err != nil {
		return 0, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var existingID int64
	var existingContent string
	var existingThumbnail string
	var existingNote string
	var existingTags string
	err = tx.QueryRow("SELECT id, content, thumbnail, note, tags FROM clips WHERE content_hash = ?", h).Scan(&existingID, &existingContent, &existingThumbnail, &existingNote, &existingTags)
	if err == sql.ErrNoRows {
		result, err := tx.Exec(
			`INSERT INTO clips (type, content, preview, thumbnail, timestamp, pinned, content_hash, source_app, source_title, source_path, note, tags) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			entry.Type, entry.Content, entry.Preview, entry.Thumbnail, entry.Timestamp, entry.Pinned, h, entry.SourceApp, entry.SourceTitle, entry.SourcePath, entry.Note, entry.Tags,
		)
		if err != nil {
			return 0, err
		}
		id, err := result.LastInsertId()
		if err != nil {
			return 0, err
		}
		return id, tx.Commit()
	} else if err != nil {
		return 0, err
	}

	// Update existing entry's timestamp to now
	thumbnail := existingThumbnail
	if thumbnail == "" && entry.Thumbnail != "" {
		thumbnail = entry.Thumbnail
	}
	hasSource := entry.SourceApp != "" || entry.SourceTitle != "" || entry.SourcePath != ""
	sourceFlag := 0
	if hasSource {
		sourceFlag = 1
	}
	note := existingNote
	if note == "" && entry.Note != "" {
		note = entry.Note
	}
	tags := existingTags
	tags = mergeTags(tags, entry.Tags)
	content := existingContent
	var discard []string
	if entry.Type == "image" {
		content, discard = chooseImageContent(existingContent, entry.Content)
	}
	_, err = tx.Exec(
		`UPDATE clips
		 SET content = ?, timestamp = ?,
		     thumbnail = ?,
		     source_app = CASE WHEN ? THEN ? ELSE source_app END,
		     source_title = CASE WHEN ? THEN ? ELSE source_title END,
		     source_path = CASE WHEN ? THEN ? ELSE source_path END,
		     note = ?,
		     tags = ?
		 WHERE id = ?`,
		content, entry.Timestamp, thumbnail,
		sourceFlag, entry.SourceApp,
		sourceFlag, entry.SourceTitle,
		sourceFlag, entry.SourcePath,
		note,
		tags,
		existingID,
	)
	if err != nil {
		return 0, err
	}
	if err := s.queueImageDeletes(tx, discard); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	if len(discard) > 0 {
		_ = s.RetryImageDeletes()
	}
	return existingID, nil
}
