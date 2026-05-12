package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const maxImageBytes = 8 * 1024 * 1024 // 8 MiB — Discord limit

// ImageAttachment describes one agent-produced image exposed to clients.
type ImageAttachment struct {
	URL     string `json:"url"`
	Caption string `json:"caption"`
}

// imageStore wraps the <workdir>/images/ directory.
type imageStore struct {
	workdir string
	logger  *slog.Logger
}

func newImageStore(workdir string, logger *slog.Logger) *imageStore {
	if logger == nil {
		logger = slog.Default()
	}
	return &imageStore{workdir: workdir, logger: logger}
}

// imagesRoot returns <workdir>/images/.
func (s *imageStore) imagesRoot() string {
	return filepath.Join(s.workdir, "images")
}

// convDir returns <workdir>/images/<convID>/.
func (s *imageStore) convDir(convID string) string {
	return filepath.Join(s.imagesRoot(), convID)
}

// Store writes r into <workdir>/images/<convID>/<uuid>.<ext> and returns the
// URL path "/api/images/<convID>/<uuid>.<ext>". ext is derived from filename.
func (s *imageStore) Store(convID, filename string, r io.Reader) (string, error) {
	dir := s.convDir(convID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("imagestore: mkdir: %w", err)
	}
	ext := filepath.Ext(filename)
	if ext == "" {
		ext = ".bin"
	}
	id := newUUID()
	name := id + ext
	dst := filepath.Join(dir, name)
	f, err := os.Create(dst)
	if err != nil {
		return "", fmt.Errorf("imagestore: create: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		_ = os.Remove(dst)
		return "", fmt.Errorf("imagestore: write: %w", err)
	}
	return "/api/images/" + convID + "/" + name, nil
}

// Cleanup removes <workdir>/images/<convID>/ and all its contents.
func (s *imageStore) Cleanup(convID string) {
	if convID == "" || convID == "default" {
		return
	}
	dir := s.convDir(convID)
	if err := os.RemoveAll(dir); err != nil {
		s.logger.Warn("imagestore: cleanup failed", "convID", convID, "err", err)
	} else {
		s.logger.Info("imagestore: cleaned on evict", "convID", convID)
	}
}

// cleanupOrphanImages scans <workdir>/images/ and removes dirs whose newest
// mtime is older than ttl. Mirrors cleanupOrphanUploads.
func cleanupOrphanImages(workdir string, ttl time.Duration) (int, int, error) {
	if workdir == "" || ttl <= 0 {
		return 0, 0, nil
	}
	root := filepath.Join(workdir, "images")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	cutoff := time.Now().Add(-ttl)
	var kept, removed int
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		convDir := filepath.Join(root, e.Name())
		newest, err := newestMtimeInDir(convDir)
		if err != nil {
			kept++
			continue
		}
		if newest.Before(cutoff) {
			if err := os.RemoveAll(convDir); err == nil {
				removed++
				continue
			}
		}
		kept++
	}
	return kept, removed, nil
}

// safeImagePath verifies that absPath is inside /tmp or workdir to prevent
// path traversal. Returns an error if the path escapes both roots.
func safeImagePath(absPath, workdir string) error {
	clean := filepath.Clean(absPath)
	tmpOK := strings.HasPrefix(clean, filepath.Clean("/tmp")+string(filepath.Separator)) ||
		clean == filepath.Clean("/tmp")
	wdOK := workdir != "" && (strings.HasPrefix(clean, filepath.Clean(workdir)+string(filepath.Separator)) ||
		clean == filepath.Clean(workdir))
	if !tmpOK && !wdOK {
		return fmt.Errorf("imagestore: path %q is outside allowed roots", absPath)
	}
	return nil
}

// imageTokenRe matches [image: <path-or-data-uri>] tokens in agent response text.
var imageTokenRe = regexp.MustCompile(`\[image:\s*([^\]]+)\]`)

