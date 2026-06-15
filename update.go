package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	maxUpdateManifestBytes = 1024 * 1024
	maxUpdatePackageBytes  = 300 * 1024 * 1024
)

type UpdateCheckResult struct {
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion"`
	UpdateAvailable bool   `json:"updateAvailable"`
	ManifestURL     string `json:"manifestURL"`
	DownloadURL     string `json:"downloadURL"`
	ReleaseNotesURL string `json:"releaseNotesURL"`
	SHA256          string `json:"sha256"`
	PublishedAt     string `json:"publishedAt"`
	CheckedAt       string `json:"checkedAt"`
	Message         string `json:"message"`
}

type UpdateDownloadResult struct {
	Path      string `json:"path"`
	Bytes     int64  `json:"bytes"`
	SHA256    string `json:"sha256"`
	Verified  bool   `json:"verified"`
	Cancelled bool   `json:"cancelled"`
	Message   string `json:"message"`
}

type updateManifest struct {
	App             string               `json:"app"`
	Version         string               `json:"version"`
	Platform        string               `json:"platform"`
	Arch            string               `json:"arch"`
	Package         string               `json:"package"`
	BuiltAtUTC      string               `json:"builtAtUtc"`
	DownloadURL     string               `json:"downloadUrl"`
	DownloadSHA256  string               `json:"downloadSha256"`
	PackageSHA256   string               `json:"packageSha256"`
	ReleaseNotesURL string               `json:"releaseNotesUrl"`
	SHA256          string               `json:"sha256"`
	Update          updateManifestExtras `json:"update"`
	Files           []updateManifestFile `json:"files"`
}

type updateManifestExtras struct {
	DownloadURL     string `json:"downloadUrl"`
	DownloadSHA256  string `json:"downloadSha256"`
	PackageSHA256   string `json:"packageSha256"`
	ReleaseNotesURL string `json:"releaseNotesUrl"`
	SHA256          string `json:"sha256"`
}

type updateManifestFile struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

func (a *App) CheckForUpdates(manifestURL string) (UpdateCheckResult, error) {
	manifestURL = strings.TrimSpace(manifestURL)
	result := UpdateCheckResult{
		CurrentVersion: appVersion,
		ManifestURL:    manifestURL,
		CheckedAt:      time.Now().Format(time.RFC3339),
	}

	if manifestURL == "" {
		result.Message = "未配置更新源"
		return result, nil
	}
	if err := validateUpdateManifestURL(manifestURL); err != nil {
		return result, err
	}

	manifest, err := fetchUpdateManifest(manifestURL)
	if err != nil {
		return result, err
	}
	if strings.TrimSpace(manifest.App) != "" && !strings.EqualFold(strings.TrimSpace(manifest.App), appName) {
		return result, fmt.Errorf("update manifest is for %s, not %s", manifest.App, appName)
	}

	latest := strings.TrimSpace(manifest.Version)
	if latest == "" {
		return result, fmt.Errorf("update manifest missing version")
	}
	cmp, err := compareVersion(latest, appVersion)
	if err != nil {
		return result, err
	}

	result.LatestVersion = latest
	result.PublishedAt = manifest.BuiltAtUTC
	result.DownloadURL = firstNonEmpty(manifest.DownloadURL, manifest.Update.DownloadURL)
	result.ReleaseNotesURL = firstNonEmpty(manifest.ReleaseNotesURL, manifest.Update.ReleaseNotesURL)
	result.SHA256 = firstNonEmpty(
		manifest.PackageSHA256,
		manifest.DownloadSHA256,
		manifest.Update.PackageSHA256,
		manifest.Update.DownloadSHA256,
		manifest.SHA256,
		manifest.Update.SHA256,
	)

	switch {
	case cmp > 0:
		result.UpdateAvailable = true
		result.Message = fmt.Sprintf("发现新版本 %s", latest)
	case cmp == 0:
		result.Message = "已是最新版本"
	default:
		result.Message = "当前版本较新"
	}
	return result, nil
}

