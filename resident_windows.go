package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/Microsoft/go-winio"
)

const uiIdleTimeout = 60 * time.Second

type resident struct {
	app           *App
	path          string
	token         string
	lastStartupMS int64
	listener      net.Listener
	mu            sync.Mutex
	child         *exec.Cmd
	childDone     chan struct{}
	launchedAt    time.Time
	connected     bool
	streamConn    net.Conn
	ready         bool
	releasing     bool
	pendingShow   bool
	settings      bool
	idle          *time.Timer
	events        chan uiEvent
	dialogs       map[string]chan ipcResponse
	hideWaiters   map[string]chan struct{}
	activity      map[string]bool
	viewState     string
	clients       map[net.Conn]struct{}
	wg            sync.WaitGroup
}

func runResident(dataDir string, hidden bool) error {
	path, descriptor, err := userPipe(dataDir)
	if err != nil {
		return err
	}
	listener, err := winio.ListenPipe(path, &winio.PipeConfig{SecurityDescriptor: descriptor, InputBufferSize: 64 * 1024, OutputBufferSize: 64 * 1024})
	if err != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		method := "__ping"
		if !hidden {
			method = "__show"
		}
		_, err = (&ipcClient{path: path}).request(ctx, method)
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := NewApp(dataDir, true)
	r := &resident{app: a, path: path, token: rand.Text(), listener: listener, events: make(chan uiEvent, 128), dialogs: make(map[string]chan ipcResponse), activity: make(map[string]bool), clients: make(map[net.Conn]struct{})}
	if hasArg("--isolated") && len(os.Getenv("CLIPBOX_ISOLATED_TOKEN")) > 20 {
		r.token = os.Getenv("CLIPBOX_ISOLATED_TOKEN")
	}
	a.resident = r
	a.startup(ctx)
	if a.store == nil {
		listener.Close()
		return fmt.Errorf("无法打开 ClipBox 数据库，请检查数据目录")
	}
	r.wg.Add(1)
	go r.accept()
	if !hidden {
		a.showWindowFromTray()
	}
	<-a.stopCh
	cancel()
	r.mu.Lock()
	if r.idle != nil {
		r.idle.Stop()
	}
	r.pendingShow = false
	r.emitLocked(uiEvent{Name: "ui:quit"})
	child := r.child
	r.mu.Unlock()
	listener.Close()
	// A normal UI quit releases its browser environment. If the UI crashed or
	// stopped responding, the resident still exits and the next launch recovers.
	if child != nil && child.Process != nil {
		for i := 0; i < 20; i++ {
			r.mu.Lock()
			running := r.child != nil
			r.mu.Unlock()
			if !running {
				break
			}
			time.Sleep(25 * time.Millisecond)
		}
		_ = child.Process.Kill()
	}
	r.mu.Lock()
	for conn := range r.clients {
		conn.Close()
	}
	r.mu.Unlock()
	workersDone := make(chan struct{})
	go func() { r.wg.Wait(); close(workersDone) }()
	select {
	case <-workersDone:
	case <-time.After(10 * time.Second):
		a.logErrorf("shutdown timed out waiting for a background operation")
	}
	a.shutdown(ctx)
	return nil
}

func (r *resident) accept() {
	defer r.wg.Done()
	for {
		conn, err := r.listener.Accept()
		if err != nil {
			return
		}
		r.mu.Lock()
		if len(r.clients) >= 16 {
			r.mu.Unlock()
			conn.Close()
			continue
		}
		r.clients[conn] = struct{}{}
		r.mu.Unlock()
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			defer conn.Close()
			defer func() { r.mu.Lock(); delete(r.clients, conn); r.mu.Unlock() }()
			r.serve(conn)
		}()
	}
}