// extractImages scans text for [image: ...] tokens, stores each image via
// store, and returns cleaned text (tokens removed/replaced) plus attachments.
func extractImages(text string, store *imageStore, workdir, convID string) (string, []ImageAttachment) {
	if store == nil || !strings.Contains(text, "[image:") {
		return text, nil
	}

	var attachments []ImageAttachment
	result := imageTokenRe.ReplaceAllStringFunc(text, func(match string) string {
		sub := imageTokenRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return ""
		}
		ref := strings.TrimSpace(sub[1])

		// data URI case
		if strings.HasPrefix(ref, "data:") {
			url, caption, err := storeDataURI(ref, store, convID)
			if err != nil {
				store.logger.Warn("imagestore: data URI extract failed", "err", err)
				return "(圖片過大，無法顯示)" // size guard hit
			}
			attachments = append(attachments, ImageAttachment{URL: url, Caption: caption})
			return ""
		}

		// File path case
		absPath := ref
		if !filepath.IsAbs(ref) {
			absPath = filepath.Join(workdir, ref)
		}
		absPath = filepath.Clean(absPath)
		if err := safeImagePath(absPath, workdir); err != nil {
			store.logger.Warn("imagestore: path traversal rejected", "path", ref, "err", err)
			return ""
		}
		fi, err := os.Stat(absPath)
		if err != nil {
			store.logger.Warn("imagestore: file not found", "path", absPath)
			return ""
		}
		if fi.Size() > maxImageBytes {
			store.logger.Warn("imagestore: file too large", "path", absPath, "size", fi.Size())
			return "(圖片過大，無法顯示)"
		}

		f, err := os.Open(absPath)
		if err != nil {
			store.logger.Warn("imagestore: open failed", "path", absPath, "err", err)
			return ""
		}
		defer f.Close()

		filename := filepath.Base(absPath)
		url, err := store.Store(convID, filename, f)
		if err != nil {
			store.logger.Warn("imagestore: store failed", "path", absPath, "err", err)
			return ""
		}
		attachments = append(attachments, ImageAttachment{URL: url, Caption: filename})
		return ""
	})

	return result, attachments
}

// storeDataURI decodes a data:image/*;base64,... URI, checks size, and stores it.
func storeDataURI(ref string, store *imageStore, convID string) (url, caption string, err error) {
	// data:<mime>;base64,<data>
	comma := strings.Index(ref, ",")
	if comma < 0 {
		return "", "", fmt.Errorf("malformed data URI")
	}
	header := ref[5:comma] // strip "data:"
	encoded := ref[comma+1:]

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(encoded)
		if err != nil {
			return "", "", fmt.Errorf("base64 decode: %w", err)
		}
	}
	if int64(len(decoded)) > maxImageBytes {
		return "", "", fmt.Errorf("image too large: %d bytes", len(decoded))
	}

	// Derive extension from MIME.
	mimeType := strings.TrimSuffix(header, ";base64")
	ext := ".bin"
	if exts, _ := mime.ExtensionsByType(mimeType); len(exts) > 0 {
		// prefer common extensions
		for _, e := range exts {
			if e == ".png" || e == ".jpg" || e == ".jpeg" || e == ".gif" || e == ".webp" {
				ext = e
				break
			}
			ext = e
		}
	}

	filename := "image" + ext
	url, err = store.Store(convID, filename, bytes.NewReader(decoded))
	if err != nil {
		return "", "", err
	}
	return url, filename, nil
}

// walkImages walks <workdir>/images/<convID>/ and returns each stored file's
// absolute path. Used by Discord to open and attach files.
func (s *imageStore) AbsPath(convID, filename string) string {
	return filepath.Join(s.convDir(convID), filename)
}

