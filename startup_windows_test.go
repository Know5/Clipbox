package main

import "testing"

func TestExtractExePath(t *testing.T) {
	cases := []struct {
		name    string
		command string
		want    string
	}{
		{"quoted with args", `"C:\Users\QGS\clipbox.exe" --hidden`, `C:\Users\QGS\clipbox.exe`},
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
	if !startupValuePointsToSelf(cmd) {
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
