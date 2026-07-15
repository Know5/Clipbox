package main

import (
	"context"
	"strconv"
	"sync"
	"time"

	"clipbox/internal/clipboard"
	"clipbox/internal/hotkey"
	"clipbox/internal/storage"
	"clipbox/internal/tray"
	"clipbox/internal/windowutil"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx            context.Context
	dataDir        string
	logPath        string
	store          *storage.Store
	watcher        *clipboard.Watcher
	hotkeyManager  *hotkey.Manager
	trayController *tray.Controller
	windowVisible  bool
	windowPinned   bool // when true, window stays visible after copy
	startHidden    bool
	lastTarget     uintptr
	lastToggle     time.Time
	mu             sync.Mutex
	settingsMu     sync.Mutex
	settingsCache  *AppSettings // 内存缓存：避免每次剪贴板事件/托盘刷新都跑 16 条 SQL
	foregroundInfo func() windowutil.ForegroundInfo
	windowInfo     func(hwnd uintptr) windowutil.ForegroundInfo
}

func NewApp(dataDir string, startHidden bool) *App {
	return &App{
		dataDir:        dataDir,
		logPath:        storageLogPath(dataDir),
		startHidden:    startHidden,
		foregroundInfo: windowutil.CurrentForegroundInfo,
		windowInfo:     windowutil.WindowInfo,
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.logInfof("startup app=%s version=%s dataDir=%s startHidden=%v", appName, appVersion, a.dataDir, a.startHidden)

	// Wire panic loggers so internal packages can report panics via the app log.
	clipboard.PanicLogger = a.logErrorf
	hotkey.PanicLogger = a.logErrorf

	store, err := storage.NewStore(a.dataDir)
	if err != nil {
		runtime.LogErrorf(ctx, "Failed to init store: %v", err)
		a.logErrorf("failed to init store: %v", err)
		return
	}
	a.store = store

	a.watcher = clipboard.NewWatcher(func(entry clipboard.ClipEntry) {
		prepared, ok := a.prepareClipEntry(entry)
		if !ok {
			a.store.DiscardEntryFiles(entry)
			return
		}
		entry = prepared

		id, err := a.store.SaveOrUpdate(entry)
		if err != nil {
			runtime.LogErrorf(ctx, "Failed to save/update clip: %v", err)
			a.logErrorf("failed to save/update clip type=%s previewLen=%d: %v", entry.Type, len([]rune(entry.Preview)), err)
			return
		}
		if err := a.applyCurrentCleanup(); err != nil {
			runtime.LogErrorf(ctx, "Failed to cleanup clips: %v", err)
			a.logErrorf("failed to cleanup clips: %v", err)
		}
		entry.ID = id
		if entry.Type == "image" {
			entry.Content = ""
		}
		// clip:new 已携带完整条目，前端会增量合并；
		// 不再广播 clips:changed，避免把已翻页的列表重置回第一页。
		runtime.EventsEmit(ctx, "clip:new", entry)
	})

	if err := a.watcher.Start(); err != nil {
		runtime.LogErrorf(ctx, "Failed to start clipboard watcher: %v", err)
		a.logErrorf("failed to start clipboard watcher: %v", err)
		runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "剪贴板监听启动失败",
			Message: err.Error(),
		})
	}

	modStr, _ := a.store.GetSetting("hotkey_modifiers")
	vkStr, _ := a.store.GetSetting("hotkey_key")
	mod := uint32(hotkey.ModControl | hotkey.ModAlt)
	vk := uint32(0x56)
	if modStr != "" {
		if v, err := strconv.ParseUint(modStr, 10, 32); err == nil {
			mod = uint32(v)
		}
	}
	if vkStr != "" {
		if v, err := strconv.ParseUint(vkStr, 10, 32); err == nil {
			vk = uint32(v)
		}
	}

	a.hotkeyManager = hotkey.NewManager()
	if err := a.hotkeyManager.Start(mod, vk); err != nil {
		a.logErrorf("failed to register hotkey modifiers=%d key=%d: %v", mod, vk, err)
		runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "热键注册失败",
			Message: err.Error(),
		})
	}

	go a.superviseLoop("hotkey", func() {
		for range a.hotkeyManager.Events {
			a.toggleWindow()
		}
	})

	a.mu.Lock()
	a.windowVisible = !a.startHidden
	a.mu.Unlock()
	a.startTray()
	go func() {
		defer a.recoverGoroutine("autoBackup")
		a.maybeRunAutoBackup()
	}()
}

func (a *App) shutdown(ctx context.Context) {
	a.logInfof("shutdown")
	if a.trayController != nil {
		a.trayController.Stop()
	}
	if a.hotkeyManager != nil {
		a.hotkeyManager.Stop()
	}
	if a.watcher != nil {
		a.watcher.Stop()
	}
	if a.store != nil {
		a.store.Close()
	}
}