// cleanupOrphanImagesDir is a convenience wrapper that mirrors the uploads
// startup sweep in main.go. Called from main during startup.
func cleanupOrphanImagesDir(workdir string, ttl time.Duration, logger *slog.Logger) {
	kept, removed, err := cleanupOrphanImages(workdir, ttl)
	if err != nil {
		logger.Warn("images orphan cleanup failed", "err", err)
	} else if removed > 0 || kept > 0 {
		logger.Info("images orphan cleanup", "kept", kept, "removed", removed)
	}
}

// collectPlaywrightScreenshots scans /tmp/playwright-output for image files
// created after since, stores them via store, and returns ImageAttachments.
// skip lists filenames (captions) already stored by extractImages so the same
// file is not double-stored when the agent emitted a partial [image:] token.
func collectPlaywrightScreenshots(since time.Time, store *imageStore, convID string, logger *slog.Logger, skip map[string]bool) []ImageAttachment {
	const playwrightOutputDir = "/tmp/playwright-output"
	entries, err := os.ReadDir(playwrightOutputDir)
	if err != nil {
		return nil
	}
	var attachments []ImageAttachment
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if skip[name] {
			continue
		}
		switch strings.ToLower(filepath.Ext(name)) {
		case ".png", ".jpg", ".jpeg", ".webp":
		default:
			continue
		}
		info, err := e.Info()
		if err != nil || !info.ModTime().After(since) {
			continue
		}
		absPath := filepath.Join(playwrightOutputDir, name)
		if info.Size() > maxImageBytes {
			logger.Warn("playwright auto-pickup: file too large", "path", absPath, "size", info.Size())
			continue
		}
		f, err := os.Open(absPath)
		if err != nil {
			logger.Warn("playwright auto-pickup: open failed", "path", absPath, "err", err)
			continue
		}
		url, storeErr := store.Store(convID, name, f)
		f.Close()
		if storeErr != nil {
			logger.Warn("playwright auto-pickup: store failed", "path", absPath, "err", storeErr)
			continue
		}
		logger.Info("playwright auto-pickup: stored screenshot", "path", absPath, "url", url)
		attachments = append(attachments, ImageAttachment{URL: url, Caption: name})
	}
	return attachments
}

// fileExt safely extracts a URL's filename extension for Content-Type sniffing.
func imageFileExt(urlPath string) string {
	return filepath.Ext(filepath.Base(urlPath))
}

// extToMime returns a safe Content-Type for a given extension.
func extToMime(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}

// inlineAsDataURI reads the stored image and returns a copy of the attachment
// with URL replaced by a base64 data URI. Falls back to original if reading
// fails or the MIME type is unknown.
func (s *imageStore) inlineAsDataURI(a ImageAttachment) ImageAttachment {
	const prefix = "/api/images/"
	if !strings.HasPrefix(a.URL, prefix) {
		return a
	}
	rest := a.URL[len(prefix):]
	idx := strings.Index(rest, "/")
	if idx < 0 {
		return a
	}
	convID, filename := rest[:idx], rest[idx+1:]
	absPath := s.AbsPath(convID, filename)
	data, err := os.ReadFile(absPath)
	if err != nil || int64(len(data)) > maxImageBytes {
		return a
	}
	mimeType := extToMime(imageFileExt(filename))
	if mimeType == "application/octet-stream" {
		return a
	}
	result := a
	result.URL = "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
	return result
}

// InlineAttachmentsAsDataURIs returns a copy of attachments with each URL
// replaced by an inline base64 data URI, eliminating the need for a separate
// authenticated HTTP request to serve the image.
func (s *imageStore) InlineAttachmentsAsDataURIs(attachments []ImageAttachment) []ImageAttachment {
	if len(attachments) == 0 {
		return attachments
	}
	result := make([]ImageAttachment, len(attachments))
	for i, a := range attachments {
		result[i] = s.inlineAsDataURI(a)
	}
	return result
}

// walkImageFiles returns all file entries under convDir (non-recursive).
func (s *imageStore) listFiles(convID string) ([]fs.DirEntry, error) {
	dir := s.convDir(convID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var files []fs.DirEntry
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e)
		}
	}
	return files, nil
}
