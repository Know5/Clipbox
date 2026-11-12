package windowutil

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

const (
	dwmRegistryKey  = `Software\Microsoft\Windows\DWM`
	accentColorName = "AccentColor"
)

// SystemAccentColor reads the DWM accent colour from
// HKCU\Software\Microsoft\Windows\DWM\AccentColor.
// The value is a 0xAABBGGRR DWORD; ok is false when it is missing or
// cannot be read.
func SystemAccentColor() (uint32, bool) {
	key, err := registry.OpenKey(registry.CURRENT_USER, dwmRegistryKey, registry.QUERY_VALUE)
	if err != nil {
		return 0, false
	}
	defer key.Close()

	value, _, err := key.GetIntegerValue(accentColorName)
	if err != nil {
		return 0, false
	}
	return uint32(value), true
}

// AccentColorHex converts an AccentColor DWORD in 0xAABBGGRR order to a
// lowercase #RRGGBB string. The alpha byte is ignored.
func AccentColorHex(value uint32) string {
	return fmt.Sprintf("#%02x%02x%02x", byte(value), byte(value>>8), byte(value>>16))
}

// SystemDarkMode reports whether the user prefers dark app mode
// (HKCU\...\Themes\Personalize\AppsUseLightTheme == 0). Missing or unreadable
// values fall back to dark, matching the app default theme.
func SystemDarkMode() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return true
	}
	defer key.Close()
	value, _, err := key.GetIntegerValue("AppsUseLightTheme")
	if err != nil {
		return true
	}
	return value == 0
}
