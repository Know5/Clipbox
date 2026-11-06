package windowutil

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"os"
	"syscall"
	"unsafe"
)

var (
	shell32            = syscall.NewLazyDLL("shell32.dll")
	gdi32              = syscall.NewLazyDLL("gdi32.dll")
	procExtractIconEx  = shell32.NewProc("ExtractIconExW")
	procDestroyIcon    = user32.NewProc("DestroyIcon")
	procGetIconInfo    = user32.NewProc("GetIconInfo")
	procGetObject      = gdi32.NewProc("GetObjectW")
	procCreateCompatDC = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC       = gdi32.NewProc("DeleteDC")
	procGetDIBits      = gdi32.NewProc("GetDIBits")
	procDeleteObject   = gdi32.NewProc("DeleteObject")
)

const dibRGBColors = 0

type iconInfo struct {
	fIcon    int32
	xHotspot uint32
	yHotspot uint32
	hbmMask  uintptr
	hbmColor uintptr
}

type bitmap struct {
	bmType       int32
	bmWidth      int32
	bmHeight     int32
	bmWidthBytes int32
	bmPlanes     uint16
	bmBitsPixel  uint16
	bmBits       uintptr
}

type bitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

// bitmapInfo carries the header plus room for the palette GetDIBits writes when
// converting 1-bit icons; passing a bare header would overflow.
type bitmapInfo struct {
	header bitmapInfoHeader
	colors [256]uint32
}

// ExtractIconDataURL extracts the first icon of exePath (preferring the large
// 32x32 variant) and returns it as a PNG data URL. It returns "" with a nil
// error when the path is empty, the file is missing, or the executable has no
// icon.
func ExtractIconDataURL(exePath string) (string, error) {
	if exePath == "" {
		return "", nil
	}
	if _, err := os.Stat(exePath); err != nil {
		return "", nil
	}

	pathPtr, err := syscall.UTF16PtrFromString(exePath)
	if err != nil {
		return "", err
	}

	var large, small uintptr
	ret, _, _ := procExtractIconEx.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		uintptr(unsafe.Pointer(&large)),
		uintptr(unsafe.Pointer(&small)),
		1,
	)
	if ret == 0 || (large == 0 && small == 0) {
		destroyIcon(large)
		destroyIcon(small)
		return "", nil
	}

	icon := large
	spare := small
	if icon == 0 {
		icon = small
		spare = large
	}
	destroyIcon(spare)
	defer destroyIcon(icon)

	var info iconInfo
	ok, _, err := procGetIconInfo.Call(icon, uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		return "", callError(err)
	}
	defer deleteObject(info.hbmMask)
	defer deleteObject(info.hbmColor)
	if info.hbmColor == 0 {
		// Monochrome icons (rare) carry only a mask; nothing useful to render.
		return "", nil
	}

	var bm bitmap
	got, _, err := procGetObject.Call(
		info.hbmColor,
		unsafe.Sizeof(bm),
		uintptr(unsafe.Pointer(&bm)),
	)
	if got == 0 {
		return "", callError(err)
	}
	width, height := int(bm.bmWidth), int(bm.bmHeight)
	if width <= 0 || height <= 0 {
		return "", nil
	}

	hdc, _, err := procCreateCompatDC.Call(0)
	if hdc == 0 {
		return "", callError(err)
	}
	defer procDeleteDC.Call(hdc)

	img, err := iconToNRGBA(hdc, info, width, height)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// iconToNRGBA reads the icon's colour bitmap as 32-bit BGRA and derives
// transparency from the alpha channel, falling back to the AND mask when the
// icon predates per-pixel alpha.
func iconToNRGBA(hdc uintptr, info iconInfo, width, height int) (*image.NRGBA, error) {
	colorInfo := bitmapInfo{header: bitmapInfoHeader{
		biSize:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		biWidth:       int32(width),
		biHeight:      int32(height),
		biPlanes:      1,
		biBitCount:    32,
		biCompression: 0, // BI_RGB
	}}
	pixels := make([]byte, width*height*4)
	lines, _, err := procGetDIBits.Call(
		hdc,
		info.hbmColor,
		0,
		uintptr(height),
		uintptr(unsafe.Pointer(&pixels[0])),
		uintptr(unsafe.Pointer(&colorInfo)),
		dibRGBColors,
	)
	if int(lines) != height {
		return nil, fmt.Errorf("GetDIBits color copied %d/%d lines: %v", lines, height, callError(err))
	}

	hasAlpha := false
	for i := 3; i < len(pixels); i += 4 {
		if pixels[i] != 0 {
			hasAlpha = true
			break
		}
	}

	var mask []byte
	maskStride := ((width + 31) / 32) * 4
	if !hasAlpha && info.hbmMask != 0 {
		maskInfo := bitmapInfo{header: bitmapInfoHeader{
			biSize:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			biWidth:       int32(width),
			biHeight:      int32(height),
			biPlanes:      1,
			biBitCount:    1,
			biCompression: 0,
		}}
		mask = make([]byte, maskStride*height)
		maskLines, _, _ := procGetDIBits.Call(
			hdc,
			info.hbmMask,
			0,
			uintptr(height),
			uintptr(unsafe.Pointer(&mask[0])),
			uintptr(unsafe.Pointer(&maskInfo)),
			dibRGBColors,
		)
		if int(maskLines) != height {
			mask = nil
		}
	}

	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		// GetDIBits returns bottom-up rows for a positive biHeight.
		srcRow := (height - 1 - y) * width * 4
		for x := range width {
			src := srcRow + x*4
			dst := img.PixOffset(x, y)
			img.Pix[dst+0] = pixels[src+2]
			img.Pix[dst+1] = pixels[src+1]
			img.Pix[dst+2] = pixels[src+0]
			alpha := byte(255)
			if hasAlpha {
				alpha = pixels[src+3]
			} else if mask != nil {
				row := (height - 1 - y) * maskStride
				if mask[row+x/8]&(0x80>>uint(x%8)) != 0 {
					alpha = 0
				}
			}
			img.Pix[dst+3] = alpha
		}
	}
	return img, nil
}

func destroyIcon(icon uintptr) {
	if icon != 0 {
		procDestroyIcon.Call(icon)
	}
}

func deleteObject(obj uintptr) {
	if obj != 0 {
		procDeleteObject.Call(obj)
	}
}

// callError turns a failed syscall.Proc.Call result into a usable error; the
// errno returned by Call may be zero when the API reported failure another way.
func callError(err error) error {
	if errno, ok := err.(syscall.Errno); ok {
		if errno != 0 {
			return errno
		}
		if last := syscall.GetLastError(); last != nil && last != syscall.Errno(0) {
			return last
		}
		return syscall.EINVAL
	}
	if err == nil {
		return syscall.EINVAL
	}
	return err
}
