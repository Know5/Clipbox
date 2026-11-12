package main

import (
	"clipbox/internal/clipboard"
)

type ClipHistoryChange struct {
	Entry      *clipboard.ClipEntry `json:"entry"`
	DeletedIDs []int64              `json:"deletedIds"`
	Total      int                  `json:"total"`
}

// Callers hold historyMu from the mutation through emission, so concurrent
// captures and user actions publish their committed state in the same order.
func (a *App) currentHistoryChange(entryID int64, deletedIDs []int64) (ClipHistoryChange, error) {
	change := ClipHistoryChange{DeletedIDs: deletedIDs}
	if change.DeletedIDs == nil {
		change.DeletedIDs = []int64{}
	}
	if entryID != 0 {
		entry, err := a.store.GetListEntry(entryID)
		if err != nil {
			return ClipHistoryChange{}, err
		}
		if entry != nil {
			*entry = sanitizeClipDetails(*entry)
		}
		change.Entry = entry
	}
	total, err := a.store.CountAll()
	if err != nil {
		return ClipHistoryChange{}, err
	}
	change.Total = total
	return change, nil
}

func (a *App) emitHistoryChange(change ClipHistoryChange) {
	if a.historyChanged != nil {
		a.historyChanged(change)
	} else if a.ctx != nil {
		a.emit("clips:updated", change)
	}
}

func (a *App) notifyHistoryChange(entryID int64, deletedIDs []int64) {
	change, err := a.currentHistoryChange(entryID, deletedIDs)
	if err != nil {
		a.logErrorf("failed to read committed history change: %v", err)
		if a.ctx != nil {
			a.emit("clips:changed")
		}
		return
	}
	a.emitHistoryChange(change)
}
