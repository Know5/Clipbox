package popup

// Native GDI list popup owned by the resident process. Production builds show
// this window instead of the WebView2 list; the WebView2 UI remains for
// settings, detail and text tools. No browser runtime means the popup adds
// only a few MiB to the resident.
//
// Threading: New locks a dedicated OS thread that creates the window and runs
// the message loop. All exported methods are safe from any goroutine; they
// marshal onto the popup thread with SendMessage. Callbacks run on fresh
// goroutines, never on the popup thread, so a callback may safely call back
// into the popup (e.g. Paste -> hideWindow -> Hide).
//
// Key handling mirrors the React list (App.tsx keydown):
//   - search box (EDIT) consumes ↑↓/Enter/Ctrl+Enter/Ctrl+1-9/Alt+1-9/Esc;
//     everything else (letters, Home, P, Space, Delete) behaves like a text
//     box, so typing "p" or spaces into search works.
//   - P/Space/i/Delete/Home act on the list only when focus is outside the
//     search box (click the toolbar or press Tab), matching the web UI.

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	gdi32  = syscall.NewLazyDLL("gdi32.dll")
	imm32  = syscall.NewLazyDLL("imm32.dll")

	procRegisterClassExW          = user32.NewProc("RegisterClassExW")
	procCreateWindowExW           = user32.NewProc("CreateWindowExW")
	procDefWindowProcW            = user32.NewProc("DefWindowProcW")
	procDestroyWindow             = user32.NewProc("DestroyWindow")
	procGetMessageW               = user32.NewProc("GetMessageW")
	procTranslateMessage          = user32.NewProc("TranslateMessage")
	procDispatchMessageW          = user32.NewProc("DispatchMessageW")
	procPostQuitMessage           = user32.NewProc("PostQuitMessage")
	procSendMessageW              = user32.NewProc("SendMessageW")
	procPostMessageW              = user32.NewProc("PostMessageW")
	procShowWindow                = user32.NewProc("ShowWindow")
	procSetForegroundWindow       = user32.NewProc("SetForegroundWindow")
	procSetFocus                  = user32.NewProc("SetFocus")
	procGetFocus                  = user32.NewProc("GetFocus")
	procGetClientRect             = user32.NewProc("GetClientRect")
	procFillRect                  = user32.NewProc("FillRect")
	procDrawTextW                 = user32.NewProc("DrawTextW")
	procLoadCursorW               = user32.NewProc("LoadCursorW")
	procSetWindowLongPtrW         = user32.NewProc("SetWindowLongPtrW")
	procCallWindowProcW           = user32.NewProc("CallWindowProcW")
	procGetKeyState               = user32.NewProc("GetKeyState")
	procGetCursorPos              = user32.NewProc("GetCursorPos")
	procSetTimer                  = user32.NewProc("SetTimer")
	procKillTimer                 = user32.NewProc("KillTimer")
	procInvalidateRect            = user32.NewProc("InvalidateRect")
	procValidateRect              = user32.NewProc("ValidateRect")
	procMoveWindow                = user32.NewProc("MoveWindow")
	procSetWindowTextW            = user32.NewProc("SetWindowTextW")
	procGetDpiForSystem           = user32.NewProc("GetDpiForSystem")
	procSetProcessDpiAwarenessCtx = user32.NewProc("SetProcessDpiAwarenessContext")
	procSetProcessDPIAware        = user32.NewProc("SetProcessDPIAware")
	procMonitorFromPoint          = user32.NewProc("MonitorFromPoint")
	procGetMonitorInfoW           = user32.NewProc("GetMonitorInfoW")
	procGetWindowDC               = user32.NewProc("GetWindowDC")
	procGetParent                 = user32.NewProc("GetParent")

	procGetCurrentThreadID = syscall.NewLazyDLL("kernel32.dll").NewProc("GetCurrentThreadId")
	procGetModuleHandleW   = syscall.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW")

	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject           = gdi32.NewProc("SelectObject")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
	procDeleteObject           = gdi32.NewProc("DeleteObject")
	procCreateSolidBrush       = gdi32.NewProc("CreateSolidBrush")
	procSetBkMode              = gdi32.NewProc("SetBkMode")
	procSetTextColor           = gdi32.NewProc("SetTextColor")
	procCreateFontW            = gdi32.NewProc("CreateFontW")
	procBitBlt                 = gdi32.NewProc("BitBlt")
	procPolygon                = gdi32.NewProc("Polygon")

	procImmGetContext            = imm32.NewProc("ImmGetContext")
	procImmReleaseContext        = imm32.NewProc("ImmReleaseContext")
	procImmGetCompositionStringW = imm32.NewProc("ImmGetCompositionStringW")
)

