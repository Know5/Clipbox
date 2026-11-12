package windowutil

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32                       = syscall.NewLazyDLL("user32.dll")
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procGetForeground            = user32.NewProc("GetForegroundWindow")
	procSetForeground            = user32.NewProc("SetForegroundWindow")
	procShowWindow               = user32.NewProc("ShowWindow")
	procIsIconic                 = user32.NewProc("IsIconic")
	procSendInput                = user32.NewProc("SendInput")
	procGetAsyncKeyState         = user32.NewProc("GetAsyncKeyState")
	procGetWindowTextLength      = user32.NewProc("GetWindowTextLengthW")
	procGetWindowText            = user32.NewProc("GetWindowTextW")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procGetCurrentThreadId       = kernel32.NewProc("GetCurrentThreadId")
	procAttachThreadInput        = user32.NewProc("AttachThreadInput")
	procAllowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")
	procOpenProcess              = kernel32.NewProc("OpenProcess")
	procCloseHandle              = kernel32.NewProc("CloseHandle")
	procQueryFullProcessImage    = kernel32.NewProc("QueryFullProcessImageNameW")
)

const (
	swRestore                       = 9
	inputKeyboard                   = 1
	keyeventfKeyUp                  = 0x0002
	vkControl                       = 0x11
	vKeyV                           = 0x56
	processQueryLimitedInformation  = 0x1000
	queryProcessImageNameBufferSize = 4096
)

type ForegroundInfo struct {
	HWND        uintptr
	ProcessID   uint32
	ProcessPath string
	ProcessName string
	Title       string
}

type keyboardInput struct {
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

type input struct {
	inputType uint32
	_         uint32 // Padding for 64-bit alignment
	ki        keyboardInput
	_         [8]byte // Padding to make total size 40 bytes on x64
}

func ForegroundWindow() uintptr {
	hwnd, _, _ := procGetForeground.Call()
	return hwnd
}

func CurrentForegroundInfo() ForegroundInfo {
	hwnd := ForegroundWindow()
	return WindowInfo(hwnd)
}

func WindowInfo(hwnd uintptr) ForegroundInfo {
	if hwnd == 0 {
		return ForegroundInfo{}
	}

	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))

	info := ForegroundInfo{
		HWND:      hwnd,
		ProcessID: pid,
		Title:     windowText(hwnd),
	}
	if pid != 0 {
		info.ProcessPath = processImagePath(pid)
		if info.ProcessPath != "" {
			info.ProcessName = filepath.Base(info.ProcessPath)
		}
	}
	return info
}

func windowText(hwnd uintptr) string {
	length, _, _ := procGetWindowTextLength.Call(hwnd)
	if length == 0 {
		return ""
	}
	buf := make([]uint16, int(length)+1)
	written, _, _ := procGetWindowText.Call(
		hwnd,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if written == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:written])
}

func processImagePath(pid uint32) string {
	handle, _, _ := procOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if handle == 0 {
		return ""
	}
	defer procCloseHandle.Call(handle)

	buf := make([]uint16, queryProcessImageNameBufferSize)
	size := uint32(len(buf))
	ret, _, _ := procQueryFullProcessImage.Call(
		handle,
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if ret == 0 || size == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:size])
}

// focusPollInterval 和 focusTimeout 控制等待前台切换的节奏。
// 用轮询代替固定 Sleep:焦点一切过去就返回,最多等 focusTimeout。
const (
	focusPollInterval    = 5 * time.Millisecond
	focusTimeout         = 100 * time.Millisecond
	pasteModifierTimeout = 500 * time.Millisecond
)

var pasteMu sync.Mutex

