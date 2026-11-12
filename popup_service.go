package main

import (
	"math"

	"clipbox/internal/popup"
	"clipbox/internal/windowutil"
)

// popupPage supplies the native popup's list from the same paged queries the
// webview list uses. It runs on a popup background goroutine.
func (a *App) popupPage(query, filter string, limit, offset int) ([]popup.Row, int, error) {
	clipType := ""
	if filter == "text" || filter == "image" {
		clipType = filter
	}
	var (
		page ClipPage
		err  error
	)
	switch {
	case query != "" && filter == "all":
		page, err = a.SearchClipPage(query, limit, offset)
	case query != "":
		page, err = a.SearchClipPageByType(query, clipType, limit, offset)
	case filter == "all":
		page, err = a.GetClipPage(limit, offset)
	default:
		page, err = a.GetClipPageByType(clipType, limit, offset)
	}
	if err != nil {
		return nil, 0, err
	}
	rows := make([]popup.Row, 0, len(page.Items))
	for _, item := range page.Items {
		rows = append(rows, popup.Row{
			ID:        item.ID,
			Kind:      item.Type,
			Preview:   item.Preview,
			Pinned:    item.Pinned,
			Timestamp: item.Timestamp,
		})
	}
	return rows, page.Total, nil
}

// applyPopupTheme pushes the current settings theme onto the popup. It is
// cheap (one registry read at most) and safe to call on every settings change.
func (a *App) applyPopupTheme() {
	if a.popup == nil {
		return
	}
	a.popup.SetTheme(popupTheme(a.loadAppSettings()))
}

func popupTheme(settings AppSettings) popup.Theme {
	dark := settings.Theme == "dark"
	if settings.Theme == "system" {
		dark = windowutil.SystemDarkMode()
	}
	theme := popup.Theme{
		Background: 0x00F9F9F9,
		Foreground: 0x001A1A1A,
		Muted:      0x006B6B6B,
		Border:     0x00E0E0E0,
	}
	if dark {
		theme = popup.Theme{
			Background: 0x00202020,
			Foreground: 0x00FFFFFF,
			Muted:      0x00949494,
			Border:     0x00333333,
		}
	}
	// System accent with the same contrast rule as theme.ts: white vs near-black,
	// whichever contrasts more with the accent. Fallback matches the CSS default.
	accent, accentText := uint32(0x4CA6FF), uint32(0x06121D)
	if value, ok := windowutil.SystemAccentColor(); ok {
		r, g, b := byte(value), byte(value>>8), byte(value>>16)
		accent = uint32(r) | uint32(g)<<8 | uint32(b)<<16
		accentText = 0x00FFFFFF
		if contrastRatio(r, g, b, 11, 11, 11) > contrastRatio(r, g, b, 255, 255, 255) {
			accentText = 0x000B0B0B
		}
	}
	theme.Accent = accent
	theme.AccentText = accentText
	return theme
}

// contrastRatio is the WCAG relative luminance ratio (a vs b), matching
// frontend/src/theme.ts contrastText.
func contrastRatio(r1, g1, b1, r2, g2, b2 byte) float64 {
	l1 := luminance(r1, g1, b1)
	l2 := luminance(r2, g2, b2)
	hi, lo := math.Max(l1, l2), math.Min(l1, l2)
	return (hi + 0.05) / (lo + 0.05)
}

func luminance(r, g, b byte) float64 {
	return 0.2126*toLinear(r) + 0.7152*toLinear(g) + 0.0722*toLinear(b)
}

func toLinear(channel byte) float64 {
	c := float64(channel) / 255
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// openDetailFromPopup spawns the webview child with the detail panel for id.
// The native popup cannot show notes/tags editing or the image preview, so
// this hands off to the existing UI.
func (a *App) openDetailFromPopup(id int64) {
	if a.resident == nil {
		return
	}
	a.mu.Lock()
	a.windowVisible = true
	a.mu.Unlock()
	a.resident.showDetail(id)
	a.updateTrayState()
}
