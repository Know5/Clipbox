package clipboard

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32                            = syscall.NewLazyDLL("user32.dll")
	kernel32                          = syscall.NewLazyDLL("kernel32.dll")
	procAddClipboardFormatListener    = user32.NewProc("AddClipboardFormatListener")
	procRemoveClipboardFormatListener = user32.NewProc("RemoveClipboardFormatListener")
	procCreateWindowEx                = user32.NewProc("CreateWindowExW")
	procDefWindowProc                 = user32.NewProc("DefWindowProcW")
	procRegisterClassEx               = user32.NewProc("RegisterClassExW")
	procGetMessage                    = user32.NewProc("GetMessageW")
	procTranslateMessage              = user32.NewProc("TranslateMessage")
	procDispatchMessage               = user32.NewProc("DispatchMessageW")
	procOpenClipboard                 = user32.NewProc("OpenClipboard")
	procCloseClipboard                = user32.NewProc("CloseClipboard")
	procGetClipboardData              = user32.NewProc("GetClipboardData")
	procIsClipboardFormatAvailable    = user32.NewProc("IsClipboardFormatAvailable")
	procSetClipboardData              = user32.NewProc("SetClipboardData")
	procEmptyClipboard                = user32.NewProc("EmptyClipboard")
	procGlobalLock                    = kernel32.NewProc("GlobalLock")
	procGlobalUnlock                  = kernel32.NewProc("GlobalUnlock")
	procGlobalAlloc                   = kernel32.NewProc("GlobalAlloc")
	procGlobalFree                    = kernel32.NewProc("GlobalFree")
	procGlobalSize                    = kernel32.NewProc("GlobalSize")
	procLstrcpy                       = kernel32.NewProc("lstrcpyW")
	procPostThreadMessageW            = user32.NewProc("PostThreadMessageW")
	procGetWindowThreadProcessId      = user32.NewProc("GetWindowThreadProcessId")
	procRegisterClipboardFormat       = user32.NewProc("RegisterClipboardFormatW")
)

const (
	CF_UNICODETEXT     = 13
	CF_DIB             = 8
	CF_BITMAP          = 2
	WM_CLIPBOARDUPDATE = 0x031D
	WM_DESTROY         = 0x0002
	WM_QUIT_MSG        = 0x0012
	GMEM_MOVEABLE      = 0x0002
	ERROR_CLASS_EXISTS = 1410
)

type ClipEntry struct {
	ID          int64  `json:"id"`
	Type        string `json:"type"`
	Content     string `json:"content"`
	Preview     string `json:"preview"`
	Thumbnail   string `json:"thumbnail,omitempty"`
	Timestamp   int64  `json:"timestamp"`
	Pinned      bool   `json:"pinned"`
	SourceApp   string `json:"sourceApp,omitempty"`
	SourceTitle string `json:"sourceTitle,omitempty"`
	SourcePath  string `json:"sourcePath,omitempty"`
	Note        string `json:"note,omitempty"`
	Tags        string `json:"tags,omitempty"`
}

type OnClipChangeFunc func(entry ClipEntry)

// imageDir is set during init so the clipboard package knows where to store image files.
var imageDir string

// selfWrite is set to 1 when the app itself is writing to the clipboard,
// so the watcher should ignore the next update(s).
var selfWrite int32

// SkipReason 解释为什么一次剪贴板更新没有产生可记录条目。
type SkipReason string

const (
	SkipReasonBusyClipboard   SkipReason = "clipboard busy"
	SkipReasonPrivateContent  SkipReason = "private content"
	SkipReasonOversizedText   SkipReason = "oversized text"
	SkipReasonOversizedImage  SkipReason = "oversized image"
	SkipReasonUnsupportedType SkipReason = "unsupported type"
	SkipReasonReadFailed      SkipReason = "read failed"
)

// OnClipSkipFunc 在跳过一次剪贴板更新时被调用，便于上层给用户可见反馈。
type OnClipSkipFunc func(reason SkipReason)

// SetImageDir configures where DIB image files will be stored.

func SetImageDir(dir string) {
	imageDir = dir
}

// MarkSelfWrite sets a flag so the watcher ignores the next clipboard update(s).
// Call this before programmatically writing to the clipboard.
func MarkSelfWrite() {
	atomic.StoreInt32(&selfWrite, 1)
}