func (r *resident) serve(conn net.Conn) {
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var request ipcRequest
	if json.NewDecoder(io.LimitReader(conn, maxIPCBytes)).Decode(&request) != nil {
		return
	}
	_ = conn.SetDeadline(time.Now().Add(3 * time.Minute))
	respond := func(value any, err error) {
		response := ipcResponse{}
		if err != nil {
			response.Error = err.Error()
		} else {
			response.Value, _ = json.Marshal(value)
		}
		_ = json.NewEncoder(conn).Encode(response)
	}
	if request.Method == "__show" || request.Method == "__ping" {
		if request.Method == "__show" {
			r.app.showWindowFromTray()
		}
		respond(true, nil)
		return
	}
	if !sameToken(request.Token, r.token) {
		respond(nil, fmt.Errorf("界面会话已失效"))
		return
	}
	switch request.Method {
	case "__bootstrap":
		respond(map[string]string{"dataDir": r.app.dataDir, "imageSession": r.app.imageSession}, nil)
	case "__ready":
		r.mu.Lock()
		if r.ready {
			r.mu.Unlock()
			respond(true, nil)
			return
		}
		r.ready = true
		r.lastStartupMS = time.Since(r.launchedAt).Milliseconds()
		r.app.logInfof("UI ready in %dms", r.lastStartupMS)
		settings := r.settings
		r.settings = false
		r.app.mu.Lock()
		visible := r.app.windowVisible
		r.app.mu.Unlock()
		if visible {
			r.emitLocked(uiEvent{Name: "ui:show"})
		}
		if settings {
			r.emitLocked(uiEvent{Name: "settings:open"})
		}
		r.mu.Unlock()
		respond(true, nil)
	case "__subscribe":
		r.stream(conn)
	case "__dialogResult":
		var result struct {
			ID     string      `json:"id"`
			Result ipcResponse `json:"result"`
		}
		if len(request.Args) != 1 || json.Unmarshal(request.Args[0], &result) != nil {
			respond(nil, fmt.Errorf("无效的对话框结果"))
			return
		}
		r.mu.Lock()
		channel := r.dialogs[result.ID]
		r.mu.Unlock()
		if channel != nil {
			select {
			case channel <- result.Result:
			default:
			}
		}
		respond(true, nil)
	case "__hidden":
		var id string
		if len(request.Args) == 1 {
			_ = json.Unmarshal(request.Args[0], &id)
		}
		r.mu.Lock()
		waiter := r.hideWaiters[id]
		r.mu.Unlock()
		if waiter != nil {
			select {
			case waiter <- struct{}{}:
			default:
			}
		}
		respond(true, nil)
	case "__release":
		if !hasArg("--isolated") {
			respond(nil, fmt.Errorf("仅限独立测试实例"))
			return
		}
		r.releaseUI()
		respond(true, nil)
	default:
		_ = json.NewEncoder(conn).Encode(dispatchBackend(r.app, request))
	}
}

func (r *resident) stream(conn net.Conn) {
	r.mu.Lock()
	if r.streamConn != nil {
		r.streamConn.Close()
	}
	r.streamConn = conn
	outgoing := make(chan uiEvent, 128)
	r.events = outgoing
	r.connected = true
	r.emitLocked(uiEvent{Name: "ui:connected"})
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		if r.streamConn == conn {
			r.connected = false
			r.ready = false
			r.streamConn = nil
			r.activity = make(map[string]bool)
			r.app.textToolsMu.Lock()
			for _, cancel := range r.app.textTasks {
				cancel()
			}
			r.app.textToolsMu.Unlock()
		}
		r.mu.Unlock()
	}()
	_ = conn.SetDeadline(time.Time{})
	encoder := json.NewEncoder(conn)
	disconnected := make(chan struct{})
	go func() { var probe [1]byte; _, _ = conn.Read(probe[:]); close(disconnected) }()
	for {
		select {
		case event := <-outgoing:
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if encoder.Encode(event) != nil {
				return
			}
		case <-r.app.stopCh:
			_ = encoder.Encode(uiEvent{Name: "ui:quit"})
			return
		case <-disconnected:
			return
		}
	}
}

