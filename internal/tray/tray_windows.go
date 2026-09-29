package tray

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

var (
	user32                = syscall.NewLazyDLL("user32.dll")
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	shell32               = syscall.NewLazyDLL("shell32.dll")
	procRegisterClassEx   = user32.NewProc("RegisterClassExW")
	procCreateWindowEx    = user32.NewProc("CreateWindowExW")
	procDefWindowProc     = user32.NewProc("DefWindowProcW")
	procDestroyWindow     = user32.NewProc("DestroyWindow")
	procGetMessage        = user32.NewProc("GetMessageW")
	procTranslateMessage  = user32.NewProc("TranslateMessage")
	procDispatchMessage   = user32.NewProc("DispatchMessageW")
	procPostThreadMessage = user32.NewProc("PostThreadMessageW")
	procPostMessage       = user32.NewProc("PostMessageW")
	procLoadIcon          = user32.NewProc("LoadIconW")
	procCreatePopupMenu   = user32.NewProc("CreatePopupMenu")
	procAppendMenu        = user32.NewProc("AppendMenuW")
	procTrackPopupMenu    = user32.NewProc("TrackPopupMenu")
	procDestroyMenu       = user32.NewProc("DestroyMenu")
	procSetForeground     = user32.NewProc("SetForegroundWindow")
	procGetCursorPos      = user32.NewProc("GetCursorPos")
	procGetCurrentThread  = kernel32.NewProc("GetCurrentThreadId")
	procShellNotifyIcon   = shell32.NewProc("Shell_NotifyIconW")
	procExtractIconEx     = shell32.NewProc("ExtractIconExW")
	procDestroyIcon       = user32.NewProc("DestroyIcon")
)

const (
	errorClassExists = 1410

	wmDestroy      = 0x0002
	wmCommand      = 0x0111
	wmNull         = 0x0000
	wmQuit         = 0x0012
	wmUser         = 0x0400
	wmTrayCallback = wmUser + 77

	wmLButtonUp     = 0x0202
	wmLButtonDblClk = 0x0203
	wmRButtonUp     = 0x0205
	wmContextMenu   = 0x007B // right-click with NOTIFYICON_VERSION_4

	nimAdd        = 0x00000000
	nimModify     = 0x00000001
	nimDelete     = 0x00000002
	nimSetVersion = 0x00000004

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	notifyIconVersion4 = 4

	idiApplication = 32512

	mfString    = 0x00000000
	mfSeparator = 0x00000800
	mfChecked   = 0x00000008

	tpmRightButton = 0x0002
	tpmRetCmd      = 0x0100
	tpmNonotify    = 0x0080

	menuShow     = 1001
	menuSettings = 1002
	menuPause    = 1003
	menuDataDir  = 1004
	menuExit     = 1005
)

type Action int

const (
	ActionShow Action = iota + 1
	ActionSettings
	ActionTogglePause
	ActionOpenDataDir
	ActionExit
)

type State struct {
	AppName string
	Paused  bool
	Visible bool
}

type Controller struct {
	Actions  chan Action
	initCh   chan error
	stopOnce sync.Once
	stateMu  sync.RWMutex
	state    State
	hwnd     uintptr
	threadID uintptr
}

type wndClassEx struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   syscall.Handle
	icon       syscall.Handle
	cursor     syscall.Handle
	background syscall.Handle
	menuName   *uint16
	className  *uint16
	iconSm     syscall.Handle
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
}

type point struct {
	x int32
	y int32
}

type guid struct {
	data1 uint32
	data2 uint16
	data3 uint16
	data4 [8]byte
}

type notifyIconData struct {
	size            uint32
	hwnd            uintptr
	id              uint32
	flags           uint32
	callbackMessage uint32
	icon            uintptr
	tip             [128]uint16
	state           uint32
	stateMask       uint32
	info            [256]uint16
	version         uint32
	infoTitle       [64]uint16
	infoFlags       uint32
	guid            guid
	balloonIcon     uintptr
}