// ClearSelfWrite clears the self-write marker if a programmatic clipboard
// write failed before Windows could emit a clipboard update.
func ClearSelfWrite() {
	atomic.StoreInt32(&selfWrite, 0)
}

// PanicLogger is set by the host application to receive panic reports from
// internal goroutines and syscall callbacks. If nil, panics are silently
// recovered.
var PanicLogger func(format string, args ...interface{})

func recoverPanic(name string) {
	if r := recover(); r != nil {
		if PanicLogger != nil {
			PanicLogger("panic in %s: %v", name, r)
		}
	}
}

// Windows 约定的剪贴板隐私格式：密码管理器（1Password、KeePass、Bitwarden 等）
// 复制敏感内容时会附带这些格式，剪贴板管理器应跳过记录。
var privacyFormatsOnce sync.Once
var (
	fmtExcludeMonitor    uintptr // ExcludeClipboardContentFromMonitorProcessing：存在即排除
	fmtCanIncludeHistory uintptr // CanIncludeInClipboardHistory：DWORD 值为 0 表示不进入历史
	fmtViewerIgnore      uintptr // Clipboard Viewer Ignore：旧约定（Ditto 等），存在即排除
)

func registerPrivacyFormats() {
	privacyFormatsOnce.Do(func() {
		fmtExcludeMonitor = registerClipboardFormat("ExcludeClipboardContentFromMonitorProcessing")
		fmtCanIncludeHistory = registerClipboardFormat("CanIncludeInClipboardHistory")
		fmtViewerIgnore = registerClipboardFormat("Clipboard Viewer Ignore")
	})
}

func registerClipboardFormat(name string) uintptr {
	ptr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return 0
	}
	id, _, _ := procRegisterClipboardFormat.Call(uintptr(unsafe.Pointer(ptr)))
	return id
}

// clipboardMarkedPrivate reports whether the current clipboard content is
// marked by its source app as excluded from history/monitoring.
// Caller must hold the clipboard open.
func clipboardMarkedPrivate() bool {
	registerPrivacyFormats()
	if fmtExcludeMonitor != 0 {
		if ret, _, _ := procIsClipboardFormatAvailable.Call(fmtExcludeMonitor); ret != 0 {
			return true
		}
	}
	if fmtViewerIgnore != 0 {
		if ret, _, _ := procIsClipboardFormatAvailable.Call(fmtViewerIgnore); ret != 0 {
			return true
		}
	}
	if fmtCanIncludeHistory != 0 {
		if ret, _, _ := procIsClipboardFormatAvailable.Call(fmtCanIncludeHistory); ret != 0 {
			if value, ok := readClipboardDword(fmtCanIncludeHistory); ok && value == 0 {
				return true
			}
		}
	}
	return false
}

func readClipboardDword(format uintptr) (uint32, bool) {
	h, _, _ := procGetClipboardData.Call(format)
	if h == 0 {
		return 0, false
	}
	sz, _, _ := procGlobalSize.Call(h)
	if sz < 4 {
		return 0, false
	}
	ptr, _, _ := procGlobalLock.Call(h)
	if ptr == 0 {
		return 0, false
	}
	defer procGlobalUnlock.Call(h)
	data := byteSliceFromPointer(ptr, 4)
	if len(data) < 4 {
		return 0, false
	}
	return uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24, true
}

type Watcher struct {
	hwnd      uintptr
	callback  OnClipChangeFunc
	onSkip    OnClipSkipFunc
	stopCh    chan struct{}
	stopOnce  sync.Once
	startOnce sync.Once
}

func NewWatcher(callback OnClipChangeFunc) *Watcher {
	return &Watcher{
		callback: callback,
		stopCh:   make(chan struct{}),
	}
}

// SetSkipCallback 注册跳过原因回调。nil 表示不报告跳过。
func (w *Watcher) SetSkipCallback(callback OnClipSkipFunc) {
	if w == nil {
		return
	}
	w.onSkip = callback
}

func (w *Watcher) reportSkip(reason SkipReason) {
	if w == nil || w.onSkip == nil {
		return
	}
	w.onSkip(reason)
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
	pt      struct{ x, y int32 }
}

type rawSliceHeader struct {
	data uintptr
	len  int
	cap  int
}

//go:nocheckptr
func byteSliceFromPointer(addr uintptr, length int) []byte {
	if addr == 0 || length <= 0 {
		return nil
	}
	header := rawSliceHeader{data: addr, len: length, cap: length}
	return *(*[]byte)(unsafe.Pointer(&header))
}

