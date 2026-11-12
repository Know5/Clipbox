package tray

import (
	"sync"
	"testing"
)

func TestTrayTipUpdatesDoNotAllocateIcons(t *testing.T) {
	controller := New("test")
	controller.icon = &iconResource{load: func() (uintptr, bool) {
		t.Fatal("tip updates and removal must not extract an icon")
		return 0, false
	}}
	for i := 0; i < 100; i++ {
		if controller.iconData(nifTip).icon != 0 || controller.iconData(0).icon != 0 {
			t.Fatal("non-icon notification unexpectedly contains an icon")
		}
	}
	controller.Stop()
}

func TestTrayOwnedIconIsLoadedAndReleasedOnce(t *testing.T) {
	controller := New("test")
	loads, destroys := 0, 0
	controller.icon = &iconResource{
		load: func() (uintptr, bool) { loads++; return 42, true },
		destroy: func(handle uintptr) {
			if handle != 42 {
				t.Fatalf("released unexpected icon %d", handle)
			}
			destroys++
		},
	}
	var workers sync.WaitGroup
	for i := 0; i < 100; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if controller.iconData(nifIcon).icon != 42 {
				t.Error("cached icon missing")
			}
		}()
	}
	workers.Wait()
	controller.Stop()
	controller.Stop()
	if loads != 1 || destroys != 1 {
		t.Fatalf("loads=%d destroys=%d, want one of each", loads, destroys)
	}
	if controller.iconData(nifIcon).icon != 0 || loads != 1 {
		t.Fatal("stopped controller reloaded an icon")
	}
}

func TestTraySharedSystemIconIsNotDestroyed(t *testing.T) {
	controller := New("test")
	controller.icon = &iconResource{
		load:    func() (uintptr, bool) { return 7, false },
		destroy: func(uintptr) { t.Fatal("shared LoadIcon handle must not be destroyed") },
	}
	if controller.iconData(nifIcon).icon != 7 {
		t.Fatal("shared icon missing")
	}
	controller.Stop()
}