var activeController *Controller

func New(appName string) *Controller {
	if appName == "" {
		appName = "ClipBox"
	}
	return &Controller{
		Actions: make(chan Action, 16),
		initCh:  make(chan error, 1),
		state: State{
			AppName: appName,
		},
	}
}

func (c *Controller) Start() error {
	activeController = c
	go c.messageLoop()
	return <-c.initCh
}

func (c *Controller) Stop() {
	c.stopOnce.Do(func() {
		if c.hwnd != 0 {
			c.removeIcon()
			procDestroyWindow.Call(c.hwnd)
		}
		if c.threadID != 0 {
			procPostThreadMessage.Call(c.threadID, wmQuit, 0, 0)
		}
	})
}

func (c *Controller) SetState(state State) {
	if state.AppName == "" {
		state.AppName = "ClipBox"
	}
	c.stateMu.Lock()
	c.state = state
	c.stateMu.Unlock()
	if c.hwnd != 0 {
		c.modifyIcon()
	}
}

func (c *Controller) currentState() State {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.state
}

func (c *Controller) messageLoop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	threadID, _, _ := procGetCurrentThread.Call()
	c.threadID = threadID

	className, _ := syscall.UTF16PtrFromString("ClipBoxTrayWindow")
	wcx := wndClassEx{
		wndProc:   syscall.NewCallback(trayWndProc),
		className: className,
	}
	wcx.size = uint32(unsafe.Sizeof(wcx))

	ret, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wcx)))
	if ret == 0 && err != syscall.Errno(errorClassExists) {
		c.initCh <- fmt.Errorf("register tray window class: %w", callError(err))
		return
	}

	hwnd, _, err := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		0,
		0,
		0, 0, 0, 0,
		0, 0, 0, 0,
	)
	if hwnd == 0 {
		c.initCh <- fmt.Errorf("create tray window: %w", callError(err))
		return
	}
	c.hwnd = hwnd

	if err := c.addIcon(); err != nil {
		c.initCh <- err
		return
	}
	c.initCh <- nil

	var m msg
	for {
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if ret == 0 {
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func trayWndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	c := activeController
	if c == nil {
		ret, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
		return ret
	}

	switch message {
	case wmTrayCallback:
		switch uint32(lParam & 0xFFFF) {
		case wmLButtonUp, wmLButtonDblClk:
			c.dispatch(ActionShow)
		case wmRButtonUp, wmContextMenu:
			c.showMenu()
		}
		return 0
	case wmCommand:
		c.dispatch(menuAction(uint16(wParam & 0xffff)))
		return 0
	case wmDestroy:
		c.removeIcon()
		return 0
	}

	ret, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return ret
}

func menuAction(id uint16) Action {
	switch id {
	case menuShow:
		return ActionShow
	case menuSettings:
		return ActionSettings
	case menuPause:
		return ActionTogglePause
	case menuDataDir:
		return ActionOpenDataDir
	case menuExit:
		return ActionExit
	default:
		return 0
	}
}

func (c *Controller) dispatch(action Action) {
	if action == 0 {
		return
	}
	select {
	case c.Actions <- action:
	default:
	}
}

func (c *Controller) addIcon() error {
	data := c.iconData(nifMessage | nifIcon | nifTip)
	if ret, _, err := procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&data))); ret == 0 {
		return fmt.Errorf("add tray icon: %w", callError(err))
	}
	data.version = notifyIconVersion4
	procShellNotifyIcon.Call(nimSetVersion, uintptr(unsafe.Pointer(&data)))
	return nil
}

func (c *Controller) modifyIcon() {
	data := c.iconData(nifTip)
	procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&data)))
}

func (c *Controller) removeIcon() {
	data := c.iconData(0)
	procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&data)))
}