func (r *resident) publish(name string, data ...any) {
	event := uiEvent{Name: name}
	if len(data) > 0 {
		event.Data, _ = json.Marshal(data[0])
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.emitLocked(event)
}

func (r *resident) emitLocked(event uiEvent) {
	if !r.connected {
		return
	}
	select {
	case r.events <- event:
	default:
		// A slow webview can re-fetch a compact page instead of accumulating an
		// unbounded event backlog in the resident.
		for len(r.events) > 0 {
			<-r.events
		}
		r.events <- uiEvent{Name: "clips:changed"}
		r.events <- event
	}
}

func (r *resident) show(settings bool) {
	select {
	case <-r.app.stopCh:
		return
	default:
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.idle != nil {
		r.idle.Stop()
	}
	r.settings = r.settings || settings
	if r.child != nil {
		if r.releasing {
			r.pendingShow = true
			return
		}
		if r.ready {
			r.emitLocked(uiEvent{Name: "ui:show"})
			if r.settings {
				r.emitLocked(uiEvent{Name: "settings:open"})
				r.settings = false
			}
		}
		return
	}
	for len(r.events) > 0 {
		<-r.events
	}
	executable, err := os.Executable()
	if err != nil {
		r.app.logErrorf("resolve UI executable failed")
		return
	}
	cmd := exec.Command(executable, "--ui")
	if hasArg("--isolated") {
		cmd.Args = append(cmd.Args, "--isolated")
	}
	cmd.Env = append(os.Environ(), "CLIPBOX_UI_PIPE="+r.path, "CLIPBOX_UI_TOKEN="+r.token)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if err := cmd.Start(); err != nil {
		r.app.logErrorf("start UI failed: %v", err)
		return
	}
	job, err := ownUIProcess(cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		r.app.logErrorf("UI process group setup failed: %v", err)
		return
	}
	r.child = cmd
	r.launchedAt = time.Now()
	childDone := make(chan struct{})
	r.childDone = childDone
	r.app.logInfof("UI process started pid=%d", cmd.Process.Pid)
	go func() {
		defer close(childDone)
		_ = cmd.Wait()
		r.mu.Lock()
		r.releasing = true
		r.ready = false
		r.mu.Unlock()
		job.stop()
		r.mu.Lock()
		if r.child == cmd {
			r.child = nil
			r.ready = false
			r.releasing = false
		}
		reopen := r.pendingShow
		r.pendingShow = false
		if r.streamConn != nil {
			r.streamConn.Close()
		}
		if !reopen {
			r.app.mu.Lock()
			r.app.windowVisible = false
			r.app.mu.Unlock()
		}
		r.mu.Unlock()
		r.app.updateTrayState()
		r.app.logInfof("UI process released")
		if reopen {
			r.app.showWindowFromTray()
		}
	}()
}

func (r *resident) hide() {
	idleSeconds := r.app.loadAppSettings().UIIdleSeconds
	r.mu.Lock()
	if r.idle != nil {
		r.idle.Stop()
	}
	r.idle = time.AfterFunc(time.Duration(idleSeconds)*time.Second, r.releaseUI)
	if !r.connected {
		r.mu.Unlock()
		return
	}
	if r.hideWaiters == nil {
		r.hideWaiters = make(map[string]chan struct{})
	}
	id := rand.Text()
	waiter := make(chan struct{}, 1)
	r.hideWaiters[id] = waiter
	data, _ := json.Marshal(map[string]string{"id": id})
	r.emitLocked(uiEvent{Name: "ui:hide", Data: data})
	r.mu.Unlock()
	// Paste must not race an asynchronously hidden, still-foreground webview.
	select {
	case <-waiter:
	case <-time.After(750 * time.Millisecond):
	case <-r.app.stopCh:
	}
	r.mu.Lock()
	delete(r.hideWaiters, id)
	r.mu.Unlock()
}

func (r *resident) releaseUI() {
	r.app.textToolsMu.Lock()
	busy := len(r.app.textTasks) > 0
	r.app.textToolsMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.app.mu.Lock()
	visible := r.app.windowVisible
	r.app.mu.Unlock()
	if visible {
		return
	}
	if busy || len(r.dialogs) > 0 || len(r.activity) > 0 {
		r.idle = time.AfterFunc(5*time.Second, r.releaseUI)
		return
	}
	if r.child == nil {
		return
	}
	r.releasing = true
	r.ready = false
	r.emitLocked(uiEvent{Name: "ui:quit"})
}

func (r *resident) dialog(kind string, options any) (string, error) {
	result := make(chan ipcResponse, 1)
	id := rand.Text()
	r.mu.Lock()
	r.dialogs[id] = result
	r.mu.Unlock()
	defer func() { r.mu.Lock(); delete(r.dialogs, id); r.mu.Unlock() }()
	r.publish("ui:dialog", map[string]any{"id": id, "kind": kind, "options": options})
	select {
	case response := <-result:
		if response.Error != "" {
			return "", fmt.Errorf("%s", response.Error)
		}
		var path string
		if err := json.Unmarshal(response.Value, &path); err != nil {
			return "", err
		}
		return path, nil
	case <-r.app.stopCh:
		return "", fmt.Errorf("程序已退出")
	case <-time.After(3 * time.Minute):
		return "", fmt.Errorf("对话框操作超时，请重试")
	}
}
