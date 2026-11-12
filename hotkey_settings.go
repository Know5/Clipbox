package main

import (
	"clipbox/internal/hotkey"
	"errors"
	"fmt"
	"strconv"
)

func hotkeyDisplayName(modifiers, keyCode int) string {
	result := ""
	if modifiers&hotkey.ModControl != 0 {
		result += "Ctrl + "
	}
	if modifiers&hotkey.ModAlt != 0 {
		result += "Alt + "
	}
	if modifiers&hotkey.ModShift != 0 {
		result += "Shift + "
	}
	if modifiers&hotkey.ModWin != 0 {
		result += "Win + "
	}
	keyName := ""
	switch {
	case keyCode >= 0x41 && keyCode <= 0x5A:
		keyName = string(rune('A' + keyCode - 0x41))
	case keyCode >= 0x30 && keyCode <= 0x39:
		keyName = string(rune('0' + keyCode - 0x30))
	case keyCode >= 0x70 && keyCode <= 0x7B:
		keyName = fmt.Sprintf("F%d", keyCode-0x70+1)
	case keyCode == 0x20:
		keyName = "Space"
	case keyCode == 0x0D:
		keyName = "Enter"
	case keyCode == 0xBE:
		keyName = "."
	case keyCode == 0xBC:
		keyName = ","
	case keyCode == 0xBF:
		keyName = "/"
	case keyCode == 0xBA:
		keyName = ";"
	case keyCode == 0xDE:
		keyName = "'"
	case keyCode == 0xDB:
		keyName = "["
	case keyCode == 0xDD:
		keyName = "]"
	case keyCode == 0xDC:
		keyName = "\\"
	case keyCode == 0xC0:
		keyName = "`"
	case keyCode == 0xBD:
		keyName = "-"
	case keyCode == 0xBB:
		keyName = "="
	default:
		keyName = fmt.Sprintf("0x%02X", keyCode)
	}
	return result + keyName
}

func (a *App) GetHotkeySettings() HotkeyConfig {
	modStr, _ := a.store.GetSetting("hotkey_modifiers")
	vkStr, _ := a.store.GetSetting("hotkey_key")
	mod := hotkey.ModControl | hotkey.ModAlt
	vk := 0x56
	if modStr != "" {
		if v, err := strconv.ParseUint(modStr, 10, 32); err == nil {
			mod = int(v)
		}
	}
	if vkStr != "" {
		if v, err := strconv.ParseUint(vkStr, 10, 32); err == nil {
			vk = int(v)
		}
	}
	return HotkeyConfig{
		Modifiers: mod,
		KeyCode:   vk,
		Display:   hotkeyDisplayName(mod, vk),
	}
}

func (a *App) UpdateHotkey(modifiers, keyCode int) error {
	a.hotkeyWriteMu.Lock()
	defer a.hotkeyWriteMu.Unlock()
	if err := validateHotkey(modifiers, keyCode); err != nil {
		return err
	}
	if a.hotkeyManager == nil || a.store == nil {
		return fmt.Errorf("热键服务尚未就绪")
	}
	previous := a.GetHotkeySettings()
	if err := a.hotkeyManager.Rebind(uint32(modifiers), uint32(keyCode)); err != nil {
		return fmt.Errorf("%w；请换一个组合键后重试", err)
	}
	if err := a.store.SetSettings(map[string]string{"hotkey_modifiers": strconv.Itoa(modifiers), "hotkey_key": strconv.Itoa(keyCode)}); err != nil {
		if rollbackErr := a.hotkeyManager.Rebind(uint32(previous.Modifiers), uint32(previous.KeyCode)); rollbackErr != nil {
			return fmt.Errorf("热键保存失败，恢复旧热键也失败，请重新设置：%w", err)
		}
		return fmt.Errorf("热键保存失败，已恢复旧组合：%w", err)
	}
	a.logInfof("updated hotkey modifiers=%d key=%d", modifiers, keyCode)
	return nil
}

func validateHotkey(modifiers, keyCode int) error {
	if modifiers <= 0 || modifiers&^15 != 0 {
		return fmt.Errorf("必须包含至少一个修饰键(Ctrl/Alt/Shift/Win)")
	}
	if keyCode <= 0 || keyCode > 255 || (keyCode >= 0x10 && keyCode <= 0x12) || keyCode == 0x5b || keyCode == 0x5c {
		return fmt.Errorf("必须指定一个按键")
	}
	if reason := reservedHotkeyReason(modifiers, keyCode); reason != "" {
		return errors.New(reason)
	}

	return nil
}

// reservedHotkeyReason returns why a combination cannot be the global hotkey,
// or "" when it can. RegisterHotKey swallows the combination in every
// application, so the hotkey must not equal a shortcut ClipBox itself needs:
// the modified list shortcuts in App.tsx, and the clipboard keys it records and
// sends when pasting.
func reservedHotkeyReason(modifiers, keyCode int) string {
	mods := modifiers & (hotkey.ModControl | hotkey.ModAlt | hotkey.ModShift | hotkey.ModWin)
	digit := (keyCode >= '1' && keyCode <= '9') || (keyCode >= 0x61 && keyCode <= 0x69) // 1-9, Numpad1-9
	switch {
	// App.tsx 对 Ctrl、Alt、Ctrl+Alt 加数字都会触发；带 Shift 时 e.key 变成符号，带 Win 时被排除。
	case digit && mods != 0 && mods&(hotkey.ModShift|hotkey.ModWin) == 0:
		return "Ctrl/Alt + 数字 1~9 是 ClipBox 列表里复制或粘贴第 n 条的快捷键，不能用作唤起热键"
	case mods == hotkey.ModControl && keyCode == 0x0D:
		return "Ctrl + 回车 是 ClipBox 列表里只复制的快捷键，不能用作唤起热键"
	case mods == hotkey.ModControl && (keyCode == 'C' || keyCode == 'V' || keyCode == 'X'):
		return "Ctrl + C/V/X 是系统的复制粘贴键，ClipBox 靠它们记录和粘贴，不能用作唤起热键"
	}
	return ""
}
