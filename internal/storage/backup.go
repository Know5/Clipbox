package storage

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"clipbox/internal/clipboard"
)

const backupFormat = "clipbox-backup"
const backupVersion = 1

const (
	maxBackupZipFiles        = 20050
	maxBackupClips           = 10000
	maxBackupSettings        = 500
	maxBackupManifestBytes   = 64 * 1024
	maxBackupSettingsBytes   = 2 * 1024 * 1024
	maxBackupClipsBytes      = 64 * 1024 * 1024
	maxBackupImageBytes      = 50 * 1024 * 1024
	maxBackupTotalImageBytes = 1024 * 1024 * 1024
)

type BackupStats struct {
	Path          string `json:"path"`
	Clips         int    `json:"clips"`
	TextClips     int    `json:"textClips"`
	ImageClips    int    `json:"imageClips"`
	Images        int    `json:"images"`
	Settings      int    `json:"settings"`
	SkippedImages int    `json:"skippedImages"`
	Cancelled     bool   `json:"cancelled"`
}

type backupManifest struct {
	Format     string `json:"format"`
	Version    int    `json:"version"`
	AppVersion string `json:"appVersion"`
	ExportedAt string `json:"exportedAt"`
}

type backupClip struct {
	Type        string `json:"type"`
	Content     string `json:"content,omitempty"`
	Preview     string `json:"preview"`
	Thumbnail   string `json:"thumbnail,omitempty"`
	Timestamp   int64  `json:"timestamp"`
	Pinned      bool   `json:"pinned"`
	ContentHash string `json:"contentHash,omitempty"`
	ImageFile   string `json:"imageFile,omitempty"`
	SourceApp   string `json:"sourceApp,omitempty"`
	SourceTitle string `json:"sourceTitle,omitempty"`
	SourcePath  string `json:"sourcePath,omitempty"`
	Note        string `json:"note,omitempty"`
	Tags        string `json:"tags,omitempty"`
}

