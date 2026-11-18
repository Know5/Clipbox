package storage

import (
	"database/sql"
	"fmt"
	"strings"

	"clipbox/internal/clipboard"

	_ "modernc.org/sqlite"
)

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
		// Retire the old trigger, which rebuilt the whole text index even for
		// timestamp-only deduplication and pin changes.
		`DROP TRIGGER IF EXISTS clips_fts_au`,
		`CREATE TRIGGER IF NOT EXISTS clips_fts_au AFTER UPDATE OF type, content, source_app, source_title, note, tags ON clips
		 WHEN old.type IS NOT new.type OR old.content IS NOT new.content
		   OR old.source_app IS NOT new.source_app OR old.source_title IS NOT new.source_title
		   OR old.note IS NOT new.note OR old.tags IS NOT new.tags BEGIN
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
	if err := s.syncSearchIndex(); err != nil {
		s.ftsAvailable = false
		return
	}
	s.ftsAvailable = true
}

// syncSearchIndex inserts only rows missing from the FTS index.
// Called on every startup - fast incremental, never a full rebuild.
func (s *Store) syncSearchIndex() error {
	_, err := s.db.Exec(`
		INSERT INTO clips_fts(rowid, content, source_app, source_title, note, tags)
		SELECT id,
		       CASE WHEN type = 'text' THEN content ELSE '' END,
		       source_app,
		       source_title,
		       note,
		       tags
		  FROM clips
		 WHERE id NOT IN (SELECT rowid FROM clips_fts)
	`)
	return err
}

// rebuildSearchIndex does a full FTS5 rebuild. Used only by manual
// repair storage - user-triggered, expected to be slow.
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
		`SELECT `+clipListColumns+`
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
		`SELECT `+clipListColumns+`
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

func (s *Store) SearchPage(query string, limit, offset int) ([]clipboard.ClipEntry, int, error) {
	return s.SearchPageByTypeAndTag(query, "text", "", limit, offset)
}

func (s *Store) SearchPageByType(query, clipType string, limit, offset int) ([]clipboard.ClipEntry, int, error) {
	return s.SearchPageByTypeAndTag(query, clipType, "", limit, offset)
}

func (s *Store) SearchPageByTypeAndTag(query, clipType, tag string, limit, offset int) ([]clipboard.ClipEntry, int, error) {
	tag = normalizeTagFilter(tag)
	if entries, total, ok, err := s.searchFTSPage(query, clipType, tag, limit, offset); ok {
		return entries, total, err
	}
	clipType = normalizeClipType(clipType)
	limit, offset = normalizePage(limit, offset)
	where, args := searchWhere(query, clipType)
	where, args = appendTagWhere(where, args, tag)
	args = append(args, limit, offset)
	rows, err := s.db.Query(
		`SELECT `+clipListColumns+`, COUNT(*) OVER() AS total_count
		 FROM clips `+where+`
		 ORDER BY pinned DESC, timestamp DESC
		 LIMIT ? OFFSET ?`,
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	entries, total, err := scanClipPageRows(rows)
	if err != nil {
		return nil, 0, err
	}
	if len(entries) == 0 && offset > 0 {
		total, err = s.CountSearchByTypeAndTag(query, clipType, tag)
	}
	return entries, total, err
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

func (s *Store) searchFTSPage(query, clipType, tag string, limit, offset int) ([]clipboard.ClipEntry, int, bool, error) {
	if !s.shouldUseFTS(query) {
		return nil, 0, false, nil
	}
	limit, offset = normalizePage(limit, offset)
	where, args, ok := ftsWhere(query, clipType, tag)
	if !ok {
		return nil, 0, false, nil
	}
	args = append(args, limit, offset)
	rows, err := s.db.Query(
		`SELECT `+qualifiedClipColumns("c")+`, COUNT(*) OVER() AS total_count
		   FROM clips c
		   JOIN clips_fts ON clips_fts.rowid = c.id
		  WHERE `+where+`
		  ORDER BY c.pinned DESC, c.timestamp DESC
		  LIMIT ? OFFSET ?`,
		args...,
	)
	if err != nil {
		return nil, 0, false, nil
	}
	defer rows.Close()
	entries, total, err := scanClipPageRows(rows)
	if err != nil {
		return nil, 0, true, err
	}
	if len(entries) == 0 && offset > 0 {
		total, err = s.CountSearchByTypeAndTag(query, clipType, tag)
	}
	return entries, total, true, err
}

func scanClipPageRows(rows *sql.Rows) ([]clipboard.ClipEntry, int, error) {
	var entries []clipboard.ClipEntry
	total := 0
	for rows.Next() {
		var e clipboard.ClipEntry
		var count int
		if err := rows.Scan(&e.ID, &e.Type, &e.Content, &e.Preview, &e.Thumbnail, &e.Timestamp, &e.Pinned, &e.SourceApp, &e.SourceTitle, &e.SourcePath, &e.Note, &e.Tags, &count); err != nil {
			return nil, 0, err
		}
		if e.Type == "image" {
			e.Content = ""
		}
		total = count
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return entries, total, nil
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
		`SELECT c.tags, COUNT(*)
		   FROM clips c
		   JOIN clips_fts ON clips_fts.rowid = c.id
		  WHERE `+where+` GROUP BY c.tags`,
		args...,
	)
}

func (s *Store) shouldUseFTS(query string) bool {
	if !s.ftsAvailable {
		return false
	}
	ftsTokens, _ := partitionSearchTokens(query)
	return len(ftsTokens) > 0
}

func splitSearchTokens(query string) []string {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return []string{query}
	}
	return strings.Fields(trimmed)
}

func partitionSearchTokens(query string) (ftsTokens, shortTokens []string) {
	for _, token := range splitSearchTokens(query) {
		if len([]rune(token)) >= 3 {
			ftsTokens = append(ftsTokens, token)
		} else if token != "" {
			shortTokens = append(shortTokens, token)
		}
	}
	return ftsTokens, shortTokens
}

func ftsWhere(query, clipType, tag string) (string, []interface{}, bool) {
	ftsTokens, shortTokens := partitionSearchTokens(query)
	if len(ftsTokens) == 0 {
		return "", nil, false
	}
	quoted := make([]string, len(ftsTokens))
	for i, token := range ftsTokens {
		quoted[i] = `"` + strings.ReplaceAll(token, `"`, `""`) + `"`
	}
	conditions := []string{"clips_fts MATCH ?"}
	args := []interface{}{strings.Join(quoted, " ")}
	normalizedType := normalizeClipType(clipType)
	switch normalizedType {
	case "text", "image":
		conditions = append(conditions, "c.type = ?")
		args = append(args, normalizedType)
	}
	for _, token := range shortTokens {
		clause, tokenArgs := tokenLikeClause("c.", normalizedType, token)
		conditions = append(conditions, clause)
		args = append(args, tokenArgs...)
	}
	if tag = normalizeTagFilter(tag); tag != "" {
		conditions = append(conditions, `instr(', ' || clipbox_tag_key(c.tags) || ', ', ?) > 0`)
		args = append(args, ", "+strings.ToLower(tag)+", ")
	}
	return strings.Join(conditions, " AND "), args, true
}

