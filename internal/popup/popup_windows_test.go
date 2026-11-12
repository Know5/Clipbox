package popup

import (
	"sync/atomic"
	"testing"
	"time"
)

type stubSource struct{}

func (stubSource) Page(query, filter string, limit, offset int) ([]Row, int, error) {
	now := time.Now().UnixMilli()
	rows := []Row{
		{ID: 1, Kind: "text", Preview: "第一行文本 ClipBox", Timestamp: now},
		{ID: 2, Kind: "image", Preview: "截图示例", Timestamp: now - 60000},
		{ID: 3, Kind: "text", Preview: "git push origin main", Pinned: true, Timestamp: now - 120000},
	}
	if query != "" {
		rows = rows[:1]
	}
	return rows, len(rows), nil
}

// TestPopupLifecycle exercises window creation, the search edit, paint,
// show/hide and refresh without injecting input: callbacks must only fire in
// response to user actions.
func TestPopupLifecycle(t *testing.T) {
	var pasted, pinned, deleted, details, settings, filters atomic.Int32
	w, err := New(stubSource{}, Callbacks{
		Paste:     func(id int64, copyOnly bool) { pasted.Add(1) },
		TogglePin: func(id int64) { pinned.Add(1) },
		Delete:    func(id int64) { deleted.Add(1) },
		Details:   func(id int64) { details.Add(1) },
		Settings:  func() { settings.Add(1) },
		Filter:    func(f string) { filters.Add(1) },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	w.SetTheme(Theme{
		Background: 0x00FFFFFF, Foreground: 0x00111111, Muted: 0x00888888,
		Accent: 0x000078D4, AccentText: 0x00FFFFFF, Border: 0x00CCCCCC,
	})
	w.Show()
	time.Sleep(300 * time.Millisecond) // let the first page load and paint
	w.Refresh()
	time.Sleep(100 * time.Millisecond)
	w.Hide()
	w.Show()
	w.Hide()

	w.mu.Lock()
	rows, visible := len(w.rows), w.visible
	w.mu.Unlock()
	if rows != 3 {
		t.Errorf("expected 3 rows loaded, got %d", rows)
	}
	if visible {
		t.Error("window should be hidden after Hide")
	}
	if pasted.Load() != 0 || pinned.Load() != 0 || deleted.Load() != 0 || details.Load() != 0 || settings.Load() != 0 {
		t.Error("callbacks fired without user input")
	}
}
