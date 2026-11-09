package main

import (
	"context"
	"fmt"
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
	lastSkipNotice time.Time // 最近一次"未记录"提示的时间（限频，避免连续复制文件刷屏）
	mu             sync.Mutex
	settingsMu     sync.Mutex
	settingsCache  *AppSettings // 内存缓存：避免每次剪贴板事件/托盘刷新都跑 16 条 SQL
	foregroundInfo func() windowutil.ForegroundInfo
	windowInfo     func(hwnd uintptr) windowutil.ForegroundInfo
	stopCh         chan struct{} // closed on shutdown to stop background loops (auto-backup ticker)
	stopOnce       sync.Once
}

func NewApp(dataDir string, startHidden bool) *App {
	return &App{
		dataDir:        dataDir,
		logPath:        storageLogPath(dataDir),
		startHidden:    startHidden,
		foregroundInfo: windowutil.CurrentForegroundInfo,
		windowInfo:     windowutil.WindowInfo,
		stopCh:         make(chan struct{}),
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
	a.watcher.SetSkipCallback(func(reason clipboard.SkipReason) {
		switch reason {
		case clipboard.SkipReasonOversizedText:
			a.logErrorf("skipped oversized clipboard text")
			runtime.EventsEmit(ctx, "clip:skipped", map[string]string{
				"reason":  string(reason),
				"message": "文本过大，已跳过记录（超过 10MB）",
			})
		case clipboard.SkipReasonOversizedImage:
			a.logErrorf("skipped oversized clipboard image")
			runtime.EventsEmit(ctx, "clip:skipped", map[string]string{
				"reason":  string(reason),
				"message": "图片过大，已跳过记录（超过 50MB）",
			})
		case clipboard.SkipReasonBusyClipboard:
			// 剪贴板被其它程序占着：用户会看到"复制了但列表没变"。30 秒内最多提示一次。
			a.logErrorf("skipped clipboard update: clipboard busy")
			if a.allowSkipNotice(30 * time.Second) {
				runtime.EventsEmit(ctx, "clip:skipped", map[string]string{
					"reason":  string(reason),
					"message": "剪贴板正被其它程序占用，这一次没能记录",
				})
			}
		case clipboard.SkipReasonUnsupportedType:
			// 复制文件、富文本等既非纯文本也非位图的内容时不会入库。
			// 以前只写日志，使用者会以为"我复制了但列表里没有"；现在 10 秒内最多提示一次。
			a.logErrorf("skipped clipboard update: %s", string(reason))
			if a.allowSkipNotice(10 * time.Second) {
				runtime.EventsEmit(ctx, "clip:skipped", map[string]string{
					"reason":  string(reason),
					"message": "这条内容不是文本或图片（如文件、网页富文本），未记录",
				})
			}
		case clipboard.SkipReasonReadFailed:
			a.logErrorf("skipped clipboard update: %s", string(reason))
			if a.allowSkipNotice(30 * time.Second) {
				runtime.EventsEmit(ctx, "clip:skipped", map[string]string{
					"reason":  string(reason),
					"message": "读取剪贴板失败，这一次没能记录",
				})
			}
		}
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
	defaultMod, defaultVK := uint32(hotkey.ModControl|hotkey.ModAlt), uint32(0x56)
	mod, vk := defaultMod, defaultVK
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
	// 0.2.1 及更早允许把热键设成 Ctrl+1 这类列表快捷键，全局注册后列表里的同名快捷键失效。
	// 存量设置改回默认并落盘；提示框异步弹出，不阻塞下面的热键注册（2026-11-06）。
	if reason := reservedHotkeyReason(int(mod), int(vk)); reason != "" {
		previous := hotkeyDisplayName(int(mod), int(vk))
		a.logErrorf("stored hotkey modifiers=%d key=%d is reserved, reset to default: %s", mod, vk, reason)
		mod, vk = defaultMod, defaultVK
		_ = a.store.SetSetting("hotkey_modifiers", strconv.Itoa(int(mod)))
		_ = a.store.SetSetting("hotkey_key", strconv.Itoa(int(vk)))
		notice := fmt.Sprintf("原热键 %s 不可用：%s。\n已改回 %s，可在设置里换成其他组合。", previous, reason, hotkeyDisplayName(int(mod), int(vk)))
		go func() {
			defer a.recoverGoroutine("hotkeyResetNotice")
			runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
				Type:    runtime.InfoDialog,
				Title:   "唤起热键已改回默认",
				Message: notice,
			})
		}()
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

	// 开机自启自愈：Run 值缺失、或指向的 exe 已不存在时改指当前 exe；指向另一个
	// 仍存在的副本时不动，避免临时副本抢走自启。意图为关时清理 Run 值。
	startAtLoginWanted := a.loadStartAtLoginIntent()
	if err := reconcileStartAtLogin(startAtLoginWanted); err != nil {
		a.logErrorf("failed to reconcile start-at-login on startup: %v", err)
	}
	if startAtLoginWanted && startAtLoginDisabledByOS() {
		// Run 值没问题，但 Windows 启动管理器把它关了；这时每次登录都会静默跳过。
		// 不去偷偷改回来（可能是用户自己关的），只在日志里留痕，设置页会给出提示。
		a.logErrorf("start-at-login entry is disabled by Windows startup manager; enable it in Task Manager > Startup or press 启用启动项 in settings")
	}

	a.mu.Lock()
	a.windowVisible = !a.startHidden
	a.mu.Unlock()
	a.startTray()
	go func() {
		defer a.recoverGoroutine("autoBackup")
		a.runAutoBackupScheduler()
	}()
}

func (a *App) shutdown(ctx context.Context) {
	a.logInfof("shutdown")
	a.stopOnce.Do(func() { close(a.stopCh) })
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

	// 跟随鼠标出现，但整块窗口必须留在鼠标所在显示器的工作区内：
	// 右下放不下就往左上收边，不越出屏幕（2026-10-29 使用者反馈）。
	cx, cy := getCursorPos()
	hwnd := windowutil.FindOwnWindow(mainWindowClass, appName)
	if w, h, ok := windowutil.WindowSize(hwnd); ok {
		if wa, ok := windowutil.MonitorWorkArea(cx, cy); ok {
			x, y := windowutil.PlaceNearCursor(cx, cy, w, h, wa, 10)
			windowutil.MoveWindow(hwnd, x, y)
		}
	} else {
		a.logErrorf("show: main window not found, keeping last position")
	}
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

// allowSkipNotice 限制"未记录"提示的发送频率：minInterval 内只允许一次。
func (a *App) allowSkipNotice(minInterval time.Duration) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	if now.Sub(a.lastSkipNotice) < minInterval {
		return false
	}
	a.lastSkipNotice = now
	return true
}

// notifyPasteFailure 在粘贴失败时给一行界面提示（内容已经在剪贴板里，可手动粘贴）。
// 10 秒限频，避免连点十次刷满提示。
func (a *App) notifyPasteFailure() {
	if a.ctx == nil {
		return
	}
	if !a.allowSkipNotice(10 * time.Second) {
		return
	}
	runtime.EventsEmit(a.ctx, "clip:skipped", map[string]string{
		"reason":  "paste-failed",
		"message": "没能粘贴到原窗口（可能是管理员窗口），内容已在剪贴板，按 Ctrl+V 即可",
	})
}

func (a *App) pasteToLastTarget() {
	a.mu.Lock()
	target := a.lastTarget
	a.mu.Unlock()
	a.logInfof("paste start target=%d", target)

	// 内容已在剪贴板中,这里失败不影响手动粘贴,只记录日志(与 Win+V 一致,不弹提示)。
	if target == 0 {
		a.logErrorf("paste skipped: no target window")
		return
	}

	focused := windowutil.FocusWindow(target)
	a.logInfof("paste focus target=%d ok=%v", target, focused)
	if !focused {
		a.logErrorf("paste aborted: target window did not gain focus hwnd=%d", target)
		a.notifyPasteFailure()
		return
	}
	if err := windowutil.SendCtrlV(); err != nil {
		a.logErrorf("paste failed: %v", err)
		a.notifyPasteFailure()
		return
	}
	a.logInfof("paste sent ctrl+v")
}