//go:nocheckptr
func uint16SliceFromPointer(addr uintptr, length int) []uint16 {
	if addr == 0 || length <= 0 {
		return nil
	}
	header := rawSliceHeader{data: addr, len: length, cap: length}
	return *(*[]uint16)(unsafe.Pointer(&header))
}

var globalWatcher *Watcher

func clipboardWndProc(hwnd uintptr, umsg uint32, wParam, lParam uintptr) uintptr {
	if umsg == WM_CLIPBOARDUPDATE {
		defer recoverPanic("clipboardWndProc")
		// Skip updates caused by our own clipboard writes.
		if atomic.CompareAndSwapInt32(&selfWrite, 1, 0) {
			return 0
		}
		if globalWatcher != nil && globalWatcher.callback != nil {
			entry, reason := readClipboard()
			if entry != nil {
				globalWatcher.callback(*entry)
			} else if reason != "" {
				globalWatcher.reportSkip(reason)
			}
		}
		return 0
	}
	ret, _, _ := procDefWindowProc.Call(hwnd, uintptr(umsg), wParam, lParam)
	return ret
}

func (w *Watcher) Start() error {
	globalWatcher = w
	initCh := make(chan error, 1)
	go w.messageLoop(initCh)

	select {
	case err := <-initCh:
		return err
	case <-time.After(3 * time.Second):
		return fmt.Errorf("clipboard watcher did not finish startup in time")
	}
}

func (w *Watcher) Stop() {
	w.stopOnce.Do(func() {
		close(w.stopCh)
		if w.hwnd != 0 {
			threadID, _, _ := procGetWindowThreadProcessId.Call(w.hwnd, 0)
			procPostThreadMessageW.Call(threadID, WM_QUIT_MSG, 0, 0)
			procRemoveClipboardFormatListener.Call(w.hwnd)
		}
	})
}

func (w *Watcher) messageLoop(initCh chan<- error) {
	defer recoverPanic("clipboard:messageLoop")
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	className, _ := syscall.UTF16PtrFromString("ClipboxWatcher")

	wcx := wndClassEx{
		wndProc:   syscall.NewCallback(clipboardWndProc),
		className: className,
	}
	wcx.size = uint32(unsafe.Sizeof(wcx))

	ret, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wcx)))
	if ret == 0 && err != syscall.Errno(ERROR_CLASS_EXISTS) {
		initCh <- fmt.Errorf("register clipboard watcher window class: %w", callError(err))
		return
	}

	hwnd, _, _ := procCreateWindowEx.Call(
		0, uintptr(unsafe.Pointer(className)), 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0,
	)
	if hwnd == 0 {
		initCh <- fmt.Errorf("create clipboard watcher window: %w", syscall.GetLastError())
		return
	}
	w.hwnd = hwnd

	ret, _, err = procAddClipboardFormatListener.Call(hwnd)
	if ret == 0 {
		initCh <- fmt.Errorf("add clipboard format listener: %w", callError(err))
		return
	}
	initCh <- nil

	var m msg
	for {
		select {
		case <-w.stopCh:
			return
		default:
			ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if ret == 0 {
				return
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
		}
	}
}
func readClipboard() (*ClipEntry, SkipReason) {
	ret, _, _ := procOpenClipboard.Call(0)
	if ret == 0 {
		return nil, SkipReasonBusyClipboard
	}
	defer procCloseClipboard.Call()

	// 尊重来源应用的隐私标记（密码管理器等），跳过记录。
	if clipboardMarkedPrivate() {
		return nil, SkipReasonPrivateContent
	}

	if ret, _, _ := procIsClipboardFormatAvailable.Call(CF_DIB); ret != 0 {
		return readImageFromClipboard()
	}

	if ret, _, _ := procIsClipboardFormatAvailable.Call(CF_UNICODETEXT); ret != 0 {
		return readTextFromClipboard()
	}

	return nil, SkipReasonUnsupportedType
}
func readTextFromClipboard() (*ClipEntry, SkipReason) {
	h, _, _ := procGetClipboardData.Call(CF_UNICODETEXT)
	if h == 0 {
		return nil, SkipReasonReadFailed
	}

	// Get the actual size of the global memory block.
	sz, _, _ := procGlobalSize.Call(h)
	if sz == 0 {
		return nil, SkipReasonReadFailed
	}
	if sz > 10*1024*1024 { // sanity: refuse >10 MB of text
		return nil, SkipReasonOversizedText
	}

	ptr, _, _ := procGlobalLock.Call(h)
	if ptr == 0 {
		return nil, SkipReasonReadFailed
	}
	defer procGlobalUnlock.Call(h)

	// Use unsafe.Slice instead of a massive [1<<20]uint16 array to avoid
	// creating a huge array type in the compiled binary.
	count := int(sz / 2)
	if count < 1 {
		return nil, SkipReasonReadFailed
	}
	utf16Slice := uint16SliceFromPointer(ptr, count)
	text := syscall.UTF16ToString(utf16Slice)
	if text == "" {
		return nil, SkipReasonReadFailed
	}

	preview := truncateRunes(text, 200)

	return &ClipEntry{
		Type:      "text",
		Content:   text,
		Preview:   preview,
		Timestamp: time.Now().UnixMilli(),
	}, ""
}

