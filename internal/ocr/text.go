package ocr

import (
	"strings"
	"unicode"
)

// Windows OCR inserts spaces between Chinese/Japanese characters and around
// CJK punctuation. Those spaces are artifacts of the recognizer's word
// segmentation, not part of the source text. Remove them without touching
// spaces inside Latin text, between CJK and Latin/digits, or across lines.
func normalizeText(text string) string {
	if text == "" {
		return text
	}
	runes := []rune(text)
	var builder strings.Builder
	builder.Grow(len(text))
	for i, r := range runes {
		if r == '\n' || r == '\r' {
			builder.WriteRune(r)
			continue
		}
		if !unicode.IsSpace(r) {
			builder.WriteRune(r)
			continue
		}
		if dropSpace(previousNonSpace(runes, i), nextNonSpace(runes, i)) {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

func previousNonSpace(runes []rune, index int) rune {
	for i := index - 1; i >= 0; i-- {
		if runes[i] == '\n' || runes[i] == '\r' {
			return 0
		}
		if !unicode.IsSpace(runes[i]) {
			return runes[i]
		}
	}
	return 0
}

func nextNonSpace(runes []rune, index int) rune {
	for i := index + 1; i < len(runes); i++ {
		if runes[i] == '\n' || runes[i] == '\r' {
			return 0
		}
		if !unicode.IsSpace(runes[i]) {
			return runes[i]
		}
	}
	return 0
}

func dropSpace(previous, next rune) bool {
	if isCJKPunctuation(previous) || isCJKPunctuation(next) {
		return true
	}
	return isCJK(previous) && isCJK(next)
}

func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		r == '\u30fc' // Katakana-Hiragana prolonged sound mark
}

func isCJKPunctuation(r rune) bool {
	return strings.ContainsRune("，。！？；：、（）【】《》〈〉「」『』“”‘’…—～·〔〕［］｛｝", r)
}