func (c *Controller) iconData(flags uint32) notifyIconData {
	state := c.currentState()
	data := notifyIconData{
		size:            uint32(unsafe.Sizeof(notifyIconData{})),
		hwnd:            c.hwnd,
		id:              1,
		flags:           flags,
		callbackMessage: wmTrayCallback,
		icon:            defaultIcon(),
	}
	copyUTF16(data.tip[:], trayTip(state))
	return data
}

// defaultIcon returns the tray icon. It first tries to extract the icon that
// wails build embedded into this executable (branded ClipBox icon), and falls
// back to the generic Windows application icon if extraction fails.
func defaultIcon() uintptr {
	if icon := exeIcon(); icon != 0 {
		return icon
	}
	icon, _, _ := procLoadIcon.Call(0, uintptr(idiApplication))
	return icon
}

// exeIcon extracts the first small icon embedded in the current executable.
func exeIcon() uintptr {
	exe, err := os.Executable()
	if err != nil {
		return 0
	}
	exePtr, err := syscall.UTF16PtrFromString(exe)
	if err != nil {
		return 0
	}
	var largeIcon, smallIcon uintptr
	// ExtractIconExW(path, 0, &large, &small, 1) — index 0 = first icon group.
	ret, _, _ := procExtractIconEx.Call(
		uintptr(unsafe.Pointer(exePtr)),
		0,
		uintptr(unsafe.Pointer(&largeIcon)),
		uintptr(unsafe.Pointer(&smallIcon)),
		1,
	)
	if ret == 0 {
		return 0
	}
	// Prefer the small icon for the tray; fall back to the large one.
	if smallIcon != 0 {
		if largeIcon != 0 {
			procDestroyIcon.Call(largeIcon)
		}
		return smallIcon
	}
	return largeIcon
}

func trayTip(state State) string {
	if state.AppName == "" {
		state.AppName = "ClipBox"
	}
	if state.Paused {
		return state.AppName + " - 记录已暂停"
	}
	return state.AppName + " - 正在记录"
}

func (c *Controller) showMenu() {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)

	state := c.currentState()
	showLabel := "显示 ClipBox"
	if state.Visible {
		showLabel = "隐藏 ClipBox"
	}
	appendMenu(menu, mfString, menuShow, showLabel)
	appendMenu(menu, mfString, menuSettings, "打开设置")
	appendMenu(menu, mfSeparator, 0, "")
	pauseFlags := uintptr(mfString)
	pauseLabel := "暂停记录"
	if state.Paused {
		pauseFlags |= mfChecked
		pauseLabel = "恢复记录"
	}
	appendMenu(menu, pauseFlags, menuPause, pauseLabel)
	appendMenu(menu, mfString, menuDataDir, "打开数据目录")
	appendMenu(menu, mfSeparator, 0, "")
	appendMenu(menu, mfString, menuExit, "退出 ClipBox")

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForeground.Call(c.hwnd)
	id, _, _ := procTrackPopupMenu.Call(
		menu,
		tpmRightButton|tpmRetCmd|tpmNonotify,
		uintptr(pt.x),
		uintptr(pt.y),
		0,
		c.hwnd,
		0,
	)
	procPostMessage.Call(c.hwnd, wmNull, 0, 0)
	if id != 0 {
		c.dispatch(menuAction(uint16(id)))
	}
}

func appendMenu(menu uintptr, flags uintptr, id uint16, text string) {
	var ptr uintptr
	if text != "" {
		wide, _ := syscall.UTF16PtrFromString(text)
		ptr = uintptr(unsafe.Pointer(wide))
	}
	procAppendMenu.Call(menu, flags, uintptr(id), ptr)
}

func copyUTF16(dst []uint16, text string) {
	wide := syscall.StringToUTF16(text)
	if len(wide) > len(dst) {
		wide = wide[:len(dst)]
		wide[len(wide)-1] = 0
	}
	copy(dst, wide)
}

func callError(err error) error {
	if errno, ok := err.(syscall.Errno); ok && errno == 0 {
		return syscall.GetLastError()
	}
	return err
}