const (
	popupClassName = "ClipBoxPopup"

	wmCreate      = 0x0001
	wmDestroy     = 0x0002
	wmPaint       = 0x000F
	wmClose       = 0x0010
	wmKeyDown     = 0x0100
	wmCommand     = 0x0111
	wmTimer       = 0x0113
	wmLButtonDown = 0x0201
	wmMouseWheel  = 0x020A
	wmSetFocus    = 0x0007
	wmKillFocus   = 0x0008
	wmDpiChanged  = 0x02E0
	wmUser        = 0x0400

	wmPopupShow    = wmUser + 1
	wmPopupHide    = wmUser + 2
	wmPopupRefresh = wmUser + 3
	wmPopupTheme   = wmUser + 4
	wmPopupClose   = wmUser + 5

	editID     = 1
	debounceID = 2

	gwlWndProc = ^uintptr(3) // -4

	wsChild   = 0x40000000
	wsVisible = 0x10000000
	wsPopup   = 0x80000000
	wsClipSib = 0x04000000

	wsExTopmost = 0x00000008

	esAutoHScroll = 0x0080

	swShow           = 5
	swShowNoActivate = 4
	swHide           = 0

	idcArrow = 32512

	dtSingleLine  = 0x20
	dtVCenter     = 0x4
	dtEndEllipsis = 0x8000
	dtNoPrefix    = 0x800
	dtCenter      = 0x1
	dtRight       = 0x2

	transparentBk = 1

	enChange  = 0x0300
	wmGetText = 0x000E
	wmSetFont = 0x0030

	cchAuto = 0xFFFFFFFF // DrawTextW: null-terminated string

	vkReturn = 0x0D
	vkEscape = 0x1B
	vkSpace  = 0x20
	vkPrior  = 0x21
	vkNext   = 0x22
	vkEnd    = 0x23
	vkHome   = 0x24
	vkUp     = 0x26
	vkDown   = 0x28
	vkDelete = 0x2E

	vkI = 0x49
	vkP = 0x50

	monitorDefaultToNearest = 2

	logicalWidth   = 400
	logicalHeight  = 560
	headerHeight   = 36
	toolbarHeight  = 30
	rowTextHeight  = 32
	rowImageHeight = 48

	gcsCompStr = 0x0008
)

// Row is one list item. Preview is the single-line text shown for both kinds.
type Row struct {
	ID        int64
	Kind      string // "text" | "image"
	Preview   string
	Pinned    bool
	Timestamp int64 // unix milliseconds
}

// Theme holds GDI COLORREFs (0x00BBGGRR).
type Theme struct {
	Background uint32
	Foreground uint32
	Muted      uint32
	Accent     uint32
	AccentText uint32
	Border     uint32
}

// DataSource supplies list pages. Page runs on a background goroutine; it must
// not call back into the popup.
type DataSource interface {
	Page(query, filter string, limit, offset int) (rows []Row, total int, err error)
}

// DataSourceFunc adapts a function to DataSource.
type DataSourceFunc func(query, filter string, limit, offset int) ([]Row, int, error)

func (f DataSourceFunc) Page(query, filter string, limit, offset int) ([]Row, int, error) {
	return f(query, filter, limit, offset)
}

// Callbacks are invoked on fresh goroutines; they may call popup methods.
type Callbacks struct {
	Paste     func(id int64, copyOnly bool)
	TogglePin func(id int64)
	Delete    func(id int64)
	Details   func(id int64)
	Settings  func()
	Filter    func(filter string)
}

// Window is a hidden popup until Show is called.
type Window struct {
	cb  Callbacks
	src DataSource

	initCh chan error

	mu       sync.Mutex
	hwnd     uintptr
	edit     uintptr
	threadID uintptr

	rows         []Row
	total        int
	query        string
	filter       string // "all" | "text" | "image"
	selected     int    // index into rows
	firstVisible int    // index of the top drawn row
	confirming   bool   // delete confirm armed
	visible      bool
	loading      bool
	theme        Theme
}

// New registers the window class and starts the popup thread. It returns once
// the window exists (hidden).
func New(src DataSource, cb Callbacks) (*Window, error) {
	w := &Window{
		src:    src,
		cb:     cb,
		initCh: make(chan error, 1),
		filter: "all",
		theme:  defaultTheme(),
	}
	class := wndClassEx{
		size:      uint32(unsafe.Sizeof(wndClassEx{})),
		wndProc:   syscall.NewCallback(wndProc),
		instance:  getModuleHandle(),
		className: utf16Ptr(popupClassName),
		cursor:    loadCursor(idcArrow),
	}
	_, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class)))
	if err != syscall.Errno(0) && err != syscall.Errno(1410) { // 1410 = already registered
		return nil, fmt.Errorf("register popup class: %v", err)
	}
	go w.run()
	if err := <-w.initCh; err != nil {
		return nil, err
	}
	return w, nil
}

