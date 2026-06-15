package main

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const startupRegistryPath = `Software\Microsoft\Windows\CurrentVersion\Run`
const startupRegistryName = "ClipBox"

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

func getStartAtLogin() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, startupRegistryPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()

	value, _, err := key.GetStringValue(startupRegistryName)
	if err != nil {
		return false
	}

	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(value), strings.ToLower(exe))
}

func startupCommand() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%q --hidden", exe), nil
}
