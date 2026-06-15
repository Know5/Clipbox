package main

import (
	"syscall"
	"unsafe"
)

var user32Procs = syscall.NewLazyDLL("user32.dll")

type point struct {
	x int32
	y int32
}

func getCursorPos() (int32, int32) {
	var p point
	user32Procs.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&p)))
	return p.x, p.y
}