func (w *Window) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// Per-monitor v2 DPI awareness for crisp scaling; ignore failure (older
	// Windows or already set by Wails in dev builds).
	procSetProcessDpiAwarenessCtx.Call(0xFFFFFFFFFFFFFFFC)
	procSetProcessDPIAware.Call(0)

	creatingWindow = w
	hwnd, _, err := procCreateWindowExW.Call(
		wsExTopmost, utf16Ptr(popupClassName), utf16Ptr("ClipBox"),
		wsPopup|wsClipSib, 0, 0, uintptr(w.width()), uintptr(w.height()),
		0, 0, getModuleHandle(), 0)
	creatingWindow = nil
	if hwnd == 0 {
		w.initCh <- fmt.Errorf("create popup window: %v", err)
		return
	}
	w.mu.Lock()
	w.hwnd = hwnd
	w.threadID = currentThreadID()
	w.mu.Unlock()
	w.initCh <- nil

	var msg struct {
		hwnd    uintptr
		message uint32
		wParam  uintptr
		lParam  uintptr
		time    uint32
		pt      struct{ x, y int32 }
	}
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func (w *Window) width() int  { return scale(logicalWidth) }
func (w *Window) height() int { return scale(logicalHeight) }

// Show places the window near the cursor inside the monitor work area, makes
// it visible and focuses the search box. activate=false shows without taking
// focus (isolated test instances).
func (w *Window) Show(activate bool) {
	if activate {
		w.send(wmPopupShow, 1, 0)
		return
	}
	w.send(wmPopupShow, 0, 0)
}

// Hide synchronously hides the window; the caller may switch focus to the
// paste target immediately afterwards.
func (w *Window) Hide() {
	w.send(wmPopupHide, 0, 0)
}

// Refresh reloads the current page (call on clips:updated).
func (w *Window) Refresh() {
	w.send(wmPopupRefresh, 0, 0)
}

// SetTheme applies colors and repaints.
func (w *Window) SetTheme(t Theme) {
	w.mu.Lock()
	w.theme = t
	w.mu.Unlock()
	w.send(wmPopupTheme, 0, 0)
}

// Close destroys the window and stops the popup thread.
func (w *Window) Close() {
	w.send(wmPopupClose, 0, 0)
}

func (w *Window) send(msg uint32, wp, lp uintptr) {
	w.mu.Lock()
	hwnd := w.hwnd
	threadID := w.threadID
	w.mu.Unlock()
	if hwnd == 0 {
		return
	}
	// Window procs run on the popup thread: a SendMessage to our own thread
	// would deadlock inside DispatchMessage. Dispatch directly instead.
	if currentThreadID() == threadID {
		wndProc(hwnd, msg, wp, lp)
		return
	}
	procSendMessageW.Call(hwnd, uintptr(msg), wp, lp)
}

// ---- window proc ---------------------------------------------------------

func wndProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	if msg == wmCreate {
		window := creatingWindow
		if window != nil {
			windows.Store(hwnd, window)
		}
		if window == nil {
			return defWindowProc(hwnd, msg, wp, lp)
		}
		return window.onCreate(hwnd)
	}
	window := windowFromHWND(hwnd)
	if window == nil {
		return defWindowProc(hwnd, msg, wp, lp)
	}
	switch msg {
	case wmDestroy:
		window.onDestroy()
		return 0
	case wmCommand:
		if wp>>16 == enChange && wp&0xFFFF == editID {
			window.onSearchChanged()
		}
		return 0
	case wmTimer:
		if wp == debounceID {
			procKillTimer.Call(hwnd, wp)
			window.reloadAsync()
		}
		return 0
	case wmPopupShow:
		window.onShow(wp != 0)
		return 0
	case wmPopupHide:
		window.onHide()
		return 0
	case wmPopupRefresh:
		window.reloadAsync()
		return 0
	case wmPopupTheme:
		procInvalidateRect.Call(hwnd, 0, 1)
		return 0
	case wmPopupClose:
		procDestroyWindow.Call(hwnd)
		return 0
	case wmPaint:
		window.paint(hwnd)
		return 0
	case wmKeyDown:
		if window.onWindowKey(int32(wp)) {
			return 0
		}
	case wmLButtonDown:
		window.onClick(int32(lp&0xFFFF), int32(lp>>16))
		return 0
	case wmMouseWheel:
		window.onWheel(int32(uint32(wp)>>16) > 0)
		return 0
	}
	return defWindowProc(hwnd, msg, wp, lp)
}

