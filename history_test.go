package main

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"clipbox/internal/clipboard"
)

func TestHistoryMutationsPublishCommittedState(t *testing.T) {
	app := newTestAppWithStore(t)
	var events []ClipHistoryChange
	app.historyChanged = func(change ClipHistoryChange) { events = append(events, change) }
	captured, err := app.persistCapturedClip(clipboard.ClipEntry{Type: "text", Content: "history", Preview: "history", Timestamp: 1})
	if err != nil {
		t.Fatal(err)
	}
	id := captured.Entry.ID
	if err := app.TogglePin(id); err != nil {
		t.Fatal(err)
	}
	if _, err := app.UpdateClipMetadata(id, "saved note", "Work"); err != nil {
		t.Fatal(err)
	}
	if err := app.DeleteClip(id); err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Fatalf("history mutations emitted %d events, want 4", len(events))
	}
	if events[1].Entry == nil || !events[1].Entry.Pinned || events[1].Total != 1 {
		t.Fatalf("pin event does not reflect the database: %+v", events[1])
	}
	metadata := events[2].Entry
	if metadata == nil || metadata.Note != "saved note" || metadata.Tags != "Work" || !metadata.Pinned {
		t.Fatalf("metadata event does not reflect the database: %+v", metadata)
	}
	deleted := events[3]
	if deleted.Entry != nil || deleted.Total != 0 || len(deleted.DeletedIDs) != 1 || deleted.DeletedIDs[0] != id {
		t.Fatalf("delete event does not reflect the database: %+v", deleted)
	}
}

func TestFailedHistoryMutationDoesNotEmitChange(t *testing.T) {
	app := newTestAppWithStore(t)
	id, err := app.store.SaveOrUpdate(clipboard.ClipEntry{Type: "text", Content: "keep", Preview: "keep", Timestamp: 1})
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(app.dataDir, "clipbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER fail_history_delete BEFORE DELETE ON clips BEGIN SELECT RAISE(ABORT, 'delete failed'); END`); err != nil {
		t.Fatal(err)
	}
	var events []ClipHistoryChange
	app.historyChanged = func(change ClipHistoryChange) { events = append(events, change) }
	if err := app.DeleteClip(id); err == nil {
		t.Fatal("expected the injected deletion error")
	}
	if len(events) != 0 {
		t.Fatalf("failed mutation emitted an event: %+v", events)
	}
}

func TestConcurrentHistoryMutationsEmitOrderedTotals(t *testing.T) {
	app := newTestAppWithStore(t)
	var events []ClipHistoryChange
	app.historyChanged = func(change ClipHistoryChange) { events = append(events, change) }
	var workers sync.WaitGroup
	for index := 0; index < 20; index++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			text := fmt.Sprintf("concurrent history %d", index)
			change, err := app.persistCapturedClip(clipboard.ClipEntry{Type: "text", Content: text, Preview: text, Timestamp: int64(index + 1)})
			if err != nil {
				t.Errorf("capture: %v", err)
				return
			}
			if err := app.DeleteClip(change.Entry.ID); err != nil {
				t.Errorf("delete: %v", err)
			}
		}(index)
	}
	workers.Wait()
	if len(events) != 40 {
		t.Fatalf("emitted %d events, want 40", len(events))
	}
	present := make(map[int64]bool)
	for index, event := range events {
		if event.Entry != nil {
			present[event.Entry.ID] = true
		}
		for _, id := range event.DeletedIDs {
			delete(present, id)
		}
		if event.Total != len(present) {
			t.Fatalf("event %d published an out-of-order total: got %d, want %d", index, event.Total, len(present))
		}
	}
}
