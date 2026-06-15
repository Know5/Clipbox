package windowutil

import (
	"path/filepath"
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
	swShow                          = 5
	swShowNoActivate                = 8
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

func FocusWindow(hwnd uintptr) {
	if hwnd == 0 {
		return
	}

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
		time.Sleep(20 * time.Millisecond)
	}

	procAllowSetForegroundWindow.Call(0xFFFFFFFF)
	procSetForeground.Call(hwnd)
	time.Sleep(60 * time.Millisecond)
}

func SendCtrlV() {
	inputs := []input{
		keyDown(vkControl),
		keyDown(vKeyV),
		keyUp(vKeyV),
		keyUp(vkControl),
	}

	procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		unsafe.Sizeof(inputs[0]),
	)
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