func (w *Window) onCreate(hwnd uintptr) uintptr {
	// A real EDIT keeps IME composition and CJK input working for free.
	edit, _, _ := procCreateWindowExW.Call(
		0, utf16Ptr("EDIT"), utf16Ptr(""),
		wsChild|wsVisible|esAutoHScroll,
		uintptr(scale(8)), uintptr(scale(4)), uintptr(w.width()-scale(16)), uintptr(scale(28)),
		hwnd, editID, getModuleHandle(), 0)
	if edit == 0 {
		return 0
	}
	if font, _ := createFont(14, 400); font != 0 {
		procSendMessageW.Call(edit, wmSetFont, font, 0)
	}
	w.mu.Lock()
	w.edit = edit
	w.mu.Unlock()
	subclassEdit(edit)
	procInvalidateRect.Call(hwnd, 0, 1)
	return 0
}

func (w *Window) onDestroy() {
	w.mu.Lock()
	hwnd := w.hwnd
	w.hwnd = 0
	if w.edit != 0 {
		editProcs.Delete(w.edit)
	}
	w.edit = 0
	w.mu.Unlock()
	if hwnd != 0 {
		windows.Delete(hwnd)
	}
	procPostQuitMessage.Call(0)
}

func (w *Window) onShow(activate bool) {
	w.mu.Lock()
	hwnd := w.hwnd
	w.mu.Unlock()
	x, y := placeNearCursor(w.width(), w.height())
	procMoveWindow.Call(hwnd, uintptr(x), uintptr(y), uintptr(w.width()), uintptr(w.height()), 1)
	w.mu.Lock()
	w.visible = true
	w.selected = 0
	w.firstVisible = 0
	w.confirming = false
	edit := w.edit
	w.mu.Unlock()
	showFlag := uintptr(swShowNoActivate)
	if activate {
		showFlag = swShow
	}
	procShowWindow.Call(hwnd, showFlag)
	// A WS_POPUP window is not invalidated by becoming visible: without this,
	// WM_PAINT never fires and the surface stays black.
	procInvalidateRect.Call(hwnd, 0, 1)
	if activate {
		procSetForegroundWindow.Call(hwnd)
		procSetFocus.Call(edit)
	}
	w.reloadAsync()
}

func (w *Window) onHide() {
	w.mu.Lock()
	w.visible = false
	hwnd := w.hwnd
	w.mu.Unlock()
	procShowWindow.Call(hwnd, swHide)
}

func (w *Window) onSearchChanged() {
	w.mu.Lock()
	edit := w.edit
	w.mu.Unlock()
	if edit == 0 {
		return
	}
	var buf [512]uint16
	n, _, _ := procSendMessageW.Call(edit, wmGetText, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	query := syscall.UTF16ToString(buf[:int(n)])
	w.mu.Lock()
	w.query = query
	w.selected = 0
	w.firstVisible = 0
	w.confirming = false
	hwnd := w.hwnd
	w.mu.Unlock()
	procSetTimer.Call(hwnd, debounceID, 40, 0)
}

func (w *Window) reloadAsync() {
	w.mu.Lock()
	if w.loading {
		w.mu.Unlock()
		return
	}
	w.loading = true
	query, filter := w.query, w.filter
	w.mu.Unlock()
	go func() {
		rows, total, err := w.src.Page(query, filter, 200, 0)
		w.mu.Lock()
		w.loading = false
		if err == nil {
			w.rows = rows
			w.total = total
			if w.selected >= len(rows) && len(rows) > 0 {
				w.selected = len(rows) - 1
			}
		}
		hwnd := w.hwnd
		w.mu.Unlock()
		if hwnd != 0 {
			procInvalidateRect.Call(hwnd, 0, 1)
		}
	}()
}

// ---- keyboard ------------------------------------------------------------

// windows maps hwnd to its *Window. The registry avoids unsafe pointer
// round-trips through GWLP_USERDATA; WM_CREATE is synchronous on the popup
// thread, so creatingWindow is set for exactly the CreateWindowExW call.
var (
	windows        sync.Map // hwnd -> *Window
	creatingWindow *Window
)

// editProcs maps the subclassed EDIT hwnd to its original window proc.
var editProcs sync.Map // edit hwnd -> uintptr

func subclassEdit(edit uintptr) {
	orig, _, _ := procSetWindowLongPtrW.Call(edit, gwlWndProc, syscall.NewCallback(editProc))
	editProcs.Store(edit, orig)
}

func editProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	orig, _ := editProcs.Load(hwnd)
	forward := func() uintptr {
		if orig != nil {
			return callWindowProc(orig.(uintptr), hwnd, msg, wp, lp)
		}
		return defWindowProc(hwnd, msg, wp, lp)
	}
	window := windowFromEdit(hwnd)
	if window == nil || msg != wmKeyDown {
		return forward()
	}
	// Never act while an IME composition is open (Enter commits the word).
	if isComposing(hwnd) {
		return forward()
	}
	vk := int32(wp)
	ctrl := keyDown(0x11)
	alt := keyDown(0x12)
	switch {
	case vk == vkEscape:
		window.escapePressed()
		return 0
	case ctrl && vk == vkReturn:
		window.activateSelected(true)
		return 0
	case vk == vkReturn:
		window.activateSelected(false)
		return 0
	case ctrl && vk >= 0x31 && vk <= 0x39:
		window.activateIndex(int(vk)-0x31, true)
		return 0
	case alt && vk >= 0x31 && vk <= 0x39:
		window.activateIndex(int(vk)-0x31, false)
		return 0
	case vk == vkUp:
		window.moveSelection(-1)
		return 0
	case vk == vkDown:
		window.moveSelection(1)
		return 0
	}
	return forward()
}

