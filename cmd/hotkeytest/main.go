package main

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procRegisterHotKey   = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey = user32.NewProc("UnregisterHotKey")
	procGetMessage       = user32.NewProc("GetMessageW")
)

const (
	wmHotkey    = 0x0312
	modControl  = 0x0002
	modAlt      = 0x0001
	modNoRepeat = 0x4000
)

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

func main() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	fmt.Println("=== 热键测试工具 ===")

	// 测试 Ctrl+Alt+V
	ret, _, err := procRegisterHotKey.Call(0, 1, modControl|modAlt|modNoRepeat, 0x56)
	if ret == 0 {
		ret2, _, err2 := procRegisterHotKey.Call(0, 1, modControl|modAlt, 0x56)
		if ret2 == 0 {
			fmt.Printf("Ctrl+Alt+V 注册失败 (norepeat: %v, fallback: %v)\n", err, err2)
		} else {
			fmt.Println("Ctrl+Alt+V 注册成功 (无 NOREPEAT)")
		}
	} else {
		fmt.Println("Ctrl+Alt+V 注册成功")
	}

	fmt.Println("等待热键按下... 按 Ctrl+Alt+V 试试，按 Ctrl+C 退出")

	var m msg
	for {
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		if m.message == wmHotkey {
			fmt.Printf("收到热键！ wParam=%d\n", m.wParam)
		}
	}

	procUnregisterHotKey.Call(0, 1)
}
