package storage

import (
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Only a definitely missing absolute file is eligible for maintenance deletion.
// Legacy image bodies and access/I/O errors must never be treated as absence.
func missingImageFile(content string) bool {
	if !filepath.IsAbs(content) {
		return false
	}
	_, err := os.Stat(content)
	return os.IsNotExist(err)
}

func legacyBMP(content string) bool {
	if len(content) > 70*1024*1024 || filepath.IsAbs(content) {
		return false
	}
	var header [14]byte
	_, err := io.ReadFull(base64.NewDecoder(base64.StdEncoding, strings.NewReader(content)), header[:])
	return err == nil && header[0] == 'B' && header[1] == 'M'
}

// Capture and backup import share the same recovery rule. Preserve usable
// existing data; retain an incoming file if it repairs a missing original.
func chooseImageContent(existing, incoming string) (content string, discard []string) {
	if existing == incoming || incoming == "" {
		return existing, nil
	}
	if missingImageFile(existing) {
		if info, err := os.Stat(incoming); err == nil && !info.IsDir() {
			return incoming, nil
		}
	}
	return existing, []string{incoming}
}
