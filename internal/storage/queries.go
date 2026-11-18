package storage

import (
	"database/sql"
	"strings"

	"clipbox/internal/clipboard"

	_ "modernc.org/sqlite"
)

func (s *Store) GetByID(id int64) (*clipboard.ClipEntry, error) {
	return s.getByID(id, clipColumns)
}

func (s *Store) GetListEntry(id int64) (*clipboard.ClipEntry, error) {
	return s.getByID(id, clipListColumns)
}

func (s *Store) getByID(id int64, columns string) (*clipboard.ClipEntry, error) {
	var e clipboard.ClipEntry
	err := s.db.QueryRow(
		`SELECT `+columns+` FROM clips WHERE id = ?`, id,
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
// Content is empty for both text and images. GetByID returns the full record.
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
	query := `SELECT ` + clipListColumns + ` FROM clips ` + where + ` ORDER BY pinned DESC, timestamp DESC LIMIT ? OFFSET ?`
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

func qualifiedClipColumns(alias string) string {
	columns := strings.Split(clipColumns, ", ")
	for i, column := range columns {
		if column == "content" {
			columns[i] = "'' AS content"
		} else {
			columns[i] = alias + "." + column
		}
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

// GetImagePathsByIDs fetches stored image content paths for the requested IDs
// in a single query per batch instead of one round-trip per row.
func (s *Store) GetImagePathsByIDs(ids []int64) (map[int64]string, error) {
	result := make(map[int64]string, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	unique := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return result, nil
	}
	const chunkSize = 500
	for start := 0; start < len(unique); start += chunkSize {
		end := start + chunkSize
		if end > len(unique) {
			end = len(unique)
		}
		batch := unique[start:end]
		placeholders := strings.Repeat("?,", len(batch)-1) + "?"
		args := make([]interface{}, len(batch))
		for i, id := range batch {
			args[i] = id
		}
		rows, err := s.db.Query(`SELECT id, content FROM clips WHERE type = 'image' AND id IN (`+placeholders+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			var content string
			if err := rows.Scan(&id, &content); err != nil {
				rows.Close()
				return nil, err
			}
			result[id] = content
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return result, nil
}