// FocusWindow 尝试把 hwnd 切换为前台窗口,并确认切换成功。
// 返回 false 表示超时仍未获得焦点(例如目标是管理员窗口,UIPI 阻止了切换)。
func FocusWindow(hwnd uintptr) bool {
	if hwnd == 0 {
		return false
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	currThread, _, _ := procGetCurrentThreadId.Call()
	targetThread, _, _ := procGetWindowThreadProcessId.Call(hwnd, 0)

	if currThread != targetThread {
		procAttachThreadInput.Call(currThread, targetThread, 1)
		defer procAttachThreadInput.Call(currThread, targetThread, 0)
	}

	// Only restore the window if it's minimized (iconic).
	// If it's maximized or fullscreen, leave it alone — calling SW_RESTORE
	// would un-maximize it, pulling the browser out of fullscreen.
	iconic, _, _ := procIsIconic.Call(hwnd)
	if iconic != 0 {
		procShowWindow.Call(hwnd, swRestore)
	}

	procAllowSetForegroundWindow.Call(0xFFFFFFFF)
	procSetForeground.Call(hwnd)

	return waitForForeground(hwnd, focusTimeout, focusPollInterval, ForegroundWindow)
}

// waitForForeground 轮询 current 直到返回 hwnd,或超过 timeout。
// current 作为参数传入,便于单元测试超时和成功两个分支。
func waitForForeground(hwnd uintptr, timeout, interval time.Duration, current func() uintptr) bool {
	deadline := time.Now().Add(timeout)
	for {
		if current() == hwnd {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(interval)
	}
}

// PasteToWindow waits for the user's shortcut to be released before moving
// focus. In particular, Alt+1 must not turn the injected paste into Ctrl+Alt+V
// or send Alt's release to the target window's menu bar.
func PasteToWindow(hwnd uintptr) error {
	pasteMu.Lock()
	defer pasteMu.Unlock()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	return pasteToWindow(hwnd, pasteModifierTimeout, focusPollInterval, pasteModifiersPressed, FocusWindow, func() error {
		if ForegroundWindow() != hwnd {
			return fmt.Errorf("目标窗口已失去焦点，内容已在剪贴板")
		}
		return SendCtrlV()
	})
}

func pasteToWindow(hwnd uintptr, timeout, interval time.Duration, modifiersPressed func() bool, focus func(uintptr) bool, send func() error) error {
	if hwnd == 0 {
		return fmt.Errorf("没有可用的目标窗口，内容已在剪贴板")
	}
	deadline := time.Now().Add(timeout)
	for modifiersPressed() {
		if !time.Now().Before(deadline) {
			return fmt.Errorf("快捷键尚未释放，内容已在剪贴板，请松开按键后按 Ctrl+V")
		}
		time.Sleep(interval)
	}
	if !focus(hwnd) {
		return fmt.Errorf("目标窗口未获得焦点，内容已在剪贴板")
	}
	if modifiersPressed() {
		return fmt.Errorf("检测到新的修饰键按下，内容已在剪贴板，未发送粘贴按键")
	}
	return send()
}

func pasteModifiersPressed() bool {
	for _, key := range []uintptr{0x10, 0x11, 0x12, 0x5B, 0x5C} { // Shift, Ctrl, Alt, left/right Win
		state, _, _ := procGetAsyncKeyState.Call(key)
		if state&0x8000 != 0 {
			return true
		}
	}
	return false
}

// SendCtrlV only injects a plain Ctrl+V. A modifier pressed after the focus
// check aborts the operation rather than sending a different shortcut.
func SendCtrlV() error {
	if pasteModifiersPressed() {
		return fmt.Errorf("修饰键尚未释放，未发送粘贴按键")
	}
	inputs := []input{
		keyDown(vkControl),
		keyDown(vKeyV),
		keyUp(vKeyV),
		keyUp(vkControl),
	}

	sent, _, _ := procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		unsafe.Sizeof(inputs[0]),
	)
	if int(sent) != len(inputs) {
		return fmt.Errorf("SendInput 只注入了 %d/%d 个事件", sent, len(inputs))
	}
	return nil
}

func keyDown(key uint16) input {
	return input{
		inputType: inputKeyboard,
		ki: keyboardInput{
			wVk: key,
		},
	}
}

func keyUp(key uint16) input {
	return input{
		inputType: inputKeyboard,
		ki: keyboardInput{
			wVk:     key,
			dwFlags: keyeventfKeyUp,
		},
	}
}
