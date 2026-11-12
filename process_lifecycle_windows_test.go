package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"

	"clipbox/internal/clipboard"
	"clipbox/internal/storage"
)

// Opt-in production test: real resident/UI processes, independent data and
// hotkey, no clipboard injection, no writes to the real startup registration.
func TestProductionResidentLifecycle(t *testing.T) {
	executable := os.Getenv("CLIPBOX_APP_EXE")
	if executable == "" {
		t.Skip("set CLIPBOX_APP_EXE to test the built application")
	}
	dir := t.TempDir()
	t.Logf("isolated data: %s", dir)
	store, err := storage.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.SaveOrUpdate(clipboard.ClipEntry{Type: "text", Content: "resident lifecycle sample", Preview: "sample", Timestamp: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetSettings(map[string]string{"start_at_login": "false", "capture_paused": "true", "hotkey_modifiers": "7", "hotkey_key": "135"}); err != nil {
		t.Fatal(err)
	}
	store.Close()
	before := startupRegistryValue()
	token := rand.Text()
	cmd := exec.Command(executable, "--isolated", "--hidden", "--data-dir", dir)
	cmd.Env = append(os.Environ(), "CLIPBOX_ISOLATED_TOKEN="+token, "CLIPBOX_TEST_NOFOCUS=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	path, _, _ := userPipe(dir)
	client := &ipcClient{path: path, token: token}
	call := func(method string, args ...any) (json.RawMessage, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		return client.request(ctx, method, args...)
	}
	t.Cleanup(func() {
		select {
		case <-done:
		default:
			_, _ = call("QuitApp")
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
		if startupRegistryValue() != before {
			t.Error("isolated lifecycle changed the real startup entry")
		}
	})
	bootDeadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := call("__ping"); err == nil {
			break
		}
		if time.Now().After(bootDeadline) {
			t.Fatal("resident did not become ready")
		}
		time.Sleep(25 * time.Millisecond)
	}
	info := func() RuntimeInfo {
		data, err := call("GetRuntimeInfo")
		if err != nil {
			if log, readErr := os.ReadFile(dir + "/clipbox.log"); readErr == nil {
				t.Log(string(log))
			}
			t.Fatal(err)
		}
		var result RuntimeInfo
		_ = json.Unmarshal(data, &result)
		return result
	}
	if initial := info(); initial.UIRunning {
		t.Fatal("hidden startup created a webview")
	}
	wait := func(wantRunning, wantReady bool) {
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			state := info()
			if state.UIRunning == wantRunning && (!wantReady || state.UIReady) {
				return
			}
			time.Sleep(30 * time.Millisecond)
		}
		t.Fatalf("unexpected UI lifecycle state: %+v", info())
	}
	waitPopup := func(wantVisible bool) {
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			state := info()
			if state.Visible == wantVisible && !state.UIRunning {
				return
			}
			time.Sleep(30 * time.Millisecond)
		}
		t.Fatalf("unexpected popup state: %+v", info())
	}
	cycles := 3
	if requested, err := strconv.Atoi(os.Getenv("CLIPBOX_LIFECYCLE_CYCLES")); err == nil && requested > 0 && requested <= 100 {
		cycles = requested
	}
	for cycle := 0; cycle < cycles; cycle++ {
		// Native popup: show/hide without any child process.
		if _, err := call("__show"); err != nil {
			t.Fatal(err)
		}
		waitPopup(true)
		if _, err := call("HideWindow"); err != nil {
			t.Fatal(err)
		}
		waitPopup(false)
		// Settings handoff: the webview child owns the remaining lifecycle.
		if _, err := call("__showSettings"); err != nil {
			t.Fatal(err)
		}
		wait(true, true)
		t.Logf("cold UI ready: %dms", info().LastStartupMS)
		if _, err := call("SetUIActivity", "test-draft", true); err != nil {
			t.Fatal(err)
		}
		if _, err := call("HideWindow"); err != nil {
			t.Fatal(err)
		}
		_, _ = call("__release")
		if !info().UIRunning {
			t.Fatal("unsaved draft was reclaimed")
		}
		_, _ = call("SetUIActivity", "test-draft", false)
		_, _ = call("__release")
		wait(false, false)
		if _, err := call("UpdateClipMetadata", id, "written while UI is absent", "Work"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := call("__show"); err != nil {
		t.Fatal(err)
	}
	waitPopup(true)
	if _, err := call("QuitApp"); err != nil {
		t.Logf("shutdown closed the IPC connection: %v", err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("resident failed to shut down")
	}
}
