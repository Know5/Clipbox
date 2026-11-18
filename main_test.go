package main

import (
	"os"
	"strings"
	"testing"
)

func TestEnsureWebViewMemoryArgs(t *testing.T) {
	t.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", "")
	ensureWebViewMemoryArgs()
	got := os.Getenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS")
	if got != webviewMemoryArgs {
		t.Fatalf("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS = %q, want %q", got, webviewMemoryArgs)
	}
	for _, want := range []string{"--disable-features=SpareRendererForSitePerProcess", "--renderer-process-limit=1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("webviewMemoryArgs %q missing %q", got, want)
		}
	}

	const custom = "--renderer-process-limit=2 --custom-flag"
	t.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", custom)
	ensureWebViewMemoryArgs()
	if got := os.Getenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS"); got != custom {
		t.Fatalf("ensureWebViewMemoryArgs overwrote custom env = %q, want %q", got, custom)
	}
}

func TestResidentHideClearsPendingShowAndSettings(t *testing.T) {
	app := NewApp(t.TempDir(), true)
	r := &resident{
		app:         app,
		pendingShow: true,
		settings:    true,
	}
	r.hide()
	if r.pendingShow || r.settings {
		t.Fatalf("after hide(): pendingShow=%v settings=%v, want both false", r.pendingShow, r.settings)
	}
	if r.idle != nil {
		r.idle.Stop()
	}
}

func TestNewUIBridgeClearsTokenEnv(t *testing.T) {
	t.Setenv("CLIPBOX_UI_PIPE", "")
	t.Setenv("CLIPBOX_UI_TOKEN", "transient-secret-token")

	if _, err := newUIBridge(); err == nil {
		t.Fatal("newUIBridge() succeeded with empty CLIPBOX_UI_PIPE, want error")
	}
	if got := os.Getenv("CLIPBOX_UI_TOKEN"); got != "" {
		t.Fatalf("CLIPBOX_UI_TOKEN still set after newUIBridge(): %q", got)
	}
}
