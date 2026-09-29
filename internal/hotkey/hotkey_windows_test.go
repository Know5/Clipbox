package hotkey

import (
	"errors"
	"syscall"
	"testing"
)

func TestIsHotkeyOccupiedError(t *testing.T) {
	occupied := &RegisterHotkeyError{Err: syscall.Errno(1409), Occupied: true}
	if !IsHotkeyOccupiedError(occupied) {
		t.Fatal("IsHotkeyOccupiedError(occupied) = false, want true")
	}
	other := &RegisterHotkeyError{Err: syscall.Errno(1410), Occupied: false}
	if IsHotkeyOccupiedError(other) {
		t.Fatal("IsHotkeyOccupiedError(other) = true, want false")
	}
	if IsHotkeyOccupiedError(syscall.Errno(1409)) {
		// Raw errno also indicates ERROR_HOTKEY_ALREADY_REGISTERED.
	} else {
		t.Fatal("IsHotkeyOccupiedError(errno 1409) = false, want true")
	}
	if IsHotkeyOccupiedError(errors.New("boom")) {
		t.Fatal("IsHotkeyOccupiedError(generic) = true, want false")
	}
	if msg := occupied.Error(); msg == "" {
		t.Fatal("RegisterHotkeyError.Error() is empty")
	}
}
