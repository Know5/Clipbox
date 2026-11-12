package storage

import (
	"fmt"
	"testing"

	"clipbox/internal/clipboard"
)

func TestTagAggregationAndFiltersUseTheSameCaseRules(t *testing.T) {
	for _, labels := range [][2]string{{"Work", "work"}, {"Ärger", "ärger"}, {"Работа", "работа"}} {
		t.Run(labels[0], func(t *testing.T) {
			store := newTestStore(t)
			for index, tag := range labels {
				if _, err := store.SaveOrUpdate(clipboard.ClipEntry{Type: "text", Content: fmt.Sprintf("entry %d", index), Preview: "entry", Timestamp: int64(index + 1), Tags: tag}); err != nil {
					t.Fatal(err)
				}
			}
			tags, err := store.ListTags("", "all")
			if err != nil || len(tags) != 1 || tags[0].Count != 2 {
				t.Fatalf("tag aggregation = %+v err=%v", tags, err)
			}
			for _, label := range labels {
				count, err := store.CountByTypeAndTag("all", label)
				if err != nil || count != tags[0].Count {
					t.Fatalf("filter %q count=%d, dropdown=%d err=%v", label, count, tags[0].Count, err)
				}
				rows, err := store.GetByTypeAndTag("all", label, 50, 0)
				if err != nil || len(rows) != 2 {
					t.Fatalf("filter %q rows=%d err=%v", label, len(rows), err)
				}
				count, err = store.CountSearchByTypeAndTag("entry", "all", label)
				if err != nil || count != 2 {
					t.Fatalf("search filter %q count=%d err=%v", label, count, err)
				}
				rows, err = store.SearchByTypeAndTag("entry", "all", label, 50, 0)
				if err != nil || len(rows) != 2 {
					t.Fatalf("search filter %q rows=%d err=%v", label, len(rows), err)
				}
			}
		})
	}
}