func readImageFromClipboard() (*ClipEntry, SkipReason) {
	h, _, _ := procGetClipboardData.Call(CF_DIB)
	if h == 0 {
		return nil, SkipReasonReadFailed
	}

	size, _, _ := procGlobalSize.Call(h)
	if size == 0 {
		return nil, SkipReasonReadFailed
	}
	if size > 50*1024*1024 { // sanity: refuse >50 MB images
		return nil, SkipReasonOversizedImage
	}

	ptr, _, _ := procGlobalLock.Call(h)
	if ptr == 0 {
		return nil, SkipReasonReadFailed
	}
	defer procGlobalUnlock.Call(h)

	// Use unsafe.Slice — the old (*[1<<30]byte)(...) cast created a 1 GB
	// array type that caused massive memory pressure / GC stalls.
	dibData := make([]byte, size)
	copy(dibData, byteSliceFromPointer(ptr, int(size)))

	// Save DIB to a file on disk — never store in SQLite as base64.
	// Screenshots can be 8+ MB raw; base64 inflates that another 33 %.
	filePath := ""
	if imageDir != "" {
		os.MkdirAll(imageDir, 0755)
		fname := filepath.Join(imageDir, uniqueFileName())
		if err := os.WriteFile(fname, dibData, 0644); err == nil {
			filePath = fname
		}
	}
	if filePath == "" {
		return nil, SkipReasonReadFailed
	}

	thumbnail, _ := DibToThumbnailDataURL(dibData, 160)
	preview := "[图片]"

	return &ClipEntry{
		Type:      "image",
		Content:   filePath, // file path on disk, NOT base64
		Preview:   preview,
		Thumbnail: thumbnail,
		Timestamp: time.Now().UnixMilli(),
	}, ""
}

var fileNameCounter int64

func uniqueFileName() string {
	n := atomic.AddInt64(&fileNameCounter, 1)
	return strconv.FormatInt(time.Now().UnixMilli(), 36) +
		"_" + strconv.FormatInt(n, 36) + ".dib"
}

func truncateRunes(text string, max int) string {
	if max <= 0 {
		return ""
	}
	count := 0
	for i := range text {
		if count == max {
			return text[:i]
		}
		count++
	}
	return text
}

// GetImageDataURL reads the DIB file for a clip and returns a
// `data:image/bmp;base64,...` string for frontend display.
func GetImageDataURL(filePath string) (string, error) {
	dibData, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	bmpData := dibToBmp(dibData)
	if bmpData == nil {
		return "", nil
	}
	return "data:image/bmp;base64," + base64.StdEncoding.EncodeToString(bmpData), nil
}

