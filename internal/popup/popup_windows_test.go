package popup

import (
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

var (
	procGetPixel = syscall.NewLazyDLL("gdi32.dll").NewProc("GetPixel")
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
	var paints atomic.Int32
	debugPaint = func(rows, selected, firstVisible int, theme Theme) {
		paints.Add(1)
		t.Logf("paint#%d rows=%d selected=%d firstVisible=%d accent=%06x",
			paints.Load(), rows, selected, firstVisible, theme.Accent)
	}
	defer func() { debugPaint = nil }()
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
	w.Show(false)
	time.Sleep(300 * time.Millisecond) // let the first page load and paint
	w.Refresh()
	time.Sleep(100 * time.Millisecond)
	w.Hide()
	w.Show(false)
	time.Sleep(300 * time.Millisecond) // paint pass after re-show

	w.mu.Lock()
	rows, hwnd := len(w.rows), w.hwnd
	w.mu.Unlock()
	dc, _, _ := procGetWindowDC.Call(hwnd)
	background, _, _ := procGetPixel.Call(dc, 3, 3)
	selected, _, _ := procGetPixel.Call(dc, 2, uintptr(scale(82))) // first row fill, left of the text
	procDeleteDC.Call(dc)
	t.Logf("measured while visible: rows=%d bg=%06x sel=%06x", rows, background&0xFFFFFF, selected&0xFFFFFF)
	w.Hide()

	w.mu.Lock()
	visible := w.visible
	w.mu.Unlock()
	if rows != 3 {
		t.Errorf("expected 3 rows loaded, got %d", rows)
	}
	if visible {
		t.Error("window should be hidden after Hide")
	}
	// Paint must have produced the themed surface (regression: a WS_POPUP is
	// not invalidated by ShowWindow, which once left the window black).
	if background&0xFFFFFF != 0xFFFFFF {
		t.Errorf("background pixel = %06x, want ffffff (paint did not run?)", background)
	}
	if selected&0xFFFFFF != 0x0078D4 {
		t.Errorf("selected row pixel = %06x, want 0078d4", selected)
	}
	if pasted.Load() != 0 || pinned.Load() != 0 || deleted.Load() != 0 || details.Load() != 0 || settings.Load() != 0 {
		t.Error("callbacks fired without user input")
	}
}