type backupSetting struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (s *Store) ExportBackupFile(path, appVersion string) (BackupStats, error) {
	if path == "" {
		return BackupStats{Cancelled: true}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return BackupStats{}, err
	}

	clips, err := s.backupClips()
	if err != nil {
		return BackupStats{}, err
	}
	settings, err := s.backupSettings()
	if err != nil {
		return BackupStats{}, err
	}

	tmp := path + ".tmp"
	_ = os.Remove(tmp)
	file, err := os.Create(tmp)
	if err != nil {
		return BackupStats{}, err
	}
	zipWriter := zip.NewWriter(file)
	stats := BackupStats{Path: path, Settings: len(settings)}
	cleanup := true
	defer func() {
		_ = zipWriter.Close()
		_ = file.Close()
		if cleanup {
			_ = os.Remove(tmp)
		}
	}()

	manifest := backupManifest{
		Format:     backupFormat,
		Version:    backupVersion,
		AppVersion: appVersion,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := writeZipJSON(zipWriter, "manifest.json", manifest); err != nil {
		return BackupStats{}, err
	}
	if err := writeZipJSON(zipWriter, "settings.json", settings); err != nil {
		return BackupStats{}, err
	}

	imageNames := map[string]bool{}
	backupClips := make([]backupClip, 0, len(clips))
	for _, clip := range clips {
		item := backupClip{
			Type:        clip.Type,
			Preview:     clip.Preview,
			Thumbnail:   clip.Thumbnail,
			Timestamp:   clip.Timestamp,
			Pinned:      clip.Pinned,
			ContentHash: clip.ContentHash,
			SourceApp:   clip.SourceApp,
			SourceTitle: clip.SourceTitle,
			SourcePath:  clip.SourcePath,
			Note:        clip.Note,
			Tags:        clip.Tags,
		}
		stats.Clips++
		switch clip.Type {
		case "text":
			item.Content = clip.Content
			stats.TextClips++
		case "image":
			stats.ImageClips++
			imageName, err := writeBackupImage(zipWriter, clip.Content, clip.ID, imageNames)
			if err != nil {
				stats.SkippedImages++
			} else if imageName != "" {
				item.ImageFile = imageName
				stats.Images++
			}
		default:
			item.Content = clip.Content
		}
		backupClips = append(backupClips, item)
	}
	if err := writeZipJSON(zipWriter, "clips.json", backupClips); err != nil {
		return BackupStats{}, err
	}
	if err := zipWriter.Close(); err != nil {
		return BackupStats{}, err
	}
	if err := file.Close(); err != nil {
		return BackupStats{}, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return BackupStats{}, err
	}
	cleanup = false
	return stats, nil
}

func (s *Store) ImportBackupFile(path string) (BackupStats, error) {
	if path == "" {
		return BackupStats{Cancelled: true}, nil
	}
	reader, err := zip.OpenReader(path)
	if err != nil {
		return BackupStats{}, err
	}
	defer reader.Close()
	if len(reader.File) > maxBackupZipFiles {
		return BackupStats{}, fmt.Errorf("backup has too many files: %d", len(reader.File))
	}

	var manifest backupManifest
	if err := readZipJSON(&reader.Reader, "manifest.json", &manifest, maxBackupManifestBytes); err != nil {
		return BackupStats{}, err
	}
	if manifest.Format != backupFormat || manifest.Version != backupVersion {
		return BackupStats{}, fmt.Errorf("unsupported backup format")
	}

	var clips []backupClip
	if err := readZipJSON(&reader.Reader, "clips.json", &clips, maxBackupClipsBytes); err != nil {
		return BackupStats{}, err
	}
	if len(clips) > maxBackupClips {
		return BackupStats{}, fmt.Errorf("backup has too many clips: %d", len(clips))
	}
	var settings []backupSetting
	if err := readZipJSON(&reader.Reader, "settings.json", &settings, maxBackupSettingsBytes); err != nil {
		return BackupStats{}, err
	}
	if len(settings) > maxBackupSettings {
		return BackupStats{}, fmt.Errorf("backup has too many settings: %d", len(settings))
	}

	imageFiles := map[string]*zip.File{}
	for _, file := range reader.File {
		if strings.HasPrefix(file.Name, "images/") && !strings.HasSuffix(file.Name, "/") {
			name := strings.TrimPrefix(file.Name, "images/")
			if validBackupImageName(name) {
				imageFiles[name] = file
			}
		}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return BackupStats{}, err
	}
	stats := BackupStats{Path: path}
	createdImages := []string{}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
			for _, path := range createdImages {
				_ = os.Remove(path)
			}
		}
	}()

	for _, setting := range settings {
		if setting.Key == "" {
			continue
		}
		if _, err := tx.Exec(
			"INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
			setting.Key, setting.Value,
		); err != nil {
			return BackupStats{}, err
		}
		stats.Settings++
	}

	imageDir := filepath.Join(s.dataDir, "images")
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		return BackupStats{}, err
	}

	var restoredImageBytes int64
	for i, item := range clips {
		entry := clipboard.ClipEntry{
			Type:        item.Type,
			Content:     item.Content,
			Preview:     item.Preview,
			Thumbnail:   item.Thumbnail,
			Timestamp:   item.Timestamp,
			Pinned:      item.Pinned,
			SourceApp:   item.SourceApp,
			SourceTitle: item.SourceTitle,
			SourcePath:  item.SourcePath,
			Note:        item.Note,
			Tags:        item.Tags,
		}
		if entry.Type == "image" {
			entry.Content = ""
			if item.ImageFile == "" {
				stats.SkippedImages++
				continue
			}
			if !validBackupImageName(item.ImageFile) {
				stats.SkippedImages++
				continue
			}
			zipFile, ok := imageFiles[item.ImageFile]
			if !ok {
				stats.SkippedImages++
				continue
			}
			remaining := maxBackupTotalImageBytes - restoredImageBytes
			destPath, bytes, err := restoreBackupImage(zipFile, imageDir, i, remaining)
			if err != nil {
				stats.SkippedImages++
				continue
			}
			createdImages = append(createdImages, destPath)
			entry.Content = destPath
			restoredImageBytes += bytes
			stats.Images++
		}
		inserted, err := s.saveOrUpdateTx(tx, entry, item.ContentHash)
		if err != nil {
			return BackupStats{}, err
		}
		if inserted {
			stats.Clips++
			if entry.Type == "image" {
				stats.ImageClips++
			} else if entry.Type == "text" {
				stats.TextClips++
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return BackupStats{}, err
	}
	committed = true
	return stats, nil
}

type storedClip struct {
	clipboard.ClipEntry
	ContentHash string
}

func (s *Store) backupClips() ([]storedClip, error) {
	rows, err := s.db.Query(`SELECT ` + clipColumns + `, content_hash FROM clips ORDER BY pinned DESC, timestamp DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clips []storedClip
	for rows.Next() {
		var clip storedClip
		if err := rows.Scan(&clip.ID, &clip.Type, &clip.Content, &clip.Preview, &clip.Thumbnail, &clip.Timestamp, &clip.Pinned, &clip.SourceApp, &clip.SourceTitle, &clip.SourcePath, &clip.Note, &clip.Tags, &clip.ContentHash); err != nil {
			return nil, err
		}
		clips = append(clips, clip)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return clips, nil
}

func (s *Store) backupSettings() ([]backupSetting, error) {
	rows, err := s.db.Query(`SELECT key, value FROM settings ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var settings []backupSetting
	for rows.Next() {
		var setting backupSetting
		if err := rows.Scan(&setting.Key, &setting.Value); err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return settings, nil
}

func (s *Store) saveOrUpdateTx(tx *sql.Tx, entry clipboard.ClipEntry, contentHash string) (bool, error) {
	entry = normalizeClipMetadata(entry)
	if contentHash == "" {
		h, err := s.hashEntryContent(entry)
		if err != nil {
			return false, err
		}
		contentHash = h
	}

	var existingID int64
	var existingThumbnail string
	var existingNote string
	var existingTags string
	err := tx.QueryRow("SELECT id, thumbnail, note, tags FROM clips WHERE content_hash = ?", contentHash).Scan(&existingID, &existingThumbnail, &existingNote, &existingTags)
	if err == sql.ErrNoRows {
		_, err := tx.Exec(
			`INSERT INTO clips (type, content, preview, thumbnail, timestamp, pinned, content_hash, source_app, source_title, source_path, note, tags) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			entry.Type, entry.Content, entry.Preview, entry.Thumbnail, entry.Timestamp, entry.Pinned, contentHash, entry.SourceApp, entry.SourceTitle, entry.SourcePath, entry.Note, entry.Tags,
		)
		return true, err
	}
	if err != nil {
		return false, err
	}

	thumbnail := existingThumbnail
	if thumbnail == "" && entry.Thumbnail != "" {
		thumbnail = entry.Thumbnail
	}
	hasSource := entry.SourceApp != "" || entry.SourceTitle != "" || entry.SourcePath != ""
	sourceFlag := 0
	if hasSource {
		sourceFlag = 1
	}
	note := existingNote
	if note == "" && entry.Note != "" {
		note = entry.Note
	}
	tags := mergeTags(existingTags, entry.Tags)
	_, err = tx.Exec(
		`UPDATE clips
		 SET timestamp = CASE WHEN timestamp < ? THEN ? ELSE timestamp END,
		     thumbnail = ?,
		     pinned = CASE WHEN pinned = 1 OR ? THEN 1 ELSE 0 END,
		     source_app = CASE WHEN ? THEN ? ELSE source_app END,
		     source_title = CASE WHEN ? THEN ? ELSE source_title END,
		     source_path = CASE WHEN ? THEN ? ELSE source_path END,
		     note = ?,
		     tags = ?
		 WHERE id = ?`,
		entry.Timestamp, entry.Timestamp, thumbnail, entry.Pinned,
		sourceFlag, entry.SourceApp,
		sourceFlag, entry.SourceTitle,
		sourceFlag, entry.SourcePath,
		note,
		tags,
		existingID,
	)
	if entry.Type == "image" && entry.Content != "" {
		s.removeImageFile(entry.Content)
	}
	return false, err
}

func writeZipJSON(zipWriter *zip.Writer, name string, value interface{}) error {
	writer, err := zipWriter.Create(name)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func readZipJSON(reader *zip.Reader, name string, value interface{}, maxBytes int64) error {
	file, err := findZipFile(reader, name)
	if err != nil {
		return err
	}
	if maxBytes > 0 && file.UncompressedSize64 > uint64(maxBytes) {
		return fmt.Errorf("backup %s is too large", name)
	}
	handle, err := file.Open()
	if err != nil {
		return err
	}
	defer handle.Close()
	var readerForJSON io.Reader = handle
	if maxBytes > 0 {
		readerForJSON = io.LimitReader(handle, maxBytes+1)
	}
	data, err := io.ReadAll(readerForJSON)
	if err != nil {
		return err
	}
	if maxBytes > 0 && int64(len(data)) > maxBytes {
		return fmt.Errorf("backup %s is too large", name)
	}
	return json.NewDecoder(bytes.NewReader(data)).Decode(value)
}

func findZipFile(reader *zip.Reader, name string) (*zip.File, error) {
	for _, file := range reader.File {
		if file.Name == name {
			return file, nil
		}
	}
	return nil, fmt.Errorf("backup missing %s", name)
}

func writeBackupImage(zipWriter *zip.Writer, sourcePath string, clipID int64, used map[string]bool) (string, error) {
	if sourcePath == "" {
		return "", os.ErrNotExist
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return "", err
	}
	defer source.Close()

	name := safeBackupImageName(fmt.Sprintf("%d-%s", clipID, filepath.Base(sourcePath)))
	for used[name] {
		name = fmt.Sprintf("%d-%d-%s", clipID, len(used), filepath.Base(sourcePath))
		name = safeBackupImageName(name)
	}
	used[name] = true

	writer, err := zipWriter.Create("images/" + name)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(writer, source); err != nil {
		return "", err
	}
	return name, nil
}

func restoreBackupImage(file *zip.File, imageDir string, index int, remainingBytes int64) (string, int64, error) {
	if file == nil {
		return "", 0, os.ErrNotExist
	}
	if remainingBytes <= 0 {
		return "", 0, fmt.Errorf("backup image storage limit exceeded")
	}
	limit := int64(maxBackupImageBytes)
	if remainingBytes < limit {
		limit = remainingBytes
	}
	if file.UncompressedSize64 > uint64(limit) {
		return "", 0, fmt.Errorf("backup image is too large")
	}
	base := safeBackupImageName(filepath.Base(file.Name))
	destPath := filepath.Join(imageDir, fmt.Sprintf("import_%d_%03d_%s", time.Now().UnixNano(), index, base))
	dest, err := os.Create(destPath)
	if err != nil {
		return "", 0, err
	}
	defer dest.Close()

	source, err := file.Open()
	if err != nil {
		return "", 0, err
	}
	defer source.Close()

	written, err := copyWithLimit(dest, source, limit)
	if err != nil {
		_ = os.Remove(destPath)
		return "", 0, err
	}
	return destPath, written, nil
}

func copyWithLimit(dst io.Writer, src io.Reader, maxBytes int64) (int64, error) {
	if maxBytes < 0 {
		return 0, fmt.Errorf("invalid copy limit")
	}
	limited := &io.LimitedReader{R: src, N: maxBytes + 1}
	written, err := io.Copy(dst, limited)
	if err != nil {
		return written, err
	}
	if written > maxBytes {
		return written, fmt.Errorf("backup image exceeds size limit")
	}
	return written, nil
}

func validBackupImageName(name string) bool {
	if name == "" || strings.ContainsAny(name, `/\`) {
		return false
	}
	return name == safeBackupImageName(name)
}

func safeBackupImageName(name string) string {
	name = filepath.Base(name)
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '.', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, name)
	if name == "" || name == "." {
		return "image.dib"
	}
	return name
}
