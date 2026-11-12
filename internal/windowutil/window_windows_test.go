package windowutil

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPasteWaitsForModifiersBeforeChangingFocus(t *testing.T) {
	checks := 0
	var actions []string
	err := pasteToWindow(42, time.Second, time.Millisecond, func() bool {
		checks++
		return checks < 3
	}, func(hwnd uintptr) bool {
		if checks < 3 || hwnd != 42 {
			t.Fatal("focus changed before the user's shortcut was released")
		}
		actions = append(actions, "focus")
		return true
	}, func() error {
		actions = append(actions, "paste")
		return nil
	})
	if err != nil || strings.Join(actions, ",") != "focus,paste" {
		t.Fatalf("actions=%v err=%v", actions, err)
	}
}

func TestPasteDoesNotInjectWhenShortcutRemainsHeld(t *testing.T) {
	err := pasteToWindow(42, 5*time.Millisecond, time.Millisecond, func() bool { return true }, func(uintptr) bool {
		t.Fatal("must not move focus while Alt/Ctrl/Shift/Win is held")
		return false
	}, func() error {
		t.Fatal("must not inject a modified Ctrl+V")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "尚未释放") {
		t.Fatalf("expected the release timeout, got %v", err)
	}
}

func TestPasteAbortsIfModifierIsPressedDuringFocusChange(t *testing.T) {
	pressed := false
	err := pasteToWindow(42, time.Second, time.Millisecond, func() bool { return pressed }, func(uintptr) bool {
		pressed = true
		return true
	}, func() error {
		t.Fatal("must not send paste after a new modifier is pressed")
		return nil
	})
	if err == nil {
		t.Fatal("expected an error after the keyboard state changed")
	}
}

func TestPasteReportsFocusAndInjectionFailures(t *testing.T) {
	if err := pasteToWindow(42, time.Second, time.Millisecond, func() bool { return false }, func(uintptr) bool { return false }, func() error {
		t.Fatal("must not inject into an unfocused target")
		return nil
	}); err == nil {
		t.Fatal("expected a focus error")
	}
	want := errors.New("input blocked")
	err := pasteToWindow(42, time.Second, time.Millisecond, func() bool { return false }, func(uintptr) bool { return true }, func() error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("injection error = %v, want %v", err, want)
	}
}

func TestWaitForForegroundSucceedsWhenFocusArrives(t *testing.T) {
	calls := 0
	current := func() uintptr {
		calls++
		if calls < 3 {
			return 1 // 旧前台窗口
		}
		return 42 // 目标窗口
	}
	if !waitForForeground(42, 100*time.Millisecond, time.Millisecond, current) {
		t.Fatal("expected true when focus arrives before timeout")
	}
}

func TestWaitForForegroundTimesOut(t *testing.T) {
	start := time.Now()
	ok := waitForForeground(42, 30*time.Millisecond, time.Millisecond, func() uintptr { return 1 })
	if ok {
		t.Fatal("expected false when focus never moves to target")
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("timeout took too long: %v", elapsed)
	}
}
