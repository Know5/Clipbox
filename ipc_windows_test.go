package main

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"clipbox/internal/clipboard"

	"github.com/Microsoft/go-winio"
)

func newTestResident(t *testing.T) (*resident, *ipcClient) {
	t.Helper()
	a := newTestAppWithStore(t)
	a.ctx = context.Background()
	path, descriptor, err := userPipe(a.dataDir)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := winio.ListenPipe(path, &winio.PipeConfig{SecurityDescriptor: descriptor})
	if err != nil {
		t.Fatal(err)
	}
	r := &resident{app: a, path: path, token: "test-session", listener: listener, events: make(chan uiEvent, 128), dialogs: make(map[string]chan ipcResponse), clients: make(map[net.Conn]struct{})}
	a.resident = r
	r.wg.Add(1)
	go r.accept()
	t.Cleanup(func() {
		a.stopOnce.Do(func() { close(a.stopCh) })
		listener.Close()
		r.mu.Lock()
		for conn := range r.clients {
			conn.Close()
		}
		r.mu.Unlock()
		r.wg.Wait()
	})
	return r, &ipcClient{path: path, token: r.token}
}

func TestIPCRestrictsSessionsAndInternalOperations(t *testing.T) {
	r, client := newTestResident(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := (&ipcClient{path: r.path, token: "wrong"}).request(ctx, "GetAppSettings"); err == nil {
		t.Fatal("invalid session accepted")
	}
	if _, err := client.request(ctx, "saveAppSettings", map[string]any{}); err == nil {
		t.Fatal("internal method exposed")
	}
	if _, err := client.request(ctx, "GetClipDetails", "wrong type"); err == nil {
		t.Fatal("invalid argument accepted")
	}
	if _, err := client.request(ctx, "PatchAppSettings", map[string]any{"capturePaused": true}); err != nil {
		t.Fatal(err)
	}
	if !r.app.GetAppSettings().CapturePaused {
		t.Fatal("patch did not reach the resident")
	}
}

func TestIPCStreamReconnectAndBackgroundCapture(t *testing.T) {
	r, client := newTestResident(t)
	connect := func() net.Conn {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		conn, err := winio.DialPipeContext(ctx, client.path)
		if err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(conn).Encode(ipcRequest{Token: client.token, Method: "__subscribe"})
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		var event uiEvent
		if err := json.NewDecoder(conn).Decode(&event); err != nil || event.Name != "ui:connected" {
			t.Fatalf("subscribe=%+v,%v", event, err)
		}
		return conn
	}
	first := connect()
	first.Close()
	second := connect()
	defer second.Close()
	_, err := r.app.persistCapturedClip(clipboard.ClipEntry{Type: "text", Content: "background source", Preview: "background", Timestamp: 1})
	if err != nil {
		t.Fatal(err)
	}
	var event uiEvent
	if err := json.NewDecoder(second).Decode(&event); err != nil || event.Name != "clips:updated" {
		t.Fatalf("capture event=%+v,%v", event, err)
	}
	second.Close()
	_, err = r.app.persistCapturedClip(clipboard.ClipEntry{Type: "text", Content: "after UI exit", Preview: "background", Timestamp: 2})
	if err != nil {
		t.Fatal(err)
	}
	page, err := r.app.GetClipPage(50, 0)
	if err != nil || page.Total != 2 {
		t.Fatalf("background lost capture: %+v,%v", page, err)
	}
}

func TestDevelopmentFacadeUsesTheSameTypedService(t *testing.T) {
	a := newTestAppWithStore(t)
	_, err := a.store.SaveOrUpdate(clipboard.ClipEntry{Type: "text", Content: "development", Preview: "dev", Timestamp: 1})
	if err != nil {
		t.Fatal(err)
	}
	bridge := &UIBridge{direct: a, local: a}
	page, err := bridge.GetClipPage(50, 0)
	if err != nil || page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("development facade=%+v,%v", page, err)
	}
}
