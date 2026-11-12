package main

import (
	"path/filepath"
	"strings"
	"sync"

	"clipbox/internal/windowutil"
)

const sourceIconCacheLimit = 64

var (
	sourceIconMu    sync.Mutex
	sourceIconCache = make(map[string]string) // lowercased path -> PNG data URL ("" = no icon)
)

// GetSourceIcon returns the executable's icon as a PNG data URL, or "" when
// the path is empty, the file is missing, or no icon can be extracted.
// Results (including misses) are cached per lowercased path.
func (a *App) GetSourceIcon(exePath string) (string, error) {
	if exePath == "" || !filepath.IsAbs(exePath) || strings.HasPrefix(exePath, `\\`) {
		return "", nil
	}
	key := strings.ToLower(exePath)

	sourceIconMu.Lock()
	if url, ok := sourceIconCache[key]; ok {
		sourceIconMu.Unlock()
		return url, nil
	}
	sourceIconMu.Unlock()

	url, err := windowutil.ExtractIconDataURL(exePath)
	if err != nil {
		url = ""
	}

	sourceIconMu.Lock()
	cachedURL, cached := sourceIconCache[key]
	if !cached {
		if len(sourceIconCache) >= sourceIconCacheLimit {
			sourceIconCache = make(map[string]string)
		}
		sourceIconCache[key] = url
		cachedURL = url
	}
	sourceIconMu.Unlock()

	if err != nil && !cached {
		a.logErrorf("extract source icon failed path=%q: %v", exePath, err)
	}
	return cachedURL, nil
}