func (a *App) DownloadUpdatePackage(downloadURL, expectedSHA256 string) (UpdateDownloadResult, error) {
	if a.ctx == nil {
		return UpdateDownloadResult{}, fmt.Errorf("app is not ready")
	}
	downloadURL = strings.TrimSpace(downloadURL)
	if downloadURL == "" {
		return UpdateDownloadResult{}, fmt.Errorf("download URL is required")
	}
	if err := validateUpdateManifestURL(downloadURL); err != nil {
		return UpdateDownloadResult{}, err
	}

	defaultName := updateDownloadDefaultName(downloadURL)
	savePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "保存 ClipBox 更新包",
		DefaultFilename: defaultName,
		Filters: []runtime.FileFilter{
			{DisplayName: "Release Package (*.zip)", Pattern: "*.zip"},
			{DisplayName: "Installer (*.exe)", Pattern: "*.exe"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		a.logErrorf("update download dialog failed: %v", err)
		return UpdateDownloadResult{}, err
	}
	if savePath == "" {
		return UpdateDownloadResult{Cancelled: true, Message: "已取消下载"}, nil
	}

	result, err := downloadUpdateToPath(downloadURL, savePath, expectedSHA256)
	if err != nil {
		a.logErrorf("download update failed url=%s path=%s: %v", downloadURL, savePath, err)
		return result, err
	}
	a.logInfof("downloaded update package path=%s bytes=%d verified=%v sha256=%s", result.Path, result.Bytes, result.Verified, result.SHA256)
	return result, nil
}

func fetchUpdateManifest(manifestURL string) (updateManifest, error) {
	client := updateHTTPClient()
	req, err := http.NewRequest(http.MethodGet, manifestURL, nil)
	if err != nil {
		return updateManifest{}, err
	}
	req.Header.Set("User-Agent", appName+"/"+appVersion)

	resp, err := client.Do(req)
	if err != nil {
		return updateManifest{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return updateManifest{}, fmt.Errorf("update manifest returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxUpdateManifestBytes+1))
	if err != nil {
		return updateManifest{}, err
	}
	if len(body) > maxUpdateManifestBytes {
		return updateManifest{}, fmt.Errorf("update manifest is too large")
	}

	var manifest updateManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return updateManifest{}, err
	}
	return manifest, nil
}

func downloadUpdateToPath(downloadURL, targetPath, expectedSHA256 string) (UpdateDownloadResult, error) {
	result := UpdateDownloadResult{Path: targetPath}
	if err := validateUpdateManifestURL(downloadURL); err != nil {
		return result, err
	}
	expected, err := normalizeExpectedSHA256(expectedSHA256)
	if err != nil {
		return result, err
	}
	if strings.TrimSpace(targetPath) == "" {
		return result, fmt.Errorf("target path is required")
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return result, err
	}

	client := updateHTTPClient()
	req, err := http.NewRequest(http.MethodGet, downloadURL, nil)
	if err != nil {
		return result, err
	}
	req.Header.Set("User-Agent", appName+"/"+appVersion)

	resp, err := client.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, fmt.Errorf("update download returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxUpdatePackageBytes {
		return result, fmt.Errorf("update package is too large")
	}

	tempFile, err := os.CreateTemp(filepath.Dir(targetPath), "."+filepath.Base(targetPath)+".*.download")
	if err != nil {
		return result, err
	}
	tempPath := tempFile.Name()
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.Remove(tempPath)
		}
	}()

	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(tempFile, hash), io.LimitReader(resp.Body, maxUpdatePackageBytes+1))
	closeErr := tempFile.Close()
	if copyErr != nil {
		return result, copyErr
	}
	if closeErr != nil {
		return result, closeErr
	}
	if written > maxUpdatePackageBytes {
		return result, fmt.Errorf("update package is too large")
	}

	actual := hex.EncodeToString(hash.Sum(nil))
	result.Bytes = written
	result.SHA256 = actual
	if expected != "" {
		if actual != expected {
			return result, fmt.Errorf("update package SHA256 mismatch")
		}
		result.Verified = true
	}

	if err := moveDownloadedPackage(tempPath, targetPath); err != nil {
		return result, err
	}
	cleanupTemp = false
	result.Message = "下载完成"
	return result, nil
}

