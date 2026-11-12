package tray

import "sync"

type iconResource struct {
	mu      sync.Mutex
	handle  uintptr
	owned   bool
	loaded  bool
	closed  bool
	load    func() (uintptr, bool)
	destroy func(uintptr)
}

func (icon *iconResource) get() uintptr {
	icon.mu.Lock()
	defer icon.mu.Unlock()
	if icon.closed {
		return 0
	}
	if !icon.loaded {
		icon.handle, icon.owned = icon.load()
		icon.loaded = true
	}
	return icon.handle
}

func (icon *iconResource) close() {
	icon.mu.Lock()
	defer icon.mu.Unlock()
	if icon.closed {
		return
	}
	icon.closed = true
	if icon.owned && icon.handle != 0 {
		icon.destroy(icon.handle)
	}
	icon.handle = 0
}