// DibToThumbnailDataURL converts common 24/32-bit uncompressed DIB data into
// a compact PNG data URL for list previews.
func DibToThumbnailDataURL(dib []byte, maxSide int) (string, error) {
	if maxSide <= 0 {
		maxSide = 160
	}

	info, err := parseDibInfo(dib)
	if err != nil {
		return "", err
	}
	if info.width <= 0 || info.height <= 0 {
		return "", fmt.Errorf("invalid DIB dimensions")
	}
	if info.bitCount != 24 && info.bitCount != 32 {
		return "", fmt.Errorf("unsupported DIB bit depth: %d", info.bitCount)
	}
	if info.compression != 0 && info.compression != 3 {
		return "", fmt.Errorf("unsupported DIB compression: %d", info.compression)
	}

	dstW, dstH := scaledDimensions(info.width, info.height, maxSide)
	img := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	for y := 0; y < dstH; y++ {
		srcY := y * info.height / dstH
		for x := 0; x < dstW; x++ {
			srcX := x * info.width / dstW
			c, ok := dibPixel(dib, info, srcX, srcY)
			if !ok {
				return "", fmt.Errorf("DIB pixel data out of range")
			}
			img.SetRGBA(x, y, c)
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// LoadDibFromFile reads a DIB file from disk and returns the raw bytes.
func LoadDibFromFile(filePath string) ([]byte, error) {
	return os.ReadFile(filePath)
}

// ── BMP / DIB conversion ────────────────────────────────────────────

type dibInfo struct {
	headerSize  int
	width       int
	height      int
	topDown     bool
	bitCount    uint16
	compression uint32
	pixelOffset int
	rowStride   int
}

func parseDibInfo(dib []byte) (dibInfo, error) {
	if len(dib) < 40 {
		return dibInfo{}, fmt.Errorf("DIB too small")
	}

	headerSize := int(readU32(dib, 0))
	if headerSize < 40 || headerSize > len(dib) {
		return dibInfo{}, fmt.Errorf("invalid DIB header size")
	}

	width := int(readI32(dib, 4))
	rawHeight := readI32(dib, 8)
	height := int(rawHeight)
	topDown := false
	if height < 0 {
		topDown = true
		height = -height
	}
	if width <= 0 || height <= 0 {
		return dibInfo{}, fmt.Errorf("invalid DIB dimensions")
	}

	bitCount := readU16(dib, 14)
	compression := readU32(dib, 16)
	pixelOffset := headerSize
	clrUsed := uint32(0)
	if len(dib) >= 36 {
		clrUsed = readU32(dib, 32)
	}
	if clrUsed > 0 {
		pixelOffset += int(clrUsed * 4)
	} else if bitCount <= 8 {
		pixelOffset += int((1 << bitCount) * 4)
	} else if compression == 3 {
		pixelOffset += 12
	}

	bytesPerPixel := int(bitCount / 8)
	if bytesPerPixel <= 0 {
		return dibInfo{}, fmt.Errorf("invalid DIB bit depth")
	}
	rowStride := ((width*bytesPerPixel + 3) / 4) * 4
	if pixelOffset < 0 || pixelOffset+rowStride*height > len(dib) {
		return dibInfo{}, fmt.Errorf("DIB pixel data truncated")
	}

	return dibInfo{
		headerSize:  headerSize,
		width:       width,
		height:      height,
		topDown:     topDown,
		bitCount:    bitCount,
		compression: compression,
		pixelOffset: pixelOffset,
		rowStride:   rowStride,
	}, nil
}

func readU16(data []byte, offset int) uint16 {
	return uint16(data[offset]) | uint16(data[offset+1])<<8
}

func readU32(data []byte, offset int) uint32 {
	return uint32(data[offset]) |
		uint32(data[offset+1])<<8 |
		uint32(data[offset+2])<<16 |
		uint32(data[offset+3])<<24
}

func readI32(data []byte, offset int) int32 {
	return int32(readU32(data, offset))
}

func scaledDimensions(width, height, maxSide int) (int, int) {
	if width <= maxSide && height <= maxSide {
		return width, height
	}
	if width >= height {
		dstW := maxSide
		dstH := height * maxSide / width
		if dstH < 1 {
			dstH = 1
		}
		return dstW, dstH
	}
	dstH := maxSide
	dstW := width * maxSide / height
	if dstW < 1 {
		dstW = 1
	}
	return dstW, dstH
}

func dibPixel(dib []byte, info dibInfo, x, y int) (color.RGBA, bool) {
	if x < 0 || x >= info.width || y < 0 || y >= info.height {
		return color.RGBA{}, false
	}
	fileY := y
	if !info.topDown {
		fileY = info.height - 1 - y
	}

	bytesPerPixel := int(info.bitCount / 8)
	offset := info.pixelOffset + fileY*info.rowStride + x*bytesPerPixel
	if offset < 0 || offset+bytesPerPixel > len(dib) {
		return color.RGBA{}, false
	}

	b := dib[offset]
	g := dib[offset+1]
	r := dib[offset+2]
	a := uint8(255)
	if info.bitCount == 32 {
		a = dib[offset+3]
		if a == 0 {
			a = 255
		}
	}
	return color.RGBA{R: r, G: g, B: b, A: a}, true
}

func dibToBmp(dib []byte) []byte {
	if len(dib) < 40 {
		return nil
	}
	biSize := uint32(dib[0]) | uint32(dib[1])<<8 | uint32(dib[2])<<16 | uint32(dib[3])<<24

	offBits := uint32(14) + biSize

	var bitCount uint16
	if len(dib) >= 16 {
		bitCount = uint16(dib[14]) | uint16(dib[15])<<8
	}

	var compression uint32
	if len(dib) >= 20 {
		compression = uint32(dib[16]) | uint32(dib[17])<<8 | uint32(dib[18])<<16 | uint32(dib[19])<<24
	}

	var clrUsed uint32
	if len(dib) >= 36 {
		clrUsed = uint32(dib[32]) | uint32(dib[33])<<8 | uint32(dib[34])<<16 | uint32(dib[35])<<24
	}

	if clrUsed > 0 {
		offBits += clrUsed * 4
	} else if bitCount <= 8 {
		offBits += (1 << bitCount) * 4
	} else if compression == 3 {
		offBits += 12
	}

	fileSize := uint32(len(dib)) + 14

	bmp := make([]byte, fileSize)
	bmp[0] = 'B'
	bmp[1] = 'M'
	bmp[2] = byte(fileSize)
	bmp[3] = byte(fileSize >> 8)
	bmp[4] = byte(fileSize >> 16)
	bmp[5] = byte(fileSize >> 24)
	bmp[10] = byte(offBits)
	bmp[11] = byte(offBits >> 8)
	bmp[12] = byte(offBits >> 16)
	bmp[13] = byte(offBits >> 24)

	copy(bmp[14:], dib)
	return bmp
}

func DibToBmp(dib []byte) []byte {
	return dibToBmp(dib)
}

func BmpToDib(bmp []byte) []byte {
	if len(bmp) < 14 {
		return nil
	}
	return bmp[14:]
}

// ── Clipboard write helpers ──────────────────────────────────────────

func callError(err error) error {
	if err != nil && err != syscall.Errno(0) {
		return err
	}
	if last := syscall.GetLastError(); last != syscall.Errno(0) {
		return last
	}
	return syscall.EINVAL
}

// WriteDibToClipboard writes raw DIB data directly to the clipboard.
func WriteDibToClipboard(dibData []byte) error {
	if len(dibData) == 0 {
		return syscall.EINVAL
	}

	ret, _, _ := procOpenClipboard.Call(0)
	if ret == 0 {
		return syscall.GetLastError()
	}
	defer procCloseClipboard.Call()

	ret, _, err := procEmptyClipboard.Call()
	if ret == 0 {
		return callError(err)
	}

	size := len(dibData)
	hMem, _, err := procGlobalAlloc.Call(GMEM_MOVEABLE, uintptr(size))
	if hMem == 0 {
		return callError(err)
	}

	ptr, _, err := procGlobalLock.Call(hMem)
	if ptr == 0 {
		procGlobalFree.Call(hMem)
		return callError(err)
	}

	copy(byteSliceFromPointer(ptr, size), dibData)
	procGlobalUnlock.Call(hMem)

	ret, _, err = procSetClipboardData.Call(CF_DIB, hMem)
	if ret == 0 {
		procGlobalFree.Call(hMem)
		return callError(err)
	}
	return nil
}

func WriteTextToClipboard(text string) error {
	ret, _, _ := procOpenClipboard.Call(0)
	if ret == 0 {
		return syscall.GetLastError()
	}
	defer procCloseClipboard.Call()

	ret, _, err := procEmptyClipboard.Call()
	if ret == 0 {
		return callError(err)
	}

	utf16, err := syscall.UTF16FromString(text)
	if err != nil {
		return err
	}
	size := len(utf16) * 2

	hMem, _, err := procGlobalAlloc.Call(GMEM_MOVEABLE, uintptr(size))
	if hMem == 0 {
		return callError(err)
	}

	ptr, _, err := procGlobalLock.Call(hMem)
	if ptr == 0 {
		procGlobalFree.Call(hMem)
		return callError(err)
	}

	copy(uint16SliceFromPointer(ptr, len(utf16)), utf16)
	procGlobalUnlock.Call(hMem)

	ret, _, err = procSetClipboardData.Call(CF_UNICODETEXT, hMem)
	if ret == 0 {
		procGlobalFree.Call(hMem)
		return callError(err)
	}
	return nil
}

func WriteImageToClipboard(dibData []byte) error {
	return WriteDibToClipboard(dibData)
}
