package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const startupRegistryPath = `Software\Microsoft\Windows\CurrentVersion\Run`
const startupRegistryName = "ClipBox"

// setStartAtLogin writes or removes the ClipBox entry in the per-user Run key,
// pointing at the CURRENT executable. This keeps auto-start working even when
// the portable exe is moved to a new folder.
func setStartAtLogin(enabled bool) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, startupRegistryPath, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()

	if !enabled {
		err := key.DeleteValue(startupRegistryName)
		if err == registry.ErrNotExist {
			return nil
		}
		return err
	}

	command, err := startupCommand()
	if err != nil {
		return err
	}
	return key.SetStringValue(startupRegistryName, command)
}

// startupRegistryEntry returns the raw command stored in the Run key and
// whether it points at this executable with the required hidden-start flag.
func startupRegistryEntry() (value string, pointsHere bool) {
	key, err := registry.OpenKey(registry.CURRENT_USER, startupRegistryPath, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer key.Close()

	value, _, err = key.GetStringValue(startupRegistryName)
	if err != nil {
		return "", false
	}
	exe, err := os.Executable()
	if err != nil {
		return value, false
	}
	return value, startupCommandMatchesExecutable(value, exe)
}

// registryHasStartupEntry reports whether ANY ClipBox Run entry exists,
// regardless of which path it points to. Used for one-time migration of the
// intent flag for users upgrading from the registry-as-source-of-truth build.
func registryHasStartupEntry() bool {
	value, _ := startupRegistryEntry()
	return strings.TrimSpace(value) != ""
}

// reconcileStartAtLogin makes the registry match the intended state using the
// current executable path. When enabling, it always rewrites the value so a
// relocated portable exe self-heals its stale/dangling Run entry.
func reconcileStartAtLogin(intended bool) error {
	if !intended {
		return setStartAtLogin(false)
	}
	value, pointsHere := startupRegistryEntry()
	if pointsHere {
		return nil
	}
	// Missing or pointing elsewhere (relocated exe): rewrite to current path.
	_ = value
	return setStartAtLogin(true)
}

// startupValuePointsToSelf parses the first quoted token (the exe path) out of
// a Run command and compares it, case-insensitively, to this executable.
func startupCommandMatchesExecutable(command, executable string) bool {
	command = strings.TrimSpace(command)
	commandExe := extractExePath(command)
	if executable == "" || !strings.EqualFold(filepath.Clean(commandExe), filepath.Clean(executable)) {
		return false
	}

	args := ""
	if strings.HasPrefix(command, `"`) {
		closingQuote := strings.Index(command[1:], `"`)
		if closingQuote < 0 {
			return false
		}
		args = strings.TrimSpace(command[closingQuote+2:])
	} else if separator := strings.IndexByte(command, ' '); separator >= 0 {
		args = strings.TrimSpace(command[separator+1:])
	}
	for _, arg := range strings.Fields(args) {
		if strings.EqualFold(arg, "--hidden") {
			return true
		}
	}
	return false
}

func startupValuePointsToSelf(value string) bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return startupCommandMatchesExecutable(value, exe)
}

// extractExePath pulls the executable path from a Run command string. It
// handles the quoted form (`"C:\path\clipbox.exe" --hidden`) and the bare form
// (`C:\path\clipbox.exe --hidden`).
func extractExePath(command string) string {
	v := strings.TrimSpace(command)
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, `"`) {
		if end := strings.Index(v[1:], `"`); end >= 0 {
			return v[1 : 1+end]
		}
		return strings.TrimPrefix(v, `"`)
	}
	// Bare path: take everything up to the first space (portable exe paths on
	// Windows may contain spaces, but the unquoted form is only produced by
	// third parties; best effort here).
	if idx := strings.IndexByte(v, ' '); idx >= 0 {
		return v[:idx]
	}
	return v
}

func startupCommand() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	// Plain Windows quoting. NOTE: do NOT use %q here — it Go-escapes
	// backslashes (C:\\Users\\...), corrupting the registry value.
	return fmt.Sprintf(`"%s" --hidden`, filepath.Clean(exe)), nil
}
