package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareVersion(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{name: "newer minor", a: "0.10.0", b: "0.2.0", want: 1},
		{name: "same missing patch", a: "1.2", b: "1.2.0", want: 0},
		{name: "older patch", a: "1.2.2", b: "1.2.3", want: -1},
		{name: "v prefix", a: "v2.0.0", b: "1.9.9", want: 1},
		{name: "prerelease suffix", a: "1.3.0-beta.1", b: "1.2.9", want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := compareVersion(tt.a, tt.b)
			if err != nil {
				t.Fatalf("compareVersion() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("compareVersion(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestValidateUpdateManifestURL(t *testing.T) {
	valid := []string{
		"https://example.com/clipbox/release-manifest.json",
		"http://localhost:8080/release-manifest.json",
		"http://127.0.0.1:8080/release-manifest.json",
		"http://[::1]:8080/release-manifest.json",
	}
	for _, raw := range valid {
		if err := validateUpdateManifestURL(raw); err != nil {
			t.Fatalf("validateUpdateManifestURL(%q) error = %v", raw, err)
		}
	}

	invalid := []string{
		"",
		"file:///tmp/release-manifest.json",
		"http://example.com/release-manifest.json",
		"https://user:pass@example.com/release-manifest.json",
	}
	for _, raw := range invalid {
		if err := validateUpdateManifestURL(raw); err == nil {
			t.Fatalf("validateUpdateManifestURL(%q) succeeded, want error", raw)
		}
	}
}

func TestCheckForUpdatesFindsNewVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.UserAgent(), appName+"/"+appVersion) {
			t.Fatalf("User-Agent = %q, want app/version prefix", r.UserAgent())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"app":"ClipBox",
			"version":"0.3.0",
			"platform":"windows",
			"arch":"amd64",
			"builtAtUtc":"2026-06-09T10:00:00Z",
			"downloadUrl":"https://example.com/ClipBox-0.3.0-windows-amd64.zip",
			"packageSha256":"abc123",
			"releaseNotesUrl":"https://example.com/releases/0.3.0",
			"files":[{"path":"ClipBox.exe","bytes":123,"sha256":"exe123"}]
		}`))
	}))
	defer server.Close()

	app := newTestAppWithStore(t)
	if err := app.store.SetSetting("update_manifest_url", server.URL); err != nil {
		t.Fatalf("SetSetting update URL: %v", err)
	}

	result, err := app.CheckForUpdates(server.URL)
	if err != nil {
		t.Fatalf("CheckForUpdates() error = %v", err)
	}
	if !result.UpdateAvailable || result.LatestVersion != "0.3.0" {
		t.Fatalf("update result = %+v, want update to 0.3.0", result)
	}
	if result.DownloadURL == "" || result.ReleaseNotesURL == "" || result.SHA256 != "abc123" {
		t.Fatalf("update metadata = %+v, want URLs and checksum", result)
	}
}

func TestDownloadUpdateToPathVerifiesSHA256(t *testing.T) {
	payload := []byte("clipbox update package")
	expected := sha256String(payload)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "ClipBox-update.zip")
	result, err := downloadUpdateToPath(server.URL+"/ClipBox-update.zip", target, expected)
	if err != nil {
		t.Fatalf("downloadUpdateToPath() error = %v", err)
	}
	if !result.Verified || result.SHA256 != expected || result.Bytes != int64(len(payload)) {
		t.Fatalf("download result = %+v, want verified bytes and sha", result)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile target: %v", err)
	}
	if string(data) != string(payload) {
		t.Fatalf("downloaded data = %q, want payload", data)
	}
}

func TestDownloadUpdateToPathRejectsBadSHA256(t *testing.T) {
	payload := []byte("clipbox update package")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "ClipBox-update.zip")
	_, err := downloadUpdateToPath(server.URL+"/ClipBox-update.zip", target, strings.Repeat("0", 64))
	if err == nil || !strings.Contains(err.Error(), "SHA256 mismatch") {
		t.Fatalf("downloadUpdateToPath() err = %v, want SHA mismatch", err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("target exists after failed download, statErr=%v", statErr)
	}
}

func TestCheckForUpdatesHandlesMissingURL(t *testing.T) {
	app := newTestAppWithStore(t)
	result, err := app.CheckForUpdates("")
	if err != nil {
		t.Fatalf("CheckForUpdates() error = %v", err)
	}
	if result.UpdateAvailable {
		t.Fatalf("UpdateAvailable = true, want false")
	}
	if result.Message == "" {
		t.Fatalf("Message is empty")
	}
}

func sha256String(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