func (a *App) toggleWindow() {
	a.mu.Lock()
	if time.Since(a.lastToggle) < 400*time.Millisecond {
		a.mu.Unlock()
		return
	}
	a.lastToggle = time.Now()
	visible := a.windowVisible
	a.mu.Unlock()

	if visible {
		a.hideWindow()
		return
	}

	a.showWindow()
}

func (a *App) showWindow() {
	a.showWindowInternal(true)
}

func (a *App) showWindowFromTray() {
	a.showWindowInternal(false)
}

func (a *App) showWindowInternal(captureTarget bool) {
	if a.ctx == nil {
		return
	}
	target := uintptr(0)
	if captureTarget {
		target = windowutil.ForegroundWindow()
	}
	a.mu.Lock()
	if target != 0 {
		a.lastTarget = target
	}
	a.windowVisible = true
	a.mu.Unlock()

	x, y := getCursorPos()
	runtime.WindowSetPosition(a.ctx, int(x)+10, int(y)+10)
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowSetAlwaysOnTop(a.ctx, true)
	runtime.EventsEmit(a.ctx, "window:shown")
	a.updateTrayState()
}

func (a *App) showFromSecondInstance() {
	if a.ctx == nil {
		return
	}
	a.mu.Lock()
	a.windowVisible = true
	a.mu.Unlock()

	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowSetAlwaysOnTop(a.ctx, true)
	runtime.EventsEmit(a.ctx, "window:shown")
	runtime.EventsEmit(a.ctx, "clips:changed")
	a.updateTrayState()
}

func (a *App) hideWindow() {
	if a.ctx == nil {
		return
	}
	a.mu.Lock()
	a.windowVisible = false
	a.mu.Unlock()
	runtime.WindowSetAlwaysOnTop(a.ctx, false)
	runtime.WindowHide(a.ctx)
	a.updateTrayState()
}

func (a *App) toggleWindowFromTray() {
	a.mu.Lock()
	visible := a.windowVisible
	a.mu.Unlock()
	if visible {
		a.hideWindow()
		return
	}
	a.showWindowFromTray()
}

func (a *App) startTray() {
	controller := tray.New(appName)
	if err := controller.Start(); err != nil {
		a.logErrorf("failed to start tray: %v", err)
		return
	}
	a.trayController = controller
	a.updateTrayState()
	go a.superviseLoop("tray", func() { a.handleTrayActions(controller) })
	a.logInfof("started tray")
}

func (a *App) handleTrayActions(controller *tray.Controller) {
	for action := range controller.Actions {
		switch action {
		case tray.ActionShow:
			a.toggleWindowFromTray()
		case tray.ActionSettings:
			a.openSettingsFromTray()
		case tray.ActionTogglePause:
			if err := a.toggleCapturePausedFromTray(); err != nil {
				a.logErrorf("toggle capture paused from tray failed: %v", err)
			}
		case tray.ActionOpenDataDir:
			if err := a.OpenDataDir(); err != nil {
				a.logErrorf("open data dir from tray failed: %v", err)
			}
		case tray.ActionExit:
			a.QuitApp()
		}
	}
}

func (a *App) openSettingsFromTray() {
	if a.ctx == nil {
		return
	}
	a.showWindowFromTray()
	runtime.EventsEmit(a.ctx, "settings:open")
}

func (a *App) toggleCapturePausedFromTray() error {
	settings := a.loadAppSettings()
	settings.CapturePaused = !settings.CapturePaused
	if err := a.setCapturePaused(settings.CapturePaused); err != nil {
		return err
	}
	a.logInfof("tray set capturePaused=%v", settings.CapturePaused)
	return nil
}

func (a *App) setCapturePaused(paused bool) error {
	if a.store == nil {
		return nil
	}
	if err := a.store.SetSetting("capture_paused", strconv.FormatBool(paused)); err != nil {
		return err
	}
	a.invalidateSettingsCache()
	settings := a.loadAppSettings()
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "settings:updated", settings)
	}
	a.updateTrayState()
	return nil
}

func (a *App) invalidateSettingsCache() {
	a.settingsMu.Lock()
	a.settingsCache = nil
	a.settingsMu.Unlock()
}

func (a *App) updateTrayState() {
	if a.trayController == nil {
		return
	}
	settings := a.loadAppSettings()
	a.mu.Lock()
	visible := a.windowVisible
	a.mu.Unlock()
	a.trayController.SetState(tray.State{
		AppName: appName,
		Paused:  settings.CapturePaused,
		Visible: visible,
	})
}

func (a *App) pasteToLastTarget() {
	a.mu.Lock()
	target := a.lastTarget
	a.mu.Unlock()

	if target == 0 {
		return
	}

	time.Sleep(150 * time.Millisecond)
	windowutil.FocusWindow(target)
	time.Sleep(100 * time.Millisecond)
	windowutil.SendCtrlV()
}
