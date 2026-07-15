package main

import "runtime/debug"

// recoverGoroutine logs a panic from a one-shot background goroutine so a
// single failure never takes down the whole process. Use with defer at the
// top of the goroutine.
func (a *App) recoverGoroutine(name string) {
	if r := recover(); r != nil {
		a.logErrorf("goroutine %s panic: %v\n%s", name, r, debug.Stack())
	}
}

// superviseLoop runs fn and restarts it if it panics, so a long-lived worker
// (hotkey / tray dispatch) keeps running after an unexpected panic. fn is
// expected to return normally when it should stop for good — for example when
// the channel it ranges over is closed during shutdown.
func (a *App) superviseLoop(name string, fn func()) {
	for {
		if !a.runLoopOnce(name, fn) {
			return
		}
	}
}

// runLoopOnce runs fn once, returning true if it panicked (and should be
// restarted) or false if it returned normally (and should stop).
func (a *App) runLoopOnce(name string, fn func()) (panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			a.logErrorf("goroutine %s panic (restarting): %v\n%s", name, r, debug.Stack())
			panicked = true
		}
	}()
	fn()
	return false
}