// onWindowKey handles list keys that only apply when focus is outside the
// search box, mirroring App.tsx (!isInput guards).
func (w *Window) onWindowKey(vk int32) bool {
	w.mu.Lock()
	edit, rows, selected := w.edit, w.rows, w.selected
	w.mu.Unlock()
	if edit != 0 {
		focus, _, _ := procGetFocus.Call(0)
		if focus == edit {
			return false
		}
	}
	switch vk {
	case vkHome:
		w.mu.Lock()
		w.selected = 0
		w.firstVisible = 0
		w.confirming = false
		hwnd := w.hwnd
		w.mu.Unlock()
		if hwnd != 0 {
			procInvalidateRect.Call(hwnd, 0, 1)
		}
		return true
	case vkP:
		if len(rows) > selected {
			go w.cb.TogglePin(rows[selected].ID)
		}
		return true
	case vkDelete:
		if len(rows) > selected {
			w.armDelete(selected)
		}
		return true
	case vkSpace, vkI:
		if len(rows) > selected {
			go w.cb.Details(rows[selected].ID)
		}
		return true
	}
	return false
}

func (w *Window) escapePressed() {
	w.mu.Lock()
	query := w.query
	w.mu.Unlock()
	if query != "" {
		w.setQuery("")
		return
	}
	w.Hide()
}

func (w *Window) setQuery(query string) {
	w.mu.Lock()
	w.query = query
	w.selected = 0
	w.firstVisible = 0
	w.confirming = false
	edit := w.edit
	hwnd := w.hwnd
	w.mu.Unlock()
	if edit != 0 {
		procSetWindowTextW.Call(edit, utf16Ptr(query))
	}
	if hwnd != 0 {
		procInvalidateRect.Call(hwnd, 0, 1)
	}
	w.reloadAsync()
}

func (w *Window) activateSelected(copyOnly bool) {
	w.mu.Lock()
	rows := w.rows
	selected := w.selected
	w.mu.Unlock()
	if selected < 0 || selected >= len(rows) {
		return
	}
	w.Hide()
	go w.cb.Paste(rows[selected].ID, copyOnly)
}

func (w *Window) activateIndex(index int, copyOnly bool) {
	w.mu.Lock()
	rows := w.rows
	w.mu.Unlock()
	if index < 0 || index >= len(rows) {
		return
	}
	w.Hide()
	go w.cb.Paste(rows[index].ID, copyOnly)
}

func (w *Window) moveSelection(delta int) {
	w.mu.Lock()
	rows := w.rows
	selected := w.selected
	firstVisible := w.firstVisible
	visibleCount := w.visibleCount()
	w.mu.Unlock()
	if len(rows) == 0 {
		return
	}
	next := selected + delta
	if next < 0 {
		next = 0
	}
	if next >= len(rows) {
		next = len(rows) - 1
	}
	w.mu.Lock()
	w.selected = next
	w.confirming = false
	if next < firstVisible {
		w.firstVisible = next
	} else if next >= firstVisible+visibleCount {
		w.firstVisible = next - visibleCount + 1
	}
	hwnd := w.hwnd
	w.mu.Unlock()
	if hwnd != 0 {
		procInvalidateRect.Call(hwnd, 0, 1)
	}
}

