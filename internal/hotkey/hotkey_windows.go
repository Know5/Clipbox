package hotkey

import (
	"errors"
	"fmt"
	goruntime "runtime"
	"sync"
	"syscall"
	"unsafe"
)

var (
	user32                 = syscall.NewLazyDLL("user32.dll")
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procRegisterHotKey     = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey   = user32.NewProc("UnregisterHotKey")
	procGetMessage         = user32.NewProc("GetMessageW")
	procPostThreadMessage  = user32.NewProc("PostThreadMessageW")
	procGetCurrentThreadId = kernel32.NewProc("GetCurrentThreadId")
)

const (
	wmHotkey    = 0x0312
	wmQuit      = 0x0012
	wmUser      = 0x0400
	wmRebind    = wmUser + 1
	modNoRepeat = 0x4000
	hotkeyID    = 1001
)

// RegisterHotkeyError 区分“被占用”和其它注册失败，便于上层给出可操作的提示。
type RegisterHotkeyError struct {
	Err      error
	Occupied bool
}

func (e *RegisterHotkeyError) Error() string {
	if e == nil || e.Err == nil {
		return "热键注册失败"
	}
	if e.Occupied {
		return "热键注册失败，该组合键可能已被其它软件占用: " + e.Err.Error()
	}
	return "热键注册失败: " + e.Err.Error()
}

func (e *RegisterHotkeyError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// IsHotkeyOccupiedError 报告错误是否很可能由其它软件占用热键导致。
func IsHotkeyOccupiedError(err error) bool {
	var hotkeyErr *RegisterHotkeyError
	if errors.As(err, &hotkeyErr) {
		return hotkeyErr.Occupied
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == 1409
	}
	return false
}

// Modifier key constants exported for use by other packages
const (
	ModAlt     = 0x0001
	ModControl = 0x0002
	ModShift   = 0x0004
	ModWin     = 0x0008
)

// PanicLogger is set by the host application to receive panic reports from
// internal goroutines. If nil, panics are silently recovered.
var PanicLogger func(format string, args ...interface{})

func recoverPanic(name string) {
	if r := recover(); r != nil {
		if PanicLogger != nil {
			PanicLogger("panic in %s: %v", name, r)
		}
	}
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

type Manager struct {
	Events       chan struct{}
	threadID     uint32
	mu           sync.Mutex
	rebindMu     sync.Mutex
	running      bool
	currentMod   uint32
	currentVK    uint32
	newMod       uint32
	newVK        uint32
	rebindResult chan error
}

func NewManager() *Manager {
	return &Manager{
		Events:       make(chan struct{}, 10),
		rebindResult: make(chan error, 1),
	}
}

func registerHotkey(modifiers, vk uint32) error {
	ret, _, err := procRegisterHotKey.Call(0, hotkeyID, uintptr(modifiers|modNoRepeat), uintptr(vk))
	if ret != 0 {
		return nil
	}

	ret, _, fallbackErr := procRegisterHotKey.Call(0, hotkeyID, uintptr(modifiers), uintptr(vk))
	if ret != 0 {
		return nil
	}

	var cause error
	if fallbackErr != syscall.Errno(0) {
		cause = fallbackErr
	} else {
		cause = err
	}
	return &RegisterHotkeyError{Err: cause, Occupied: IsHotkeyOccupiedError(cause)}
}
func rebindRegistrationError(err error) error {
	if _, ok := err.(syscall.Errno); ok {
		return &RegisterHotkeyError{Err: err, Occupied: IsHotkeyOccupiedError(err)}
	}
	return fmt.Errorf("新热键注册失败: %v", err)
}

func (m *Manager) Start(modifiers, vk uint32) error {
	m.mu.Lock()
	m.currentMod = modifiers
	m.currentVK = vk
	m.running = false
	m.mu.Unlock()

	errCh := make(chan error, 1)

	go func() {
		defer recoverPanic("hotkey:messageLoop")
		goruntime.LockOSThread()
		defer goruntime.UnlockOSThread()
		registered := false
		defer func() {
			if registered {
				procUnregisterHotKey.Call(0, hotkeyID)
			}
			m.mu.Lock()
			m.running = false
			m.threadID = 0
			m.mu.Unlock()
		}()

		tid, _, _ := procGetCurrentThreadId.Call()
		m.mu.Lock()
		m.threadID = uint32(tid)
		m.mu.Unlock()

		if err := registerHotkey(modifiers, vk); err != nil {
			errCh <- fmt.Errorf("热键注册失败: %v\n请检查是否有其他软件占用了该热键。", err)
			return
		}

		registered = true
		m.mu.Lock()
		m.running = true
		m.mu.Unlock()
		errCh <- nil

		var mData msg
		for {
			res, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&mData)), 0, 0, 0)
			if int32(res) <= 0 {
				break
			}

			if mData.message == wmHotkey && mData.wParam == hotkeyID {
				select {
				case m.Events <- struct{}{}:
				default:
				}
			}

			if mData.message == wmRebind {
				m.mu.Lock()
				newMod := m.newMod
				newVK := m.newVK
				m.mu.Unlock()

				// Unregister old hotkey
				procUnregisterHotKey.Call(0, hotkeyID)

				// Register new hotkey
				ret, _, rebindErr := procRegisterHotKey.Call(0, hotkeyID, uintptr(newMod|modNoRepeat), uintptr(newVK))
				if ret == 0 {
					ret, _, rebindErr = procRegisterHotKey.Call(0, hotkeyID, uintptr(newMod), uintptr(newVK))
				}

				if ret == 0 {
					// Failed: re-register old hotkey
					m.mu.Lock()
					oldMod := m.currentMod
					oldVK := m.currentVK
					m.mu.Unlock()
					restoreErr := registerHotkey(oldMod, oldVK)
					wrappedRebindErr := rebindRegistrationError(rebindErr)
					if restoreErr != nil {
						registered = false
						m.rebindResult <- fmt.Errorf("新热键注册失败: %v；原热键恢复失败: %v", wrappedRebindErr, restoreErr)
					} else {
						registered = true
						m.rebindResult <- wrappedRebindErr
					}
				} else {
					registered = true
					m.mu.Lock()
					m.currentMod = newMod
					m.currentVK = newVK
					m.mu.Unlock()
					m.rebindResult <- nil
				}
			}
		}
	}()

	return <-errCh
}

func (m *Manager) Rebind(modifiers, vk uint32) error {
	m.rebindMu.Lock()
	defer m.rebindMu.Unlock()

	m.mu.Lock()
	if !m.running || m.threadID == 0 {
		m.mu.Unlock()
		return fmt.Errorf("热键线程未运行")
	}
	m.newMod = modifiers
	m.newVK = vk
	tid := m.threadID
	m.mu.Unlock()

	ret, _, err := procPostThreadMessage.Call(uintptr(tid), wmRebind, 0, 0)
	if ret == 0 {
		return fmt.Errorf("热键线程不可用: %v", err)
	}
	return <-m.rebindResult
}

func (m *Manager) Stop() {
	m.mu.Lock()
	tid := m.threadID
	m.mu.Unlock()

	if tid != 0 {
		procPostThreadMessage.Call(uintptr(tid), wmQuit, 0, 0)
	}
}
