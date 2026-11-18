package storage

import (
	"os"
	"path/filepath"
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
