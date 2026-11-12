package main

import (
	"fmt"

	"clipbox/internal/clipboard"
	"clipbox/internal/storage"
)

func (a *App) persistCapturedClip(entry clipboard.ClipEntry) (ClipHistoryChange, error) {
	a.historyMu.Lock()
	defer a.historyMu.Unlock()
	if a.store == nil {
		return ClipHistoryChange{}, fmt.Errorf("app is not ready")
	}
	id, err := a.store.SaveOrUpdate(entry)
	if err != nil {
		a.store.DiscardEntryFiles(entry)
		return ClipHistoryChange{}, err
	}
	settings := a.loadAppSettings()
	imageBudget := 0
	if entry.Type == "image" {
		imageBudget = settings.MaxImageStorageMB
	}
	// Capturing text cannot increase image storage. Avoid re-statting every
	// stored screenshot on each keystroke-copy; startup, policy changes and
	// image captures still enforce the full image budget.
	cleanup, cleanupErr := a.store.CleanupWithResult(storage.CleanupPolicy{
		MaxClips: settings.MaxClips, MaxDays: settings.RetentionDays, MaxImageMB: imageBudget,
	})
	if cleanupErr != nil {
		a.logErrorf("failed to cleanup captured clips: %v", cleanupErr)
	}
	change, err := a.currentHistoryChange(id, cleanup.DeletedIDs)
	if err != nil {
		return ClipHistoryChange{}, err
	}
	a.emitHistoryChange(change)
	return change, nil
}
