package storage

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"clipbox/internal/clipboard"
)

func TestListQueriesOmitBodiesButDetailsAndSearchKeepFullText(t *testing.T) {
	store := newTestStore(t)
	body := strings.Repeat("long source ", 10000) + "hidden-needle"
	id, err := store.SaveOrUpdate(clipboard.ClipEntry{Type: "text", Content: body, Preview: "short preview", Timestamp: 1, Tags: "Work"})
	if err != nil {
		t.Fatal(err)
	}
	queries := []func() ([]clipboard.ClipEntry, error){
		func() ([]clipboard.ClipEntry, error) { return store.GetAll(50, 0) },
		func() ([]clipboard.ClipEntry, error) { return store.GetByTypeAndTag("text", "Work", 50, 0) },
		func() ([]clipboard.ClipEntry, error) {
			return store.SearchByTypeAndTag("hidden-needle", "all", "Work", 50, 0)
		},
		func() ([]clipboard.ClipEntry, error) { return store.SearchByType("ce", "text", 50, 0) },
	}
	for index, query := range queries {
		entries, err := query()
		if err != nil || len(entries) != 1 || entries[0].ID != id || entries[0].Content != "" || entries[0].Preview != "short preview" {
			t.Fatalf("list query %d = %+v, %v", index, entries, err)
		}
		data, _ := json.Marshal(entries)
		if len(data) > 1000 {
			t.Fatalf("list payload is too large: %d", len(data))
		}
	}
	entry, err := store.GetByID(id)
	if err != nil || entry.Content != body {
		t.Fatal("list projection changed stored text")
	}
}

func TestListQueriesUseOrderingIndexes(t *testing.T) {
	store := newTestStore(t)
	for _, query := range []string{
		`SELECT ` + clipListColumns + ` FROM clips ORDER BY pinned DESC, timestamp DESC LIMIT 50`,
		`SELECT ` + clipListColumns + ` FROM clips WHERE type = 'image' ORDER BY pinned DESC, timestamp DESC LIMIT 50`,
	} {
		rows, err := store.db.Query("EXPLAIN QUERY PLAN " + query)
		if err != nil {
			t.Fatal(err)
		}
		var plan string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			plan += detail
		}
		rows.Close()
		if !strings.Contains(plan, "idx_clips_") || strings.Contains(plan, "TEMP B-TREE") {
			t.Fatalf("list sort did not use its ordering index: %s", plan)
		}
	}
}

func TestTimestampAndPinUpdatesDoNotRewriteFTS(t *testing.T) {
	store := newTestStore(t)
	if !store.ftsAvailable {
		t.Skip("FTS unavailable")
	}
	id, err := store.SaveOrUpdate(clipboard.ClipEntry{Type: "text", Content: "original searchable", Preview: "text", Timestamp: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TABLE fts_writes(count INTEGER); INSERT INTO fts_writes VALUES(0);
	 CREATE TRIGGER track_fts_write AFTER INSERT ON clips_fts_content BEGIN UPDATE fts_writes SET count = count + 1; END`); err != nil {
		t.Fatal(err)
	}
	if err := store.TogglePin(id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveOrUpdate(clipboard.ClipEntry{Type: "text", Content: "original searchable", Preview: "text", Timestamp: 2}); err != nil {
		t.Fatal(err)
	}
	var writes int
	if err := store.db.QueryRow("SELECT count FROM fts_writes").Scan(&writes); err != nil || writes != 0 {
		t.Fatalf("timestamp/pin caused FTS writes: %d, %v", writes, err)
	}
	if _, err := store.UpdateMetadata(id, "updated searchable memo", "Work"); err != nil {
		t.Fatal(err)
	}
	entries, err := store.SearchByType("updated searchable memo", "all", 50, 0)
	if err != nil || len(entries) != 1 || entries[0].ID != id {
		t.Fatal("search index was not updated after editing metadata")
	}
}

func TestTagSortIsDeterministicWithUnicodeAndCounts(t *testing.T) {
	tags := []TagCount{{Name: "z", Count: 1}, {Name: "Ärger", Count: 2}, {Name: "A", Count: 1}, {Name: "a", Count: 1}, {Name: "工作", Count: 3}}
	sortTagCounts(tags)
	if fmt.Sprint(tags) != "[{工作 3} {Ärger 2} {A 1} {a 1} {z 1}]" {
		t.Fatalf("unexpected tag order: %+v", tags)
	}
}

func BenchmarkHistoryPage(b *testing.B) {
	store, err := NewStore(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	tx, _ := store.db.Begin()
	statement, _ := tx.Prepare(`INSERT INTO clips(type, content, preview, timestamp, pinned, content_hash, tags) VALUES ('text', ?, ?, ?, 0, ?, 'Work')`)
	for index := 0; index < 10000; index++ {
		preview := fmt.Sprintf("record %05d needle", index)
		body := preview + strings.Repeat("x", 512)
		if index >= 9995 {
			body = preview + strings.Repeat("x", 512*1024)
		}
		if _, err := statement.Exec(body, preview, index, hashContent(body)); err != nil {
			b.Fatal(err)
		}
	}
	statement.Close()
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
	b.Run("FirstPage", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			entries, err := store.GetAll(50, 0)
			if err != nil || len(entries) != 50 {
				b.Fatal(err)
			}
		}
	})
	b.Run("SearchAndCount", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := store.SearchByTypeAndTag("needle", "all", "", 50, 0); err != nil {
				b.Fatal(err)
			}
			if _, err := store.CountSearchByTypeAndTag("needle", "all", ""); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkTagSort(b *testing.B) {
	for _, count := range []int{1000, 5000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			base := make([]TagCount, count)
			for i := range base {
				base[i] = TagCount{Name: fmt.Sprintf("Tag-%05d", count-i), Count: 1}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				sortTagCounts(append([]TagCount(nil), base...))
			}
		})
	}
}