func searchWhere(query, clipType string) (string, []interface{}) {
	normalizedType := normalizeClipType(clipType)
	var conditions []string
	var args []interface{}
	switch normalizedType {
	case "text", "image":
		conditions = append(conditions, "type = ?")
		args = append(args, normalizedType)
	}
	for _, token := range splitSearchTokens(query) {
		clause, tokenArgs := tokenLikeClause("", normalizedType, token)
		conditions = append(conditions, clause)
		args = append(args, tokenArgs...)
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

func tokenLikeClause(alias, clipType, token string) (string, []interface{}) {
	pattern := likePattern(token)
	const escapeClause = ` ESCAPE '\'`
	col := func(name string) string { return alias + name }
	switch clipType {
	case "text":
		return "(" + col("content") + ` LIKE ?` + escapeClause +
				" OR " + col("source_app") + ` LIKE ?` + escapeClause +
				" OR " + col("source_title") + ` LIKE ?` + escapeClause +
				" OR " + col("note") + ` LIKE ?` + escapeClause +
				" OR " + col("tags") + ` LIKE ?` + escapeClause + ")",
			[]interface{}{pattern, pattern, pattern, pattern, pattern}
	case "image":
		return "(" + col("source_app") + ` LIKE ?` + escapeClause +
				" OR " + col("source_title") + ` LIKE ?` + escapeClause +
				" OR " + col("note") + ` LIKE ?` + escapeClause +
				" OR " + col("tags") + ` LIKE ?` + escapeClause + ")",
			[]interface{}{pattern, pattern, pattern, pattern}
	default:
		return "((" + col("type") + ` = 'text' AND ` + col("content") + ` LIKE ?` + escapeClause + ")" +
				" OR " + col("source_app") + ` LIKE ?` + escapeClause +
				" OR " + col("source_title") + ` LIKE ?` + escapeClause +
				" OR " + col("note") + ` LIKE ?` + escapeClause +
				" OR " + col("tags") + ` LIKE ?` + escapeClause + ")",
			[]interface{}{pattern, pattern, pattern, pattern, pattern}
	}
}

func likePattern(query string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(query)
	return "%" + escaped + "%"
}
