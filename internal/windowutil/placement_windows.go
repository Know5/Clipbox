package windowutil

import (
	"syscall"
	"unsafe"
)

var (
	procMonitorFromPoint = user32.NewProc("MonitorFromPoint")
	procGetMonitorInfo   = user32.NewProc("GetMonitorInfoW")
	procGetWindowRect    = user32.NewProc("GetWindowRect")
	procFindWindowW      = user32.NewProc("FindWindowW")
)

const (
	monitorDefaultToNearest = 0x00000002
	// safeInset 让窗口离工作区边缘留一点空隙，避免贴边。
	safeInset int32 = 8
)

// WorkArea 是显示器工作区（排除任务栏），使用屏幕物理像素坐标。
type WorkArea struct {
	Left, Top, Right, Bottom int32
}

func (w WorkArea) Width() int32  { return w.Right - w.Left }
func (w WorkArea) Height() int32 { return w.Bottom - w.Top }

type monitorInfo struct {
	cbSize    uint32
	rcMonitor rect32
	rcWork    rect32
	dwFlags   uint32
}

type rect32 struct {
	left, top, right, bottom int32
}

type point32 struct {
	x, y int32
}

// MonitorWorkArea 返回屏幕坐标 (x,y) 所在显示器的可用工作区。
// MONITOR_DEFAULTTONEAREST：坐标落在多屏拼接的空隙时取最近的一块。
func MonitorWorkArea(x, y int32) (WorkArea, bool) {
	hmon, _, _ := procMonitorFromPoint.Call(
		uintptr(unsafe.Pointer(&point32{x: x, y: y})),
		monitorDefaultToNearest,
	)
	if hmon == 0 {
		return WorkArea{}, false
	}
	info := monitorInfo{cbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
	if ok, _, _ := procGetMonitorInfo.Call(hmon, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return WorkArea{}, false
	}
	return WorkArea{
		Left:   info.rcWork.left,
		Top:    info.rcWork.top,
		Right:  info.rcWork.right,
		Bottom: info.rcWork.bottom,
	}, true
}

// WindowSize 返回窗口矩形尺寸（物理像素）。
func WindowSize(hwnd uintptr) (int32, int32, bool) {
	if hwnd == 0 {
		return 0, 0, false
	}
	var r rect32
	if ok, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); ok == 0 {
		return 0, 0, false
	}
	w, h := r.right-r.left, r.bottom-r.top
	if w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

// FindWindowByTitle 按标题查找顶层窗口（找不到返回 0）。
func FindWindowByTitle(title string) uintptr {
	if title == "" {
		return 0
	}
	ptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return 0
	}
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(ptr)))
	return hwnd
}

// PlaceNearCursor 计算窗口左上角坐标：放在光标右下方 offset 处，
// 但整块窗口必须留在工作区内（右下越界就收边），窗口比工作区还大时贴左上。
func PlaceNearCursor(cursorX, cursorY, winW, winH int32, wa WorkArea, offset int32) (int32, int32) {
	if offset < 0 {
		offset = 0
	}
	x := cursorX + offset
	y := cursorY + offset
	if x+winW > wa.Right-safeInset {
		x = wa.Right - safeInset - winW
	}
	if y+winH > wa.Bottom-safeInset {
		y = wa.Bottom - safeInset - winH
	}
	if x < wa.Left+safeInset {
		x = wa.Left + safeInset
	}
	if y < wa.Top+safeInset {
		y = wa.Top + safeInset
	}
	return x, y
}
