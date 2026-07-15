package storage

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"clipbox/internal/clipboard"

	_ "modernc.org/sqlite"
)

type Store struct {
	db           *sql.DB
	dataDir      string
	ftsAvailable bool
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

const (
	maxNoteRunes = 2000
	maxTags      = 20
	maxTagRunes  = 40
)

var tagSeparators = regexp.MustCompile(`[,\n\r;，；]+`)

func NewStore(dataDir string) (*Store, error) {
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

	s.ensureSearchIndex()
	return nil
}

func (s *Store) ensureSearchIndex() {
	if _, err := s.db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS clips_fts USING fts5(content, source_app, source_title, note, tags, tokenize='trigram')`); err != nil {
		s.ftsAvailable = false
		return
	}
	triggers := []string{
		`CREATE TRIGGER IF NOT EXISTS clips_fts_ai AFTER INSERT ON clips BEGIN
			INSERT INTO clips_fts(rowid, content, source_app, source_title, note, tags)
			VALUES (new.id, CASE WHEN new.type = 'text' THEN new.content ELSE '' END, new.source_app, new.source_title, new.note, new.tags);
		END`,
		`CREATE TRIGGER IF NOT EXISTS clips_fts_ad AFTER DELETE ON clips BEGIN
			DELETE FROM clips_fts WHERE rowid = old.id;
		END`,
		`CREATE TRIGGER IF NOT EXISTS clips_fts_au AFTER UPDATE ON clips BEGIN
			DELETE FROM clips_fts WHERE rowid = old.id;
			INSERT INTO clips_fts(rowid, content, source_app, source_title, note, tags)
			VALUES (new.id, CASE WHEN new.type = 'text' THEN new.content ELSE '' END, new.source_app, new.source_title, new.note, new.tags);
		END`,
	}
	for _, trigger := range triggers {
		if _, err := s.db.Exec(trigger); err != nil {
			s.ftsAvailable = false
			return
		}
	}
	if err := s.rebuildSearchIndex(); err != nil {
		s.ftsAvailable = false
		return
	}
	s.ftsAvailable = true
}

func (s *Store) rebuildSearchIndex() error {
	if _, err := s.db.Exec(`DELETE FROM clips_fts`); err != nil {
		return err
	}
	_, err := s.db.Exec(`
		INSERT INTO clips_fts(rowid, content, source_app, source_title, note, tags)
		SELECT id,
		       CASE WHEN type = 'text' THEN content ELSE '' END,
		       source_app,
		       source_title,
		       note,
		       tags
		  FROM clips
	`)
	return err
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
		if data, err := os.ReadFile(entry.Content); err == nil {
			return hashBytes(data), nil
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
	var existingID int64
	var existingContent string
	var existingThumbnail string
	var existingNote string
	var existingTags string
	err = s.db.QueryRow("SELECT id, content, thumbnail, note, tags FROM clips WHERE content_hash = ?", h).Scan(&existingID, &existingContent, &existingThumbnail, &existingNote, &existingTags)
	if err == sql.ErrNoRows {
		result, err := s.db.Exec(
			`INSERT INTO clips (type, content, preview, thumbnail, timestamp, pinned, content_hash, source_app, source_title, source_path, note, tags) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			entry.Type, entry.Content, entry.Preview, entry.Thumbnail, entry.Timestamp, entry.Pinned, h, entry.SourceApp, entry.SourceTitle, entry.SourcePath, entry.Note, entry.Tags,
		)
		if err != nil {
			return 0, err
		}
		return result.LastInsertId()
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
	_, err = s.db.Exec(
		`UPDATE clips
		 SET timestamp = ?,
		     thumbnail = ?,
		     source_app = CASE WHEN ? THEN ? ELSE source_app END,
		     source_title = CASE WHEN ? THEN ? ELSE source_title END,
		     source_path = CASE WHEN ? THEN ? ELSE source_path END,
		     note = ?,
		     tags = ?
		 WHERE id = ?`,
		entry.Timestamp, thumbnail,
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
	if entry.Type == "image" && entry.Content != "" && entry.Content != existingContent {
		s.removeImageFile(entry.Content)
	}
	return existingID, nil
}

func (s *Store) GetByID(id int64) (*clipboard.ClipEntry, error) {
	var e clipboard.ClipEntry
	err := s.db.QueryRow(
		`SELECT `+clipColumns+` FROM clips WHERE id = ?`, id,
	).Scan(&e.ID, &e.Type, &e.Content, &e.Preview, &e.Thumbnail, &e.Timestamp, &e.Pinned, &e.SourceApp, &e.SourceTitle, &e.SourcePath, &e.Note, &e.Tags)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// GetAll returns clips for the list view.
// For image entries, Content is returned empty so we don't ship huge
// base64 strings to the frontend. Call GetByID / GetImagePath when
// you need the actual image data.
func (s *Store) GetAll(limit, offset int) ([]clipboard.ClipEntry, error) {
	limit, offset = normalizePage(limit, offset)
	return s.queryClips(``, nil, limit, offset)
}

func (s *Store) GetByType(clipType string, limit, offset int) ([]clipboard.ClipEntry, error) {
	clipType = normalizeClipType(clipType)
	if clipType == "all" {
		return s.GetAll(limit, offset)
	}
	limit, offset = normalizePage(limit, offset)
	return s.queryClips(`WHERE type = ?`, []interface{}{clipType}, limit, offset)
}

func (s *Store) GetByTypeAndTag(clipType, tag string, limit, offset int) ([]clipboard.ClipEntry, error) {
	tag = normalizeTagFilter(tag)
	if tag == "" {
		return s.GetByType(clipType, limit, offset)
	}
	clipType = normalizeClipType(clipType)
	limit, offset = normalizePage(limit, offset)
	where, args := typeWhere(clipType)
	where, args = appendTagWhere(where, args, tag)
	return s.queryClips(where, args, limit, offset)
}

func (s *Store) queryClips(where string, args []interface{}, limit, offset int) ([]clipboard.ClipEntry, error) {
	queryArgs := append([]interface{}{}, args...)
	queryArgs = append(queryArgs, limit, offset)
	query := `SELECT ` + clipColumns + ` FROM clips ` + where + ` ORDER BY pinned DESC, timestamp DESC LIMIT ? OFFSET ?`
	rows, err := s.db.Query(query, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []clipboard.ClipEntry
	for rows.Next() {
		var e clipboard.ClipEntry
		if err := rows.Scan(&e.ID, &e.Type, &e.Content, &e.Preview, &e.Thumbnail, &e.Timestamp, &e.Pinned, &e.SourceApp, &e.SourceTitle, &e.SourcePath, &e.Note, &e.Tags); err != nil {
			return nil, err
		}
		// Never send image data (file path or base64) to the frontend in list views.
		// The frontend requests images on demand via GetImageDataURL.
		if e.Type == "image" {
			e.Content = ""
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func normalizeClipType(clipType string) string {
	switch clipType {
	case "text", "image":
		return clipType
	default:
		return "all"
	}
}

func normalizePage(limit, offset int) (int, int) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func (s *Store) CountAll() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM clips`).Scan(&count)
	return count, err
}

func (s *Store) CountByType(clipType string) (int, error) {
	clipType = normalizeClipType(clipType)
	if clipType == "all" {
		return s.CountAll()
	}
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM clips WHERE type = ?`, clipType).Scan(&count)
	return count, err
}

func (s *Store) CountByTypeAndTag(clipType, tag string) (int, error) {
	tag = normalizeTagFilter(tag)
	if tag == "" {
		return s.CountByType(clipType)
	}
	where, args := typeWhere(normalizeClipType(clipType))
	where, args = appendTagWhere(where, args, tag)
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM clips `+where, args...).Scan(&count)
	return count, err
}

func (s *Store) Search(query string, limit, offset int) ([]clipboard.ClipEntry, error) {
	return s.SearchByType(query, "text", limit, offset)
}

func (s *Store) SearchByType(query, clipType string, limit, offset int) ([]clipboard.ClipEntry, error) {
	if entries, ok, err := s.searchFTS(query, clipType, "", limit, offset); ok {
		return entries, err
	}
	clipType = normalizeClipType(clipType)
	limit, offset = normalizePage(limit, offset)
	where, args := searchWhere(query, clipType)
	args = append(args, limit, offset)
	rows, err := s.db.Query(
		`SELECT `+clipColumns+`
		 FROM clips `+where+`
		 ORDER BY pinned DESC, timestamp DESC
		 LIMIT ? OFFSET ?`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []clipboard.ClipEntry
	for rows.Next() {
		var e clipboard.ClipEntry
		if err := rows.Scan(&e.ID, &e.Type, &e.Content, &e.Preview, &e.Thumbnail, &e.Timestamp, &e.Pinned, &e.SourceApp, &e.SourceTitle, &e.SourcePath, &e.Note, &e.Tags); err != nil {
			return nil, err
		}
		if e.Type == "image" {
			e.Content = ""
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *Store) SearchByTypeAndTag(query, clipType, tag string, limit, offset int) ([]clipboard.ClipEntry, error) {
	tag = normalizeTagFilter(tag)
	if tag == "" {
		return s.SearchByType(query, clipType, limit, offset)
	}
	if entries, ok, err := s.searchFTS(query, clipType, tag, limit, offset); ok {
		return entries, err
	}
	clipType = normalizeClipType(clipType)
	limit, offset = normalizePage(limit, offset)
	where, args := searchWhere(query, clipType)
	where, args = appendTagWhere(where, args, tag)
	args = append(args, limit, offset)
	rows, err := s.db.Query(
		`SELECT `+clipColumns+`
		 FROM clips `+where+`
		 ORDER BY pinned DESC, timestamp DESC
		 LIMIT ? OFFSET ?`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []clipboard.ClipEntry
	for rows.Next() {
		var e clipboard.ClipEntry
		if err := rows.Scan(&e.ID, &e.Type, &e.Content, &e.Preview, &e.Thumbnail, &e.Timestamp, &e.Pinned, &e.SourceApp, &e.SourceTitle, &e.SourcePath, &e.Note, &e.Tags); err != nil {
			return nil, err
		}
		if e.Type == "image" {
			e.Content = ""
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *Store) CountSearch(query string) (int, error) {
	return s.CountSearchByType(query, "text")
}

func (s *Store) CountSearchByType(query, clipType string) (int, error) {
	if count, ok, err := s.countSearchFTS(query, clipType, ""); ok {
		return count, err
	}
	clipType = normalizeClipType(clipType)
	where, args := searchWhere(query, clipType)
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM clips `+where, args...).Scan(&count)
	return count, err
}

func (s *Store) CountSearchByTypeAndTag(query, clipType, tag string) (int, error) {
	tag = normalizeTagFilter(tag)
	if tag == "" {
		return s.CountSearchByType(query, clipType)
	}
	if count, ok, err := s.countSearchFTS(query, clipType, tag); ok {
		return count, err
	}
	where, args := searchWhere(query, normalizeClipType(clipType))
	where, args = appendTagWhere(where, args, tag)
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM clips `+where, args...).Scan(&count)
	return count, err
}

func (s *Store) searchFTS(query, clipType, tag string, limit, offset int) ([]clipboard.ClipEntry, bool, error) {
	if !s.shouldUseFTS(query) {
		return nil, false, nil
	}
	limit, offset = normalizePage(limit, offset)
	where, args, ok := ftsWhere(query, clipType, tag)
	if !ok {
		return nil, false, nil
	}
	args = append(args, limit, offset)
	rows, err := s.db.Query(
		`SELECT `+qualifiedClipColumns("c")+`
		   FROM clips c
		   JOIN clips_fts ON clips_fts.rowid = c.id
		  WHERE `+where+`
		  ORDER BY c.pinned DESC, c.timestamp DESC
		  LIMIT ? OFFSET ?`,
		args...,
	)
	if err != nil {
		return nil, false, nil
	}
	defer rows.Close()
	entries, err := scanClipRows(rows)
	return entries, true, err
}

func (s *Store) countSearchFTS(query, clipType, tag string) (int, bool, error) {
	if !s.shouldUseFTS(query) {
		return 0, false, nil
	}
	where, args, ok := ftsWhere(query, clipType, tag)
	if !ok {
		return 0, false, nil
	}
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*)
		   FROM clips c
		   JOIN clips_fts ON clips_fts.rowid = c.id
		  WHERE `+where,
		args...,
	).Scan(&count)
	if err != nil {
		return 0, false, nil
	}
	return count, true, nil
}

func (s *Store) queryTagRowsFTS(query, clipType string) (*sql.Rows, error) {
	where, args, ok := ftsWhere(query, clipType, "")
	if !ok {
		return nil, fmt.Errorf("FTS query is not available")
	}
	return s.db.Query(
		`SELECT c.tags
		   FROM clips c
		   JOIN clips_fts ON clips_fts.rowid = c.id
		  WHERE `+where,
		args...,
	)
}

func (s *Store) shouldUseFTS(query string) bool {
	return s.ftsAvailable && ftsQueryLongEnough(query)
}

func ftsWhere(query, clipType, tag string) (string, []interface{}, bool) {
	match, ok := ftsMatchQuery(query)
	if !ok {
		return "", nil, false
	}
	conditions := []string{"clips_fts MATCH ?"}
	args := []interface{}{match}
	switch normalizeClipType(clipType) {
	case "text", "image":
		conditions = append(conditions, "c.type = ?")
		args = append(args, normalizeClipType(clipType))
	}
	if tag = normalizeTagFilter(tag); tag != "" {
		conditions = append(conditions, `instr(', ' || c.tags || ', ', ?) > 0`)
		args = append(args, ", "+tag+", ")
	}
	return strings.Join(conditions, " AND "), args, true
}

func ftsMatchQuery(query string) (string, bool) {
	query = strings.TrimSpace(query)
	if !ftsQueryLongEnough(query) {
		return "", false
	}
	query = strings.ReplaceAll(query, `"`, `""`)
	return `"` + query + `"`, true
}

func ftsQueryLongEnough(query string) bool {
	return len([]rune(strings.TrimSpace(query))) >= 3
}

func qualifiedClipColumns(alias string) string {
	columns := strings.Split(clipColumns, ", ")
	for i, column := range columns {
		columns[i] = alias + "." + column
	}
	return strings.Join(columns, ", ")
}

func scanClipRows(rows *sql.Rows) ([]clipboard.ClipEntry, error) {
	var entries []clipboard.ClipEntry
	for rows.Next() {
		var e clipboard.ClipEntry
		if err := rows.Scan(&e.ID, &e.Type, &e.Content, &e.Preview, &e.Thumbnail, &e.Timestamp, &e.Pinned, &e.SourceApp, &e.SourceTitle, &e.SourcePath, &e.Note, &e.Tags); err != nil {
			return nil, err
		}
		if e.Type == "image" {
			e.Content = ""
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func searchWhere(query, clipType string) (string, []interface{}) {
	pattern := "%" + query + "%"
	switch normalizeClipType(clipType) {
	case "text":
		return `WHERE type = 'text' AND (content LIKE ? OR source_app LIKE ? OR source_title LIKE ? OR note LIKE ? OR tags LIKE ?)`,
			[]interface{}{pattern, pattern, pattern, pattern, pattern}
	case "image":
		return `WHERE type = 'image' AND (source_app LIKE ? OR source_title LIKE ? OR note LIKE ? OR tags LIKE ?)`,
			[]interface{}{pattern, pattern, pattern, pattern}
	default:
		return `WHERE ((type = 'text' AND content LIKE ?) OR source_app LIKE ? OR source_title LIKE ? OR note LIKE ? OR tags LIKE ?)`,
			[]interface{}{pattern, pattern, pattern, pattern, pattern}
	}
}

func typeWhere(clipType string) (string, []interface{}) {
	switch normalizeClipType(clipType) {
	case "text", "image":
		return `WHERE type = ?`, []interface{}{clipType}
	default:
		return ``, nil
	}
}

func appendTagWhere(where string, args []interface{}, tag string) (string, []interface{}) {
	tag = normalizeTagFilter(tag)
	if tag == "" {
		return where, args
	}
	condition := `instr(', ' || tags || ', ', ?) > 0`
	args = append(args, ", "+tag+", ")
	if strings.TrimSpace(where) == "" {
		return `WHERE ` + condition, args
	}
	return where + ` AND ` + condition, args
}

func (s *Store) ListTags(query, clipType string) ([]TagCount, error) {
	query = strings.TrimSpace(query)
	var where string
	var args []interface{}
	if query == "" {
		where, args = typeWhere(normalizeClipType(clipType))
	} else {
		where, args = searchWhere(query, normalizeClipType(clipType))
	}
	var rows *sql.Rows
	var err error
	if query != "" && s.shouldUseFTS(query) {
		rows, err = s.queryTagRowsFTS(query, clipType)
		if err != nil {
			rows = nil
		}
	}
	if rows == nil {
		rows, err = s.db.Query(`SELECT tags FROM clips `+where, args...)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := map[string]int{}
	names := map[string]string{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		for _, tag := range splitNormalizedTags(raw) {
			key := strings.ToLower(tag)
			counts[key]++
			if _, ok := names[key]; !ok {
				names[key] = tag
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	tags := make([]TagCount, 0, len(counts))
	for key, count := range counts {
		tags = append(tags, TagCount{Name: names[key], Count: count})
	}
	sortTagCounts(tags)
	return tags, nil
}

func (s *Store) UpdateMetadata(id int64, note, tags string) (*clipboard.ClipEntry, error) {
	entry := normalizeClipMetadata(clipboard.ClipEntry{Note: note, Tags: tags})
	result, err := s.db.Exec(`UPDATE clips SET note = ?, tags = ? WHERE id = ?`, entry.Note, entry.Tags, id)
	if err != nil {
		return nil, err
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return nil, nil
	}
	return s.GetByID(id)
}

func normalizeClipMetadata(entry clipboard.ClipEntry) clipboard.ClipEntry {
	entry.Note = truncateRunes(strings.TrimSpace(entry.Note), maxNoteRunes)
	entry.Tags = normalizeTags(entry.Tags)
	return entry
}

func normalizeTags(tags string) string {
	parts := tagSeparators.Split(tags, -1)
	normalized := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		tag := truncateRunes(strings.TrimSpace(part), maxTagRunes)
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, tag)
		if len(normalized) >= maxTags {
			break
		}
	}
	return strings.Join(normalized, ", ")
}

func normalizeTagFilter(tag string) string {
	tags := splitNormalizedTags(normalizeTags(tag))
	if len(tags) == 0 {
		return ""
	}
	return tags[0]
}

func splitNormalizedTags(tags string) []string {
	if strings.TrimSpace(tags) == "" {
		return nil
	}
	parts := strings.Split(tags, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		tag := strings.TrimSpace(part)
		if tag != "" {
			result = append(result, tag)
		}
	}
	return result
}

func sortTagCounts(tags []TagCount) {
	for i := 0; i < len(tags); i++ {
		for j := i + 1; j < len(tags); j++ {
			if tags[j].Count > tags[i].Count || (tags[j].Count == tags[i].Count && strings.ToLower(tags[j].Name) < strings.ToLower(tags[i].Name)) {
				tags[i], tags[j] = tags[j], tags[i]
			}
		}
	}
}

func mergeTags(existing, incoming string) string {
	if strings.TrimSpace(existing) == "" {
		return normalizeTags(incoming)
	}
	if strings.TrimSpace(incoming) == "" {
		return normalizeTags(existing)
	}
	return normalizeTags(existing + "," + incoming)
}

func truncateRunes(text string, max int) string {
	if max <= 0 {
		return ""
	}
	count := 0
	for i := range text {
		if count == max {
			return text[:i]
		}
		count++
	}
	return text
}

func (s *Store) TogglePin(id int64) error {
	_, err := s.db.Exec(`UPDATE clips SET pinned = CASE WHEN pinned = 1 THEN 0 ELSE 1 END WHERE id = ?`, id)
	return err
}

func (s *Store) Delete(id int64) error {
	// If this is an image clip, also delete the file from disk.
	entry, err := s.GetByID(id)
	if err == nil && entry != nil && entry.Type == "image" && entry.Content != "" {
		s.removeImageFile(entry.Content)
	}
	_, err = s.db.Exec(`DELETE FROM clips WHERE id = ?`, id)
	return err
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
		} else {
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
		if _, err := os.Stat(path); err != nil {
			ids = append(ids, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *Store) Cleanup(policy CleanupPolicy) error {
	if policy.MaxDays > 0 {
		cutoff := int64(0)
		cutoff = (int64(policy.MaxDays) * 24 * 60 * 60 * 1000)
		if err := s.deleteWhere(`pinned = 0 AND timestamp < (strftime('%s','now') * 1000 - ?)`, cutoff); err != nil {
			return err
		}
	}

	if policy.MaxClips > 0 {
		ids, err := s.idsBeyondLimit(policy.MaxClips)
		if err != nil {
			return err
		}
		if err := s.deleteIDs(ids); err != nil {
			return err
		}
	}

	if policy.MaxImageMB > 0 {
		ids, err := s.imageIDsOverLimit(int64(policy.MaxImageMB) * 1024 * 1024)
		if err != nil {
			return err
		}
		if err := s.deleteIDs(ids); err != nil {
			return err
		}
	}

	return nil
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
	if err := tx.Commit(); err != nil {
		return result, err
	}
	committed = true

	seenPaths := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if _, seen := seenPaths[path]; seen {
			continue
		}
		seenPaths[path] = struct{}{}
		s.removeImageFile(path)
	}

	stats, err := s.Stats()
	if err != nil {
		return result, err
	}
	result.Stats = stats
	return result, nil
}

func (s *Store) DiscardEntryFiles(entry clipboard.ClipEntry) {
	if entry.Type == "image" && entry.Content != "" {
		s.removeImageFile(entry.Content)
	}
}

func (s *Store) deleteWhere(where string, args ...interface{}) error {
	rows, err := s.db.Query(`SELECT id FROM clips WHERE `+where, args...)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	return s.deleteIDs(ids)
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
	for _, id := range ids {
		entry, err := s.GetByID(id)
		if err != nil {
			return err
		}
		if entry != nil {
			s.DiscardEntryFiles(*entry)
		}
		if _, err := s.db.Exec(`DELETE FROM clips WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) removeImageFile(content string) {
	if content == "" {
		return
	}
	imageDir, err := filepath.Abs(filepath.Join(s.dataDir, "images"))
	if err != nil {
		return
	}
	path, err := filepath.Abs(content)
	if err != nil {
		return
	}
	rel, err := filepath.Rel(imageDir, path)
	if err != nil || rel == "." || filepath.IsAbs(rel) || strings.HasPrefix(rel, "..") {
		return
	}
	_ = os.Remove(path)
}

func (s *Store) Close() error {
	return s.db.Close()
}
