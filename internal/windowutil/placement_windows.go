package windowutil

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	procMonitorFromPoint = user32.NewProc("MonitorFromPoint")
	procGetMonitorInfo   = user32.NewProc("GetMonitorInfoW")
	procGetWindowRect    = user32.NewProc("GetWindowRect")
	procFindWindowExW    = user32.NewProc("FindWindowExW")
	procSetWindowPos     = user32.NewProc("SetWindowPos")
)

const (
	monitorDefaultToNearest = 0x00000002
	// safeInset 让窗口离工作区边缘留一点空隙，避免贴边。
	safeInset int32 = 8

	swpNoSize     = 0x0001
	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010
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

// FindOwnWindow 返回本进程中窗口类为 className、标题为 title 的顶层窗口（找不到返回 0）。
// 只按标题找会命中别的进程的同名窗口，例如资源管理器打开了名为 ClipBox 的文件夹；
// 不比对窗口类会命中本进程里标题相同的对话框。
func FindOwnWindow(className, title string) uintptr {
	if className == "" || title == "" {
		return 0
	}
	classPtr, err := syscall.UTF16PtrFromString(className)
	if err != nil {
		return 0
	}
	titlePtr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return 0
	}
	self := uint32(os.Getpid())
	var hwnd uintptr
	for {
		hwnd, _, _ = procFindWindowExW.Call(0, hwnd, uintptr(unsafe.Pointer(classPtr)), uintptr(unsafe.Pointer(titlePtr)))
		if hwnd == 0 {
			return 0
		}
		var pid uint32
		procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid == self {
			return hwnd
		}
	}
}

// MoveWindow 把窗口左上角移到屏幕坐标 (x, y)（物理像素），不改尺寸和 Z 序，不激活。
// Wails 的 runtime.WindowSetPosition 把坐标当作相对窗口当前所在显示器工作区的偏移，
// 传绝对坐标时，任务栏在顶部/左侧或窗口在副屏上，会多挪出一个工作区原点。
func MoveWindow(hwnd uintptr, x, y int32) bool {
	return setWindowPos(hwnd, x, y, swpNoSize|swpNoZOrder|swpNoActivate)
}

func setWindowPos(hwnd uintptr, x, y int32, flags uintptr) bool {
	if hwnd == 0 {
		return false
	}
	ok, _, _ := procSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), 0, 0, flags)
	return ok != 0
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