func moveDownloadedPackage(tempPath, targetPath string) error {
	if err := os.Rename(tempPath, targetPath); err == nil {
		return nil
	}
	if _, err := os.Stat(targetPath); err == nil {
		if removeErr := os.Remove(targetPath); removeErr != nil {
			return removeErr
		}
	}
	return os.Rename(tempPath, targetPath)
}

func updateHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return http.ErrUseLastResponse
			}
			return validateUpdateManifestURL(req.URL.String())
		},
	}
}

func validateUpdateManifestURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return err
	}
	if parsed.Scheme == "" || parsed.Hostname() == "" {
		return fmt.Errorf("update manifest URL must include scheme and host")
	}
	if parsed.User != nil {
		return fmt.Errorf("update manifest URL must not include credentials")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		return nil
	case "http":
		if isLoopbackHost(parsed.Hostname()) {
			return nil
		}
		return fmt.Errorf("update manifest URL must use HTTPS")
	default:
		return fmt.Errorf("unsupported update manifest URL scheme: %s", parsed.Scheme)
	}
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func updateDownloadDefaultName(downloadURL string) string {
	parsed, err := url.Parse(downloadURL)
	if err != nil {
		return "ClipBox-update.zip"
	}
	name := path.Base(parsed.EscapedPath())
	if unescaped, err := url.PathUnescape(name); err == nil {
		name = unescaped
	}
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == "/" {
		return "ClipBox-update.zip"
	}
	cleaned := safeFilenamePart(strings.TrimSuffix(name, filepath.Ext(name)), 80)
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".zip", ".exe", ".msi":
	default:
		ext = ".zip"
	}
	return cleaned + ext
}

func normalizeExpectedSHA256(value string) (string, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, "sha256:") {
		value = strings.TrimSpace(strings.TrimPrefix(value, "sha256:"))
	}
	if len(value) != 64 {
		return "", fmt.Errorf("expected SHA256 must be 64 hex characters")
	}
	for _, r := range value {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return "", fmt.Errorf("expected SHA256 must be hex")
	}
	return value, nil
}

func compareVersion(a, b string) (int, error) {
	left, err := parseVersionParts(a)
	if err != nil {
		return 0, fmt.Errorf("invalid version %q: %w", a, err)
	}
	right, err := parseVersionParts(b)
	if err != nil {
		return 0, fmt.Errorf("invalid version %q: %w", b, err)
	}

	maxLen := len(left)
	if len(right) > maxLen {
		maxLen = len(right)
	}
	for i := 0; i < maxLen; i++ {
		l, r := 0, 0
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		if l > r {
			return 1, nil
		}
		if l < r {
			return -1, nil
		}
	}
	return 0, nil
}

func parseVersionParts(version string) ([]int, error) {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(strings.TrimPrefix(version, "v"), "V")
	if version == "" {
		return nil, fmt.Errorf("empty version")
	}
	if idx := strings.IndexAny(version, "-+"); idx >= 0 {
		version = version[:idx]
	}
	segments := strings.Split(version, ".")
	parts := make([]int, 0, len(segments))
	for _, segment := range segments {
		if segment == "" {
			return nil, fmt.Errorf("empty version segment")
		}
		for _, r := range segment {
			if r < '0' || r > '9' {
				return nil, fmt.Errorf("non-numeric version segment %q", segment)
			}
		}
		value, err := strconv.Atoi(segment)
		if err != nil {
			return nil, err
		}
		parts = append(parts, value)
	}
	return parts, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
