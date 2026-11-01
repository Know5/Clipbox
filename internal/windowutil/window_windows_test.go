package windowutil

import (
	"testing"
	"time"
)

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
