package main

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

var (
	dlgUser32        = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW  = dlgUser32.NewProc("MessageBoxW")
	procShellExecute = syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW")
)

const (
	mbOK             = 0x00000000
	mbYesNo          = 0x00000004
	mbIconError      = 0x00000010
	mbIconWarning    = 0x00000030
	mbSetForeground  = 0x00010000
	mbTopMost        = 0x00040000
	idYes            = 6
	webView2FwLink   = "https://go.microsoft.com/fwlink/p/?LinkId=2124703"
	webView2ClientID = `SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`
)

// showFatalStartupError shows a blocking native error box when the app cannot
// even establish a data directory. Uses MessageBoxW directly because the Wails
// runtime (and its dialog API) is not up yet at this point.
func showFatalStartupError(err error) {
	messageBox(appName+" 无法启动", err.Error(), mbOK|mbIconError)
}

// ensureWebView2 checks for the WebView2 runtime before wails.Run. Without it
// the window renders blank with no explanation on machines that lack the
// Evergreen runtime (Windows LTSC/Server/N SKUs). If missing, offer to open the
// Microsoft download page. Returns false if the user declined and we should not
// continue into a blank window.
func ensureWebView2() bool {
	if webView2Installed() {
		return true
	}
	const msg = "ClipBox 需要 Microsoft Edge WebView2 运行时才能显示界面，当前系统未检测到该组件。\n\n" +
		"是否现在打开微软官方下载页面？下载并安装 “Evergreen Bootstrapper” 后重新启动 ClipBox 即可。"
	if messageBox("缺少 WebView2 运行时", msg, mbYesNo|mbIconWarning) == idYes {
		openURL(webView2FwLink)
	}
	return false
}

// webView2Installed reports whether the Evergreen WebView2 runtime is present
// by reading its version from the per-machine and per-user EdgeUpdate keys.
func webView2Installed() bool {
	roots := []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER}
	paths := []string{
		webView2ClientID,
		`SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`,
	}
	for _, root := range roots {
		for _, path := range paths {
			if v := regStringValue(root, path, "pv"); v != "" && v != "0.0.0.0" {
				return true
			}
		}
	}
	return false
}

func regStringValue(root registry.Key, path, name string) string {
	key, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()
	value, _, err := key.GetStringValue(name)
	if err != nil {
		return ""
	}
	return value
}

func messageBox(title, text string, flags uint32) int {
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	textPtr, _ := syscall.UTF16PtrFromString(text)
	ret, _, _ := procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(textPtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(flags|mbSetForeground|mbTopMost),
	)
	return int(ret)
}

func openURL(url string) {
	verb, _ := syscall.UTF16PtrFromString("open")
	target, _ := syscall.UTF16PtrFromString(url)
	procShellExecute.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(target)), 0, 0, uintptr(swShowNormal))
}

const swShowNormal = 1
