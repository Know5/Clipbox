package main

import (
	"os"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestExtractExePath(t *testing.T) {
	cases := []struct {
		name    string
		command string
		want    string
	}{
		{"quoted with args", `"C:\Users\user\clipbox.exe" --hidden`, `C:\Users\user\clipbox.exe`},
		{"quoted no args", `"C:\Program Files\ClipBox\clipbox.exe"`, `C:\Program Files\ClipBox\clipbox.exe`},
		{"quoted path with spaces and args", `"C:\Program Files\Clip Box\clipbox.exe" --hidden`, `C:\Program Files\Clip Box\clipbox.exe`},
		{"bare path with args", `C:\tools\clipbox.exe --hidden`, `C:\tools\clipbox.exe`},
		{"bare path no args", `C:\tools\clipbox.exe`, `C:\tools\clipbox.exe`},
		{"empty", ``, ``},
		{"whitespace", `   `, ``},
		{"unterminated quote", `"C:\tools\clipbox.exe`, `C:\tools\clipbox.exe`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractExePath(tc.command); got != tc.want {
				t.Fatalf("extractExePath(%q) = %q, want %q", tc.command, got, tc.want)
			}
		})
	}
}

// TestStartupCommandNoGoEscaping guards against the regression where %q
// Go-escaped backslashes into the registry value (C:\\Users\\...), which broke
// both the launch and the readback comparison.
func TestStartupCommandNoGoEscaping(t *testing.T) {
	cmd, err := startupCommand()
	if err != nil {
		t.Fatalf("startupCommand() error: %v", err)
	}
	if got := extractExePath(cmd); got == "" {
		t.Fatalf("startupCommand() = %q, extracted empty exe path", cmd)
	}
	// A correctly-quoted Windows command must round-trip through the same
	// parser that readback uses, and must point back at this executable.
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable(): %v", err)
	}
	if !startupCommandMatchesExecutable(cmd, exe) {
		t.Fatalf("startupCommand() = %q does not point to self after parsing", cmd)
	}
}

func TestStartupCommandMatchesExecutable(t *testing.T) {
	tests := []struct {
		name       string
		command    string
		executable string
		want       bool
	}{
		{"current hidden command", `"C:\Program Files\ClipBox\clipbox.exe" --hidden`, `C:\Program Files\ClipBox\clipbox.exe`, true},
		{"same executable without hidden flag", `"C:\Program Files\ClipBox\clipbox.exe"`, `C:\Program Files\ClipBox\clipbox.exe`, false},
		{"different executable", `"C:\Old\clipbox.exe" --hidden`, `C:\New\clipbox.exe`, false},
		{"similar flag is not hidden", `"C:\Program Files\ClipBox\clipbox.exe" --hide`, `C:\Program Files\ClipBox\clipbox.exe`, false},
		{"hidden flag case-insensitive", `"C:\Program Files\ClipBox\clipbox.exe" --HIDDEN`, `C:\Program Files\ClipBox\clipbox.exe`, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := startupCommandMatchesExecutable(tc.command, tc.executable); got != tc.want {
				t.Fatalf("startupCommandMatchesExecutable(%q, %q) = %v, want %v", tc.command, tc.executable, got, tc.want)
			}
		})
	}
}

// TestStartupEntryNeedsRepair 钉住 2026-11-06 的事故：临时副本运行一次就把自启
// 改写成自己，副本删除后开机不再启动。指向另一个仍存在的副本时必须保持不动，
// 只有缺失、指向已删除文件、或指向自己但缺 --hidden 时才改写。
func TestStartupEntryNeedsRepair(t *testing.T) {
	const self = `C:\Users\u\AppData\Local\Programs\ClipBox\ClipBox.exe`
	existing := map[string]bool{self: true, `D:\Tools\ClipBox.exe`: true}
	missing := func(path string) bool { return !existing[path] }
	cases := []struct {
		name    string
		command string
		want    bool
	}{
		{"no entry", ``, true},
		{"already points here", `"` + self + `" --hidden`, false},
		{"points here without --hidden", `"` + self + `"`, true},
		{"another copy that still exists", `"D:\Tools\ClipBox.exe" --hidden`, false},
		{"a copy that was deleted", `"C:\repo\dist\release\verify-exe.exe" --hidden`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := startupEntryNeedsRepair(tc.command, self, missing); got != tc.want {
				t.Fatalf("startupEntryNeedsRepair(%q) = %v, want %v", tc.command, got, tc.want)
			}
		})
	}
}

// readRunValue 读取当前用户的 Run 键值（只读）。
func readRunValue(t *testing.T, name string) string {
	t.Helper()
	key, err := registry.OpenKey(registry.CURRENT_USER, startupRegistryPath, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()
	value, _, err := key.GetStringValue(name)
	if err != nil {
		return ""
	}
	return value
}

// TestSetStartAtLoginNeverWritesFromTestBinary 钉住 2026-10-29 的事故：
// 测试进程曾经把自己的临时路径写进 HKCU\...\Run，导致重启后自启指向已删除文件。
func TestSetStartAtLoginNeverWritesFromTestBinary(t *testing.T) {
	if !underTest() {
		t.Fatal("expected the test process to be recognized as a test binary")
	}
	before := readRunValue(t, startupRegistryName)
	if err := setStartAtLogin(true); err != nil {
		t.Fatalf("setStartAtLogin(true) error = %v", err)
	}
	if after := readRunValue(t, startupRegistryName); after != before {
		t.Fatalf("test binary changed the Run value: %q -> %q", before, after)
	}
	if err := setStartAtLogin(false); err != nil {
		t.Fatalf("setStartAtLogin(false) error = %v", err)
	}
	if after := readRunValue(t, startupRegistryName); after != before {
		t.Fatalf("test binary removed the Run value: %q -> %q", before, after)
	}
}

func TestStartupApprovedDisabled(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
		want bool
	}{
		{"no value", nil, false},
		{"enabled", []byte{0x02, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, false},
		{"no decision", []byte{0x00, 0, 0, 0}, false},
		{"disabled by user", []byte{0x03, 0xCA, 0xFE, 0x4F, 0x0E, 0x0D, 0x5D, 0xDD, 0x01}, true},
		{"disabled variant 0x06", []byte{0x06, 0, 0, 0}, true},
		{"disabled variant 0x0a", []byte{0x0A, 0, 0, 0}, true},
	}
	for _, tc := range cases {
		if got := startupApprovedDisabled(tc.raw); got != tc.want {
			t.Errorf("%s: startupApprovedDisabled(% X) = %v, want %v", tc.name, tc.raw, got, tc.want)
		}
	}
}