func (w *Window) armDelete(index int) {
	w.mu.Lock()
	if index < 0 || index >= len(w.rows) {
		w.mu.Unlock()
		return
	}
	if w.confirming {
		w.confirming = false
		row := w.rows[index]
		hwnd := w.hwnd
		w.mu.Unlock()
		go w.cb.Delete(row.ID)
		if hwnd != 0 {
			procInvalidateRect.Call(hwnd, 0, 1)
		}
		return
	}
	w.confirming = true
	hwnd := w.hwnd
	w.mu.Unlock()
	if hwnd != 0 {
		procInvalidateRect.Call(hwnd, 0, 1)
	}
}

// visibleCount is the number of rows that fit in the list area (upper bound;
// image rows are taller).
func (w *Window) visibleCount() int {
	available := logicalHeight - headerHeight - toolbarHeight
	return available / rowTextHeight
}

// ---- mouse ---------------------------------------------------------------

func (w *Window) onClick(x, y int32) {
	w.mu.Lock()
	rows := w.rows
	firstVisible := w.firstVisible
	w.mu.Unlock()
	if int(y) < scale(headerHeight) {
		return // search box area (the edit owns it)
	}
	if int(y) < scale(headerHeight+toolbarHeight) {
		w.onToolbarClick(x)
		return
	}
	idx := firstVisible + w.rowAt(y)
	if idx >= 0 && idx < len(rows) {
		w.mu.Lock()
		w.selected = idx
		w.mu.Unlock()
		w.Hide()
		go w.cb.Paste(rows[idx].ID, false)
	}
}

func (w *Window) onToolbarClick(x int32) {
	segment := int(x) / scale(64)
	if segment < 0 {
		segment = 0
	}
	if segment > 2 {
		segment = 2
	}
	filter := []string{"all", "text", "image"}[segment]
	w.mu.Lock()
	w.filter = filter
	w.selected = 0
	w.firstVisible = 0
	hwnd := w.hwnd
	w.mu.Unlock()
	// Clicking a filter is a body-focus click in the web UI; match it so
	// P/Space/Delete/Home keys work afterwards.
	procSetFocus.Call(hwnd)
	go w.cb.Filter(filter)
	w.reloadAsync()
}

func (w *Window) onWheel(up bool) {
	if up {
		w.moveSelection(-1)
	} else {
		w.moveSelection(1)
	}
}

// rowAt maps a client y inside the list area to a visual row index.
func (w *Window) rowAt(y int32) int {
	w.mu.Lock()
	rows := w.rows
	firstVisible := w.firstVisible
	w.mu.Unlock()
	top := scale(headerHeight + toolbarHeight)
	for i := firstVisible; i < len(rows); i++ {
		h := scale(rowTextHeight)
		if rows[i].Kind == "image" {
			h = scale(rowImageHeight)
		}
		if int(y) >= top && int(y) < top+h {
			return i
		}
		top += h
	}
	return -1
}

// debugPaint is a test hook invoked on every paint with the copied state.
var debugPaint func(rows, selected, firstVisible int, theme Theme)

// ---- painting ------------------------------------------------------------

func (w *Window) paint(hwnd uintptr) {
	var rect struct{ left, top, right, bottom int32 }
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	wdt := int(rect.right)
	hgt := int(rect.bottom)
	if wdt <= 0 || hgt <= 0 {
		return
	}
	screen, _, _ := procGetWindowDC.Call(hwnd)
	mem, _, _ := procCreateCompatibleDC.Call(screen)
	bitmap, _, _ := procCreateCompatibleBitmap.Call(screen, uintptr(wdt), uintptr(hgt))
	old, _, _ := procSelectObject.Call(mem, bitmap)
	defer func() {
		procSelectObject.Call(mem, old)
		procDeleteObject.Call(bitmap)
		procDeleteDC.Call(mem)
		procValidateRect.Call(hwnd, 0)
	}()

	w.mu.Lock()
	theme := w.theme
	rows := w.rows
	selected := w.selected
	firstVisible := w.firstVisible
	query := w.query
	filter := w.filter
	confirming := w.confirming
	w.mu.Unlock()

	if debugPaint != nil {
		debugPaint(len(rows), selected, firstVisible, theme)
	}

	procSetBkMode.Call(mem, transparentBk)
	background, _, _ := procCreateSolidBrush.Call(uintptr(theme.Background))
	procFillRect.Call(mem, uintptr(unsafe.Pointer(&rect)), background)
	procDeleteObject.Call(background)

	// Header underline.
	lineBrush, _, _ := procCreateSolidBrush.Call(uintptr(theme.Border))
	lineRect := rectOf(0, scale(headerHeight), wdt, 1)
	procFillRect.Call(mem, uintptr(unsafe.Pointer(lineRect)), lineBrush)
	procDeleteObject.Call(lineBrush)

	// Toolbar: filter segments.
	w.paintToolbar(mem, wdt, filter, theme)

	if len(rows) == 0 {
		text := "没有记录"
		if query != "" {
			text = "没有匹配的记录"
		}
		procSetTextColor.Call(mem, uintptr(theme.Muted))
		empty := rectOf(0, scale(headerHeight+toolbarHeight), wdt, hgt-scale(headerHeight+toolbarHeight))
		procDrawTextW.Call(mem, utf16Ptr(text), cchAuto, uintptr(unsafe.Pointer(empty)),
			dtCenter|dtVCenter|dtSingleLine|dtNoPrefix)
	} else {
		w.paintRows(mem, wdt, hgt, rows, selected, firstVisible, confirming, theme)
	}

	procBitBlt.Call(screen, 0, 0, uintptr(wdt), uintptr(hgt), mem, 0, 0, 0x00CC0020)
	procDeleteDC.Call(screen)
}

