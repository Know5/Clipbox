package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const startupRegistryPath = `Software\Microsoft\Windows\CurrentVersion\Run`
const startupRegistryName = "ClipBox"

// underTest 报告当前进程是否为 `go test` 构建的测试二进制。
//
// 2026-10-29 定位的事故：TestUpdateAppSettingsFailureKeepsPersistedState 调用
// UpdateAppSettings → reconcileStartAtLogin(true)，把自己的临时路径写进
// HKCU\...\Run；随后 DB 写入失败的回滚又因为"注册表已指向自己"而空转，
// 结果用户的开机自启指向一个测试结束后被删除的文件，重启后静默不启动。
// 因此测试进程一律不写用户注册表。
func underTest() bool {
	name := strings.ToLower(filepath.Base(os.Args[0]))
	return strings.HasSuffix(name, ".test.exe") || strings.HasSuffix(name, ".test")
}

// setStartAtLogin writes or removes the ClipBox entry in the per-user Run key,
// pointing at the CURRENT executable.
func setStartAtLogin(enabled bool) error {
	if underTest() {
		return nil
	}
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

// startupRegistryValue returns the raw command stored in the Run key, or "".
func startupRegistryValue() string {
	key, err := registry.OpenKey(registry.CURRENT_USER, startupRegistryPath, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()

	value, _, err := key.GetStringValue(startupRegistryName)
	if err != nil {
		return ""
	}
	return value
}

// registryHasStartupEntry reports whether ANY ClipBox Run entry exists,
// regardless of which path it points to. Used for one-time migration of the
// intent flag for users upgrading from the registry-as-source-of-truth build.
func registryHasStartupEntry() bool {
	return strings.TrimSpace(startupRegistryValue()) != ""
}

// reconcileStartAtLogin aligns the Run key with the intended state; it runs at
// startup and on every settings save. When enabled it only repairs the entry
// (see startupEntryNeedsRepair), so a temporary copy of ClipBox cannot take
// auto-start over from the copy the user installed. Moving auto-start to
// another copy is explicit: switching it off removes the entry, switching it
// back on from that copy writes it fresh.
func reconcileStartAtLogin(intended bool) error {
	if !intended {
		return setStartAtLogin(false)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if !startupEntryNeedsRepair(startupRegistryValue(), exe, pathMissing) {
		return nil
	}
	return setStartAtLogin(true)
}

// startupEntryNeedsRepair reports whether the Run command should be rewritten
// to launch executable: the entry is missing, is ours but lacks --hidden, or
// launches an exe that no longer exists. An entry owned by another copy that
// still exists is left alone, whatever its version: a newer copy run from a
// temporary folder (release verification, an exe opened inside a zip) would
// otherwise take over and leave the entry dangling once that folder is gone.
//
// 2026-11-06 发现：Run 值指向已删除的 dist\release\verify-exe.exe（发版验证时的临时
// 副本），开机不再启动。旧逻辑只要 Run 值不指向当前 exe 就改写，任何临时副本运行一次
// 都会抢走自启。
func startupEntryNeedsRepair(command, executable string, missing func(path string) bool) bool {
	if startupCommandMatchesExecutable(command, executable) {
		return false
	}
	target := extractExePath(command)
	if target == "" || strings.EqualFold(filepath.Clean(target), filepath.Clean(executable)) {
		return true
	}
	return missing(target)
}

// pathMissing reports whether path definitely does not exist. Other Stat
// errors (for example access denied) count as present, so an entry is never
// taken over just because its target could not be checked.
func pathMissing(path string) bool {
	_, err := os.Stat(path)
	return errors.Is(err, fs.ErrNotExist)
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
