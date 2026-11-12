package main

import (
	"encoding/base64"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

func protectTranslationKey(key string) (string, error) {
	data := []byte(key)
	if len(data) == 0 {
		return "", nil
	}
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return "", fmt.Errorf("Windows 无法加密 API Key，设置未保存")
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return base64.StdEncoding.EncodeToString(unsafe.Slice(out.Data, out.Size)), nil
}

func unprotectTranslationKey(protected string) (string, error) {
	if protected == "" {
		return "", nil // Local compatible services may not require a key.
	}
	data, err := base64.StdEncoding.DecodeString(protected)
	if err != nil || len(data) == 0 {
		return "", fmt.Errorf("API Key 无法读取，请重新填写并保存")
	}
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return "", fmt.Errorf("API Key 无法解密，请在当前 Windows 账户下重新填写并保存")
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	plain := unsafe.Slice(out.Data, out.Size)
	defer clear(plain)
	return string(plain), nil
}
