package main

import (
	"embed"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"clipbox/internal/clipboard"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	debug.SetMemoryLimit(50 * 1024 * 1024)
	// WebView2 reads WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS when the environment
	// is created; the UI process inherits it from this process. Keep an
	// existing value intact so a user can override or extend it.
	if os.Getenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS") == "" {
		os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", webviewMemoryArgs)
	}
	if bindingBuild() {
		_ = runUI(&UIBridge{local: NewApp("", true)})
		return
	}
	if hasArg("--ui") {
		bridge, err := newUIBridge()
		if err != nil {
			showFatalStartupError(err)
			return
		}
		if !ensureWebView2() {
			return
		}
		if err := runUI(bridge); err != nil {
			showFatalStartupError(err)
		}
		return
	}
	dataDir, err := resolveDataDir()
	if err != nil {
		showFatalStartupError(err)
		return
	}
	imageDir := filepath.Join(dataDir, "images")
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		showFatalStartupError(err)
		return
	}
	clipboard.SetImageDir(imageDir)
	if developmentBuild() {
		app := NewApp(dataDir, hasArg("--hidden"))
		if err := runUI(&UIBridge{local: app, direct: app}); err != nil {
			showFatalStartupError(err)
		}
		return
	}
	if err := runResident(dataDir, hasArg("--hidden")); err != nil {
		showFatalStartupError(err)
	}
}

func runUI(bridge *UIBridge) error {
	os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", "--renderer-process-limit=1")
	startHidden := true
	if bridge.direct != nil {
		startHidden = bridge.local.startHidden
	}
	return wails.Run(&options.App{
		Title:             appName,
		Width:             defaultWindowWidth,
		Height:            defaultWindowHeight,
		MinWidth:          340,
		MinHeight:         420,
		AlwaysOnTop:       true,
		HideWindowOnClose: false,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: http.HandlerFunc(bridge.local.serveImage),
		},
		// 与 App.css 的 --bg-primary 一致；前端切主题时再调 WindowSetBackgroundColour。
		BackgroundColour: &options.RGBA{R: 32, G: 32, B: 32, A: 255},
		OnStartup:        bridge.startup,
		OnShutdown:       bridge.shutdown,
		OnBeforeClose:    bridge.beforeClose,
		StartHidden:      startHidden,
		Frameless:        true,
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			WebviewBrowserPath:   "",
			WebviewUserDataPath:  filepath.Join(bridge.local.dataDir, "webview"),
			// 2026-10-29: 原为 true(CPU 软渲染)，每次从隐藏到显示要重画整块窗口，
			// 是"打开后空白一下才出内容"的主要嫌疑。改回 GPU 合成；若出现黑窗/闪烁，改回 true。
			WebviewGpuIsDisabled:              false,
			DisableFramelessWindowDecorations: false,
			WindowClassName:                   mainWindowClass,
		},
		Bind: []interface{}{
			bridge,
		},
	})

}

func hasArg(name string) bool {
	for _, arg := range os.Args[1:] {
		if arg == name {
			return true
		}
	}
	return false
}

// resolveDataDir picks a writable per-user directory for ClipBox data.
// It never returns a relative path: if %USERPROFILE% is unset (so that
// os.UserHomeDir fails or is empty) it falls back to the OS config dir and
// finally to the executable's own folder, so a stranger's data can never
// scatter into System32 or an arbitrary working directory.
func resolveDataDir() (string, error) {
	for i, arg := range os.Args[1:] {
		if arg == "--data-dir" && i+2 < len(os.Args) {
			dir, err := filepath.Abs(os.Args[i+2])
			if err != nil {
				return "", err
			}
			if err := os.MkdirAll(dir, 0755); err != nil {
				return "", err
			}
			return dir, nil
		}
	}

	base := ""
	if home, err := os.UserHomeDir(); err == nil {
		base = strings.TrimSpace(home)
	}
	if !filepath.IsAbs(base) {
		if cfg, err := os.UserConfigDir(); err == nil && filepath.IsAbs(cfg) {
			base = cfg
		} else if exe, err := os.Executable(); err == nil {
			base = filepath.Dir(exe)
		}
	}
	if !filepath.IsAbs(base) {
		return "", fmt.Errorf("无法确定可写的数据目录：请确认当前用户目录可访问")
	}
	dataDir := filepath.Join(base, ".clipbox")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return "", fmt.Errorf("无法创建数据目录 %s：%w", dataDir, err)
	}
	return dataDir, nil
}
