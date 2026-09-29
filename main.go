package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"clipbox/internal/clipboard"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Limit Go heap to 50 MiB for a lightweight clipboard tool.
	// When near the limit the GC runs more aggressively and returns memory to the OS.
	debug.SetMemoryLimit(50 * 1024 * 1024)

	// Periodically force the Go runtime to return unused heap to the OS.
	// Without this the working set can stay at peak levels even after GC.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				println("clipbox: panic in FreeOSMemory loop:", fmt.Sprint(r))
			}
		}()
		for {
			time.Sleep(30 * time.Second)
			debug.FreeOSMemory()
		}
	}()

	dataDir, dataErr := resolveDataDir()
	if dataErr != nil {
		// No writable location resolved — the app cannot store history.
		// Surface it rather than silently scattering data into the CWD.
		showFatalStartupError(dataErr)
		return
	}

	// Images are stored as raw DIB files on disk, NOT as base64 in SQLite.
	// A single screenshot can be 8+ MB; base64 inflates that another 33 %.
	// By keeping images as files we avoid loading them into memory on every
	// list query and prevent SQLite bloat.
	imageDir := filepath.Join(dataDir, "images")
	os.MkdirAll(imageDir, 0755)
	clipboard.SetImageDir(imageDir)

	// Let WebView2 share the system runtime (broker, GPU, network processes)
	// instead of spawning its own. This saves ~130+ MB because multiple
	// WebView2 apps on the same machine share those processes.
	// Downsides: none for a clipboard tool (no sensitive browsing data).
	os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS",
		"--renderer-process-limit=1") // one renderer is enough for a 400×560 window

	// WebView2 is required to render the UI. Detect it before wails.Run so we
	// can point the user at the installer instead of showing a blank window.
	if !ensureWebView2() {
		return
	}

	startHidden := hasArg("--hidden")
	app := NewApp(dataDir, startHidden)

	err := wails.Run(&options.App{
		Title:             appName,
		Width:             400,
		Height:            560,
		MinWidth:          340,
		MinHeight:         420,
		AlwaysOnTop:       true,
		HideWindowOnClose: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 30, G: 30, B: 30, A: 255},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		StartHidden:      startHidden,
		Frameless:        true,
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: singleInstanceID,
			OnSecondInstanceLaunch: func(_ options.SecondInstanceData) {
				app.showFromSecondInstance()
			},
		},
		Windows: &windows.Options{
			WebviewIsTransparent:              false,
			WindowIsTranslucent:               false,
			WebviewBrowserPath:                "",
			WebviewUserDataPath:               "", // share system WebView2 runtime
			WebviewGpuIsDisabled:              true,
			DisableFramelessWindowDecorations: false,
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
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
