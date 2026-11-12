package tray

import (
	"testing"
	"time"
)

func TestTrayStopDestroysItsHiddenWindow(t *testing.T) {
	controller := New("ClipBox tray test")
	if err := controller.Start(); err != nil {
		t.Skipf("native tray is unavailable in this Windows session: %v", err)
	}
	defer controller.Stop()
	controller.windowMu.RLock()
	hwnd := controller.hwnd
	controller.windowMu.RUnlock()
	if hwnd == 0 {
		t.Fatal("tray did not create its hidden window")
	}
	controller.Stop()
	done := make(chan struct{})
	go func() {
		for range controller.Actions {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("tray worker did not finish shutdown")
	}
	if exists, _, _ := user32.NewProc("IsWindow").Call(hwnd); exists != 0 {
		t.Fatal("tray left its native window alive after shutdown")
	}
	if controller.icon.get() != 0 {
		t.Fatal("tray icon was not released after shutdown")
	}
}
