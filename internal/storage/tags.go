package storage

import (
	"database/sql"
	"slices"
	"strings"

	"clipbox/internal/clipboard"

	_ "modernc.org/sqlite"
)

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
	condition := `instr(', ' || clipbox_tag_key(tags) || ', ', ?) > 0`
	args = append(args, ", "+strings.ToLower(tag)+", ")
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
		rows, err = s.db.Query(`SELECT tags, COUNT(*) FROM clips `+where+` GROUP BY tags`, args...)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := map[string]int{}
	names := map[string]string{}
	for rows.Next() {
		var raw string
		var occurrences int
		if err := rows.Scan(&raw, &occurrences); err != nil {
			return nil, err
		}
		for _, tag := range splitNormalizedTags(raw) {
			key := strings.ToLower(tag)
			counts[key] += occurrences
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
	type keyedTag struct {
		value TagCount
		key   string
	}
	keyed := make([]keyedTag, len(tags))
	for i, tag := range tags {
		keyed[i] = keyedTag{value: tag, key: strings.ToLower(tag.Name)}
	}
	slices.SortFunc(keyed, func(a, b keyedTag) int {
		if a.value.Count > b.value.Count {
			return -1
		}
		if a.value.Count < b.value.Count {
			return 1
		}
		if order := strings.Compare(a.key, b.key); order != 0 {
			return order
		}
		return strings.Compare(a.value.Name, b.value.Name)
	})
	for i := range keyed {
		tags[i] = keyed[i].value
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
