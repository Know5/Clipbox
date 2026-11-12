package ocr

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// Opt in with a generated, non-private image. This exercises the actual WinRT
// OCR engine without changing the clipboard or installing language packages.
func TestWindowsOCRIntegration(t *testing.T) {
	path := os.Getenv("CLIPBOX_OCR_TEST_IMAGE")
	if path == "" {
		t.Skip("set CLIPBOX_OCR_TEST_IMAGE to exercise the Windows OCR engine")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	languages, err := Languages(ctx)
	if err != nil || len(languages) == 0 {
		t.Fatalf("system OCR languages: %+v, %v", languages, err)
	}
	bmp, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Recognize(ctx, bmp, languages[0].Tag)
	if err != nil {
		t.Fatal(err)
	}
	expected := strings.TrimSpace(os.Getenv("CLIPBOX_OCR_TEST_EXPECT"))
	if expected == "" {
		expected = "hello"
	}
	got := strings.ReplaceAll(strings.ToLower(result.Text), " ", "")
	want := strings.ReplaceAll(strings.ToLower(expected), " ", "")
	if !strings.Contains(got, want) {
		t.Fatalf("unexpected OCR text: %q (want %q)", result.Text, expected)
	}
	t.Logf("recognized %q using %s", result.Text, result.Language)
}
