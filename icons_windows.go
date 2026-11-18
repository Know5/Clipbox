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
	sourceIconOrder []string                  // oldest-to-most-recent keys
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
		touchSourceIconKeyLocked(key)
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
		if len(sourceIconCache) >= sourceIconCacheLimit && len(sourceIconOrder) > 0 {
			evict := sourceIconOrder[0]
			sourceIconOrder = sourceIconOrder[1:]
			delete(sourceIconCache, evict)
		}
		sourceIconCache[key] = url
		sourceIconOrder = append(sourceIconOrder, key)
		cachedURL = url
	}
	sourceIconMu.Unlock()

	if err != nil && !cached {
		a.logErrorf("extract source icon failed path=%q: %v", exePath, err)
	}
	return cachedURL, nil
}

func touchSourceIconKeyLocked(key string) {
	for i, existing := range sourceIconOrder {
		if existing != key {
			continue
		}
		copy(sourceIconOrder[i:], sourceIconOrder[i+1:])
		sourceIconOrder[len(sourceIconOrder)-1] = key
		return
	}
	sourceIconOrder = append(sourceIconOrder, key)
}