func (w *Window) paintToolbar(mem uintptr, wdt int, filter string, theme Theme) {
	names := []string{"全部", "文本", "图片"}
	values := []string{"all", "text", "image"}
	x := scale(8)
	for i, name := range names {
		selected := values[i] == filter
		color := theme.Muted
		if selected {
			color = theme.Accent
		}
		procSetTextColor.Call(mem, uintptr(color))
		rect := rectOf(x, scale(headerHeight+4), scale(56), scale(toolbarHeight-8))
		procDrawTextW.Call(mem, utf16Ptr(name), cchAuto, uintptr(unsafe.Pointer(rect)),
			dtCenter|dtVCenter|dtSingleLine|dtNoPrefix)
		if selected {
			brush, _, _ := procCreateSolidBrush.Call(uintptr(theme.Accent))
			under := rectOf(x, scale(headerHeight+toolbarHeight-5), scale(56), 2)
			procFillRect.Call(mem, uintptr(unsafe.Pointer(under)), brush)
			procDeleteObject.Call(brush)
		}
		x += scale(64)
	}
	_ = wdt
}

func (w *Window) paintRows(mem uintptr, wdt, hgt int, rows []Row, selected, firstVisible int, confirming bool, theme Theme) {
	top := scale(headerHeight + toolbarHeight)
	regular, _ := createFont(13, 400)
	muted, _ := createFont(12, 400)
	defer func() {
		if regular != 0 {
			procDeleteObject.Call(regular)
		}
		if muted != 0 {
			procDeleteObject.Call(muted)
		}
	}()
	pad := scale(8)
	rightPad := scale(58)
	numberPad := scale(14)
	for i := firstVisible; i < len(rows); i++ {
		if top >= hgt {
			break
		}
		row := rows[i]
		h := scale(rowTextHeight)
		if row.Kind == "image" {
			h = scale(rowImageHeight)
		}
		if top+h > hgt {
			h = hgt - top // draw a partial row at the bottom edge
		}
		rowRect := rectOf(0, top, wdt, h)
		if i == selected {
			brush, _, _ := procCreateSolidBrush.Call(uintptr(theme.Accent))
			procFillRect.Call(mem, uintptr(unsafe.Pointer(rowRect)), brush)
			procDeleteObject.Call(brush)
			procSetTextColor.Call(mem, uintptr(theme.AccentText))
		} else {
			procSetTextColor.Call(mem, uintptr(theme.Foreground))
		}

		procSelectObject.Call(mem, regular)
		text := row.Preview
		if i == selected && confirming {
			text = "再次按 Delete 确认删除"
		}
		label := text
		if i < 9 {
			label = fmt.Sprintf("%d  %s", i+1, text)
		}
		textRect := rectOf(pad+numberPad, top, wdt-pad-numberPad-rightPad, h)
		procDrawTextW.Call(mem, utf16Ptr(label), cchAuto, uintptr(unsafe.Pointer(textRect)),
			dtSingleLine|dtVCenter|dtEndEllipsis|dtNoPrefix)

		procSelectObject.Call(mem, muted)
		timeRect := rectOf(wdt-rightPad, top, rightPad-pad, h)
		procDrawTextW.Call(mem, utf16Ptr(formatTime(row.Timestamp)), cchAuto, uintptr(unsafe.Pointer(timeRect)),
			dtSingleLine|dtVCenter|dtRight|dtNoPrefix)
		if row.Pinned {
			pinBrush, _, _ := procCreateSolidBrush.Call(uintptr(theme.Accent))
			procSelectObject.Call(mem, pinBrush)
			pts := []struct{ x, y int32 }{
				{int32(wdt - scale(15)), int32(top + h/2 - scale(3))},
				{int32(wdt - scale(9)), int32(top + h/2)},
				{int32(wdt - scale(15)), int32(top + h/2 + scale(3))},
			}
			procPolygon.Call(mem, uintptr(unsafe.Pointer(&pts[0])), 3)
			procDeleteObject.Call(pinBrush)
		}
		top += h
	}
}

