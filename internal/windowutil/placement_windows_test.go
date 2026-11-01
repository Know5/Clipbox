package windowutil

import "testing"

func TestPlaceNearCursorPrefersBottomRight(t *testing.T) {
	wa := WorkArea{Left: 0, Top: 0, Right: 1920, Bottom: 1040}
	x, y := PlaceNearCursor(500, 300, 400, 560, wa, 10)
	if x != 510 || y != 310 {
		t.Fatalf("got (%d,%d), want (510,310)", x, y)
	}
}

func TestPlaceNearCursorClampsAtRightEdge(t *testing.T) {
	wa := WorkArea{Left: 0, Top: 0, Right: 1920, Bottom: 1040}
	x, y := PlaceNearCursor(1900, 300, 400, 560, wa, 10)
	if x+400 > wa.Right || x < wa.Left {
		t.Fatalf("x=%d escapes work area", x)
	}
	if y != 310 {
		t.Fatalf("y = %d, want 310", y)
	}
}

// 使用者报告的原始问题：鼠标靠近屏幕下方时窗口整个跑到屏幕外。
func TestPlaceNearCursorClampsAtBottomEdge(t *testing.T) {
	wa := WorkArea{Left: 0, Top: 0, Right: 1920, Bottom: 1040}
	x, y := PlaceNearCursor(500, 1030, 400, 560, wa, 10)
	if y+560 > wa.Bottom || y < wa.Top {
		t.Fatalf("y=%d escapes work area (window bottom %d > %d)", y, y+560, wa.Bottom)
	}
	if x != 510 {
		t.Fatalf("x = %d, want 510", x)
	}
}

func TestPlaceNearCursorBottomRightCorner(t *testing.T) {
	wa := WorkArea{Left: 0, Top: 0, Right: 1920, Bottom: 1040}
	x, y := PlaceNearCursor(1919, 1039, 400, 560, wa, 10)
	if x+400 > wa.Right || y+560 > wa.Bottom || x < wa.Left || y < wa.Top {
		t.Fatalf("window (%d,%d)+400x560 escapes work area", x, y)
	}
}

// 多屏：左侧副屏坐标是负的，不能被拉回主屏。
func TestPlaceNearCursorKeepsSecondaryMonitorWithNegativeX(t *testing.T) {
	wa := WorkArea{Left: -1920, Top: 0, Right: 0, Bottom: 1040}
	x, y := PlaceNearCursor(-1800, 300, 400, 560, wa, 10)
	if x < wa.Left || x+400 > wa.Right {
		t.Fatalf("x=%d escapes monitor %+v", x, wa)
	}
	if y != 310 {
		t.Fatalf("y = %d, want 310", y)
	}
}

// 窗口比工作区还大时贴左上角，不产生负坐标。
func TestPlaceNearCursorWindowLargerThanWorkArea(t *testing.T) {
	wa := WorkArea{Left: 0, Top: 0, Right: 300, Bottom: 300}
	x, y := PlaceNearCursor(150, 150, 400, 560, wa, 10)
	if x != wa.Left+safeInset || y != wa.Top+safeInset {
		t.Fatalf("got (%d,%d), want (%d,%d)", x, y, wa.Left+safeInset, wa.Top+safeInset)
	}
}

func TestMonitorWorkAreaForTopLeft(t *testing.T) {
	wa, ok := MonitorWorkArea(0, 0)
	if !ok {
		t.Fatal("MonitorWorkArea(0,0) failed")
	}
	if wa.Width() <= 0 || wa.Height() <= 0 {
		t.Fatalf("bad work area: %+v", wa)
	}
}
