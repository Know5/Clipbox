package ocr

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
)

//go:embed recognize.ps1
var script string

type Result struct {
	Text     string `json:"text"`
	Language string `json:"language"`
	Scaled   bool   `json:"scaled"`
}

type Language struct {
	Tag  string `json:"tag"`
	Name string `json:"name"`
}

// Windows' OCR engine is loaded only for this request. The helper exits after
// recognition, leaving no model or additional runtime resident in ClipBox.
func Recognize(ctx context.Context, bmp []byte, language string) (Result, error) {
	file, err := os.CreateTemp("", "clipbox-ocr-*.bmp")
	if err != nil {
		return Result{}, fmt.Errorf("无法创建识别临时文件")
	}
	path := file.Name()
	defer os.Remove(path)
	_, writeErr := file.Write(bmp)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return Result{}, fmt.Errorf("无法写入识别临时文件")
	}
	output, err := run(ctx, "recognize", path, language)
	if err != nil {
		return Result{}, err
	}
	var result Result
	if err := json.Unmarshal(output, &result); err != nil {
		return Result{}, fmt.Errorf("无法读取 OCR 结果")
	}
	result.Text = normalizeText(result.Text)
	return result, nil
}

func Languages(ctx context.Context) ([]Language, error) {
	output, err := run(ctx, "languages", "", "")
	if err != nil {
		return nil, err
	}
	var languages []Language
	if err := json.Unmarshal(output, &languages); err != nil {
		return nil, fmt.Errorf("无法读取系统 OCR 语言")
	}
	return languages, nil
}

func run(ctx context.Context, mode, path, language string) ([]byte, error) {
	encoded := utf16.Encode([]rune(script))
	data := make([]byte, len(encoded)*2)
	for i, value := range encoded {
		binary.LittleEndian.PutUint16(data[i*2:], value)
	}
	// Resolve the system binary explicitly; never execute a PATH shadow.
	powershell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	cmd := exec.CommandContext(ctx, powershell, "-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(data))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	cmd.Env = append(os.Environ(), "CLIPBOX_OCR_MODE="+mode, "CLIPBOX_OCR_IMAGE="+path, "CLIPBOX_OCR_LANGUAGE="+language)
	var output, stderr bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("识别已取消或超时")
		}
		switch {
		case strings.Contains(stderr.String(), "OCR_LANGUAGE_UNAVAILABLE"):
			return nil, fmt.Errorf("系统没有可用的 OCR 语言，请在 Windows 设置 → 语言和区域中安装对应语言的文字识别组件")
		case strings.Contains(stderr.String(), "OCR_IMAGE_TOO_LARGE"):
			return nil, fmt.Errorf("图片尺寸超过系统 OCR 上限，请缩小截图范围后重试")
		default:
			return nil, fmt.Errorf("Windows 文字识别失败，请确认系统 OCR 语言组件可用且图片完整")
		}
	}
	return bytes.TrimSpace(bytes.TrimPrefix(output.Bytes(), []byte{0xef, 0xbb, 0xbf})), nil
}