// ---- helpers -------------------------------------------------------------

func defaultTheme() Theme {
	// Dark theme by default; the resident applies real settings on start.
	return Theme{
		Background: 0x00202020,
		Foreground: 0x00FFFFFF,
		Muted:      0x00999999,
		Accent:     0x00D47800,
		AccentText: 0x00FFFFFF,
		Border:     0x00333333,
	}
}

func rectOf(x, y, w, h int) *struct{ left, top, right, bottom int32 } {
	return &struct{ left, top, right, bottom int32 }{
		int32(x), int32(y), int32(x + w), int32(y + h)}
}

func placeNearCursor(w, h int) (x, y int) {
	var pt struct{ x, y int32 }
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	monitor, _, _ := procMonitorFromPoint.Call(uintptr(unsafe.Pointer(&pt)), monitorDefaultToNearest)
	var info struct {
		cbSize    uint32
		rcMonitor struct{ left, top, right, bottom int32 }
		rcWork    struct{ left, top, right, bottom int32 }
		flags     uint32
	}
	info.cbSize = uint32(unsafe.Sizeof(info))
	procGetMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info)))
	wa := info.rcWork
	x = int(wa.left) + 10
	y = int(wa.top) + 10
	if int(pt.x)+w > int(wa.right)-10 {
		x = int(wa.right) - w - 10
	}
	if int(pt.y)+h > int(wa.bottom)-10 {
		y = int(wa.bottom) - h - 10
	}
	if x < int(wa.left) {
		x = int(wa.left)
	}
	if y < int(wa.top) {
		y = int(wa.top)
	}
	return x, y
}

func formatTime(ms int64) string {
	t := time.UnixMilli(ms)
	now := time.Now()
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return t.Format("15:04")
	}
	return t.Format("01-02 15:04")
}

func scale(v int) int {
	return v * systemDPI() / 96
}

func systemDPI() int {
	dpi, _, _ := procGetDpiForSystem.Call(0)
	if dpi == 0 {
		return 96
	}
	return int(dpi)
}

func createFont(sizePx int, weight int) (uintptr, error) {
	font, _, err := procCreateFontW.Call(
		uintptr(-scale(sizePx)), 0, 0, 0, uintptr(weight), 0, 0, 0,
		1, // DEFAULT_CHARSET: GDI font linking picks CJK/Latin faces
		0, 0, 4 /*ANTIALIASED_QUALITY*/, 0, utf16Ptr("Microsoft YaHei UI"))
	if font == 0 {
		return 0, err
	}
	return font, nil
}

func isComposing(hwnd uintptr) bool {
	himc, _, _ := procImmGetContext.Call(hwnd)
	if himc == 0 {
		return false
	}
	defer procImmReleaseContext.Call(hwnd, himc)
	length, _, _ := procImmGetCompositionStringW.Call(himc, gcsCompStr, 0, 0)
	return length > 0
}

func defWindowProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return ret
}

func callWindowProc(orig, hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	ret, _, _ := procCallWindowProcW.Call(orig, hwnd, uintptr(msg), wp, lp)
	return ret
}

func keyDown(vk uintptr) bool {
	state, _, _ := procGetKeyState.Call(vk)
	return state&0x8000 != 0
}

func currentThreadID() uintptr {
	id, _, _ := procGetCurrentThreadID.Call(0)
	return id
}

func loadCursor(id uintptr) uintptr {
	c, _, _ := procLoadCursorW.Call(0, id)
	return c
}

func utf16Ptr(s string) uintptr {
	u, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		return 0
	}
	return uintptr(unsafe.Pointer(u))
}

func getModuleHandle() uintptr {
	h, _, _ := procGetModuleHandleW.Call(0)
	return h
}

// ---- window plumbing -----------------------------------------------------

type wndClassEx struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   uintptr
	icon       uintptr
	cursor     uintptr
	background uintptr
	menuName   uintptr
	className  uintptr
	iconSm     uintptr
}

func windowFromHWND(hwnd uintptr) *Window {
	value, ok := windows.Load(hwnd)
	if !ok {
		return nil
	}
	return value.(*Window)
}

func windowFromEdit(hwnd uintptr) *Window {
	parent, _, _ := procGetParent.Call(hwnd)
	if parent == 0 {
		return nil
	}
	return windowFromHWND(parent)
}
