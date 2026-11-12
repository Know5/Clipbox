package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"clipbox/internal/storage"

	"github.com/Microsoft/go-winio"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// UIBridge is a thin, typed facade. The resident exclusively owns database
// writes, capture, hotkeys and credentials; this process owns only the webview.
type UIBridge struct {
	client    *ipcClient
	local     *App
	direct    *App // development builds keep Wails hot reload in one process
	ctx       context.Context
	connected chan struct{}
	readyOnce sync.Once
	quitting  atomic.Bool
}

func newUIBridge() (*UIBridge, error) {
	client := &ipcClient{path: os.Getenv("CLIPBOX_UI_PIPE"), token: os.Getenv("CLIPBOX_UI_TOKEN")}
	if client.path == "" || client.token == "" {
		return nil, fmt.Errorf("请从 ClipBox 主程序打开界面")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	data, err := client.request(ctx, "__bootstrap")
	if err != nil {
		return nil, err
	}
	var bootstrap struct {
		DataDir      string `json:"dataDir"`
		ImageSession string `json:"imageSession"`
	}
	if err := json.Unmarshal(data, &bootstrap); err != nil {
		return nil, err
	}
	local := NewApp(bootstrap.DataDir, true)
	local.imageSession = bootstrap.ImageSession
	local.store, err = storage.OpenReader(bootstrap.DataDir)
	if err != nil {
		return nil, err
	}
	return &UIBridge{client: client, local: local, connected: make(chan struct{})}, nil
}

func (b *UIBridge) startup(ctx context.Context) {
	b.ctx, b.local.ctx = ctx, ctx
	if b.direct != nil {
		b.direct.startup(ctx)
		return
	}
	go b.events()
}

func (b *UIBridge) shutdown(context.Context) {
	if b.direct != nil {
		b.direct.shutdown(b.ctx)
		return
	}
	if b.local != nil && b.local.store != nil {
		_ = b.local.store.Close()
	}
}

func (b *UIBridge) FrontendReady() error {
	if b.direct != nil {
		return nil
	}
	select {
	case <-b.connected:
	case <-time.After(5 * time.Second):
		return fmt.Errorf("后台连接尚未就绪，请重试")
	}
	_, err := callUIBackend[bool](b, "__ready")
	return err
}

func (b *UIBridge) events() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := winio.DialPipeContext(ctx, b.client.path)
	if err != nil {
		b.quit()
		return
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(ipcRequest{Token: b.client.token, Method: "__subscribe"}); err != nil {
		b.quit()
		return
	}
	decoder := json.NewDecoder(conn)
	for {
		var event uiEvent
		if err := decoder.Decode(&event); err != nil {
			b.quit()
			return
		}
		switch event.Name {
		case "ui:connected":
			b.readyOnce.Do(func() { close(b.connected) })
		case "ui:show":
			if hasArg("--isolated") && os.Getenv("CLIPBOX_TEST_NOFOCUS") == "1" {
				runtime.EventsEmit(b.ctx, "window:shown")
			} else {
				b.local.showWindowFromTray()
			}
		case "ui:hide":
			b.local.hideWindow()
			var data struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(event.Data, &data)
			if data.ID != "" {
				_, _ = callUIBackend[bool](b, "__hidden", data.ID)
			}
		case "ui:quit":
			b.quit()
			return
		case "ui:dialog":
			go b.dialog(event.Data)
		default:
			var data any
			_ = json.Unmarshal(event.Data, &data)
			runtime.EventsEmit(b.ctx, event.Name, data)
		}
	}
}

func (b *UIBridge) quit() {
	if !b.quitting.CompareAndSwap(false, true) {
		return
	}
	if b.ctx != nil {
		runtime.Quit(b.ctx)
	}
}

func (b *UIBridge) dialog(data json.RawMessage) {
	var request struct {
		ID      string          `json:"id"`
		Kind    string          `json:"kind"`
		Options json.RawMessage `json:"options"`
	}
	if json.Unmarshal(data, &request) != nil {
		return
	}
	var path string
	var err error
	if request.Kind == "save" {
		var options runtime.SaveDialogOptions
		_ = json.Unmarshal(request.Options, &options)
		path, err = runtime.SaveFileDialog(b.ctx, options)
	} else {
		var options runtime.OpenDialogOptions
		_ = json.Unmarshal(request.Options, &options)
		if request.Kind == "directory" {
			path, err = runtime.OpenDirectoryDialog(b.ctx, options)
		} else {
			path, err = runtime.OpenFileDialog(b.ctx, options)
		}
	}
	result := ipcResponse{}
	if err != nil {
		result.Error = err.Error()
	} else {
		result.Value, _ = json.Marshal(path)
	}
	_, _ = callUIBackend[bool](b, "__dialogResult", map[string]any{"id": request.ID, "result": result})
}

func (b *UIBridge) beforeClose(context.Context) bool {
	if b.quitting.Load() {
		return false
	}
	b.HideWindow()
	return true
}

func bridgeValue[T any](b *UIBridge, name string, args ...any) T {
	value, err := callUIBackend[T](b, name, args...)
	if err != nil && b.ctx != nil {
		runtime.EventsEmit(b.ctx, "clip:skipped", map[string]string{"message": err.Error()})
	}
	return value
}

// These dialog helpers keep application services independent of a live UI.
func (a *App) saveFileDialog(options runtime.SaveDialogOptions) (string, error) {
	if a.resident != nil {
		return a.resident.dialog("save", options)
	}
	return runtime.SaveFileDialog(a.ctx, options)
}
func (a *App) openFileDialog(options runtime.OpenDialogOptions) (string, error) {
	if a.resident != nil {
		return a.resident.dialog("open", options)
	}
	return runtime.OpenFileDialog(a.ctx, options)
}
func (a *App) openDirectoryDialog(options runtime.OpenDialogOptions) (string, error) {
	if a.resident != nil {
		return a.resident.dialog("directory", options)
	}
	return runtime.OpenDirectoryDialog(a.ctx, options)
}
