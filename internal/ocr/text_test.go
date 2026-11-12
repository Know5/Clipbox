package ocr

import "testing"

func TestNormalizeTextRemovesCJKSegmentationSpaces(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"han characters", "你 好 世 界", "你好世界"},
		{"cjk punctuation", "你 好 ， 世 界 ！", "你好，世界！"},
		{"latin punctuation is untouched", "Hello , world", "Hello , world"},
		{"latin spaces are untouched", "Hello  world", "Hello  world"},
		{"cjk and latin keep the boundary space", "你好 ClipBox 123", "你好 ClipBox 123"},
		{"latin before cjk punctuation", "ClipBox ， 你好", "ClipBox，你好"},
		{"lines are not joined", "你 好\n世 界", "你好\n世界"},
		{"japanese kana", "こ ん に ち は", "こんにちは"},
		{"japanese prolonged sound mark", "コ ー ヒ ー", "コーヒー"},
		{"korean spaces are meaningful", "안 녕 하 세 요", "안 녕 하 세 요"},
		{"full width space", "你　好", "你好"},
		{"tabs", "你\t好", "你好"},
		{"empty", "", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeText(test.in); got != test.want {
				t.Fatalf("normalizeText(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}
