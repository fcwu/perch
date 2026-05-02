package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// ErrUploadQuotaExceeded is returned by WriteAttachmentsToDisk when the new
// payload would push <workdir>/uploads/<convID>/ over the per-conversation
// quota. Wrapped with %w so callers can use errors.Is for HTTP 400 mapping.
var ErrUploadQuotaExceeded = errors.New("conversation upload quota exceeded")

// PersistedAttachment describes one non-image attachment that has been written
// under <workdir>/uploads/<convID>/. RelPath is the agent-facing relative path
// (e.g. "./uploads/c-abc123/error.log").
type PersistedAttachment struct {
	Filename  string
	RelPath   string
	MimeType  string
	SizeBytes int64
	AbsPath   string // absolute on-disk path; for cleanup on error
}

// IsImageMIME reports whether mime is one of the inline-ACP image MIMEs.
// Image attachments stay in memory (base64 → ACP image block); everything
// else takes the disk-save path.
func IsImageMIME(mime string) bool {
	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	}
	return false
}

const (
	maxAttachmentFilenameLen = 200
)

// SanitizeAttachmentFilename produces a filesystem-safe variant of the client-
// supplied filename. It preserves the extension, strips path separators / NUL /
// `..` segments, allows ASCII alphanumerics + common punctuation + Unicode
// letters & digits (so CJK names survive), and caps the total length.
//
// Returns an error when the result would be empty (e.g. ".." or ""), since
// such names cannot be safely written.
func SanitizeAttachmentFilename(name string) (string, error) {
	// First: drop any path components the client may have included.
	// filepath.Base on the raw input handles both / and the embedded \
	// (after explicit normalisation).
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	if name == "." || name == ".." || name == "/" || name == "" {
		return "", fmt.Errorf("invalid filename")
	}

	// Split off extension to preserve it after length capping.
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)

	clean := func(s string) string {
		var b strings.Builder
		b.Grow(len(s))
		for _, r := range s {
			switch {
			case r == 0:
				// drop NUL
			case r < 0x20 || r == 0x7F:
				// drop control chars
			case r == '/' || r == '\\':
				// drop path separators
			case unicode.IsLetter(r) || unicode.IsDigit(r):
				b.WriteRune(r)
			case r == ' ' || r == '.' || r == '-' || r == '_' || r == '(' || r == ')' || r == '[' || r == ']':
				b.WriteRune(r)
			default:
				// replace anything else with underscore so users can still
				// recognise the original name structure
				b.WriteRune('_')
			}
		}
		return b.String()
	}

	stem = clean(stem)
	ext = clean(ext)

	// Reject ".." style stems after cleaning.
	if stem == "" || strings.Trim(stem, ".") == "" {
		return "", fmt.Errorf("invalid filename after sanitization")
	}

	// Cap length, preserving extension where possible.
	if len(stem)+len(ext) > maxAttachmentFilenameLen {
		room := maxAttachmentFilenameLen - len(ext)
		if room < 1 {
			// Pathological extension length — drop it and just use first 200 of stem.
			return stem[:maxAttachmentFilenameLen], nil
		}
		stem = stem[:room]
	}
	return stem + ext, nil
}

// ResolveAttachmentPath builds the absolute path for a sanitized filename
// under <workdir>/uploads/<convID>/, then verifies via filepath.Clean +
// prefix-check that the result lies inside that directory. Returns the
// (uploadsRoot, absolutePath) pair on success.
func ResolveAttachmentPath(workdir, convID, sanitizedName string) (string, string, error) {
	if workdir == "" {
		return "", "", fmt.Errorf("workdir is empty")
	}
	if convID == "" {
		return "", "", fmt.Errorf("convID is empty")
	}
	if strings.ContainsAny(convID, "/\\") || strings.Contains(convID, "..") {
		return "", "", fmt.Errorf("invalid convID")
	}
	uploadsBase := filepath.Join(workdir, "uploads")
	convDir := filepath.Join(uploadsBase, convID)
	full := filepath.Clean(filepath.Join(convDir, sanitizedName))

	// Defence-in-depth: ensure full still has convDir as a prefix even if
	// sanitizedName slipped through (it shouldn't after SanitizeAttachmentFilename).
	rel, err := filepath.Rel(convDir, full)
	if err != nil || strings.HasPrefix(rel, "..") || strings.ContainsAny(rel, string(filepath.Separator)+"\\") {
		return "", "", fmt.Errorf("path traversal blocked: %s", sanitizedName)
	}
	return convDir, full, nil
}

// NextAvailableFilename returns sanitizedName when no file with that name
// exists in dir; otherwise it appends " (2)", " (3)", … before the extension
// until a free name is found. Bounded loop to avoid pathological behaviour.
func NextAvailableFilename(dir, sanitizedName string) string {
	if _, err := os.Stat(filepath.Join(dir, sanitizedName)); os.IsNotExist(err) {
		return sanitizedName
	}
	ext := filepath.Ext(sanitizedName)
	stem := strings.TrimSuffix(sanitizedName, ext)
	for i := 2; i < 1000; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", stem, i, ext)
		if _, err := os.Stat(filepath.Join(dir, candidate)); os.IsNotExist(err) {
			return candidate
		}
	}
	// Fallback: timestamp suffix; extremely unlikely to be hit.
	return fmt.Sprintf("%s.%d%s", stem, time.Now().UnixNano(), ext)
}

// dirSizeBytes returns the total size of regular files under dir (recursive).
// Returns 0 if dir does not exist.
func dirSizeBytes(dir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		fi, ferr := d.Info()
		if ferr != nil {
			return ferr
		}
		if fi.Mode().IsRegular() {
			total += fi.Size()
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	return total, nil
}

// WriteAttachmentsToDisk persists every non-image attachment under
// <workdir>/uploads/<convID>/. Image MIMEs are skipped (they take the inline
// ACP image-block path). On any error the function rolls back any files it
// has written for this batch (best-effort), so the on-disk view stays
// consistent with the request being rejected.
//
// Pre-conditions: atts must already have passed ValidateAttachments — in
// particular, DataBase64 is the raw base64 payload (data: prefix stripped).
func WriteAttachmentsToDisk(ctx context.Context, workdir, convID string, atts []Attachment, lim AttachmentLimits) ([]PersistedAttachment, error) {
	if len(atts) == 0 {
		return nil, nil
	}
	// Filter to non-image first; if none, we're done without touching disk.
	var pending []Attachment
	for _, a := range atts {
		if !IsImageMIME(a.MimeType) {
			pending = append(pending, a)
		}
	}
	if len(pending) == 0 {
		return nil, nil
	}

	convDir, _, err := ResolveAttachmentPath(workdir, convID, "noop")
	if err != nil {
		return nil, fmt.Errorf("resolve uploads dir: %w", err)
	}

	// Pre-compute sizes + check quota before mkdir / writes.
	var newSizes []int64
	var newTotal int64
	for _, a := range pending {
		decoded, err := base64.StdEncoding.DecodeString(a.DataBase64)
		if err != nil {
			return nil, fmt.Errorf("attachment %s: invalid base64 (post-validation): %w", a.Filename, err)
		}
		newSizes = append(newSizes, int64(len(decoded)))
		newTotal += int64(len(decoded))
	}
	if lim.DirQuotaBytes > 0 {
		existing, err := dirSizeBytes(convDir)
		if err != nil {
			return nil, fmt.Errorf("stat uploads dir: %w", err)
		}
		if existing+newTotal > lim.DirQuotaBytes {
			return nil, fmt.Errorf("%w: existing %d + new %d > %d", ErrUploadQuotaExceeded, existing, newTotal, lim.DirQuotaBytes)
		}
	}

	if err := os.MkdirAll(convDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir uploads dir: %w", err)
	}

	persisted := make([]PersistedAttachment, 0, len(pending))
	rollback := func() {
		for _, p := range persisted {
			_ = os.Remove(p.AbsPath)
		}
	}

	for i, a := range pending {
		select {
		case <-ctx.Done():
			rollback()
			return nil, ctx.Err()
		default:
		}
		safeName, err := SanitizeAttachmentFilename(a.Filename)
		if err != nil {
			rollback()
			return nil, fmt.Errorf("attachment %s: %w", a.Filename, err)
		}
		safeName = NextAvailableFilename(convDir, safeName)
		_, abs, err := ResolveAttachmentPath(workdir, convID, safeName)
		if err != nil {
			rollback()
			return nil, fmt.Errorf("attachment %s: %w", a.Filename, err)
		}

		decoded, err := base64.StdEncoding.DecodeString(a.DataBase64)
		if err != nil {
			rollback()
			return nil, fmt.Errorf("attachment %s: invalid base64: %w", a.Filename, err)
		}

		// temp file + rename for atomic write
		tmp, err := os.CreateTemp(convDir, ".upload-*.tmp")
		if err != nil {
			rollback()
			return nil, fmt.Errorf("attachment %s: create tmp: %w", a.Filename, err)
		}
		tmpPath := tmp.Name()
		if _, err := tmp.Write(decoded); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
			rollback()
			return nil, fmt.Errorf("attachment %s: write: %w", a.Filename, err)
		}
		if err := tmp.Close(); err != nil {
			_ = os.Remove(tmpPath)
			rollback()
			return nil, fmt.Errorf("attachment %s: close tmp: %w", a.Filename, err)
		}
		if err := os.Rename(tmpPath, abs); err != nil {
			_ = os.Remove(tmpPath)
			rollback()
			return nil, fmt.Errorf("attachment %s: rename: %w", a.Filename, err)
		}

		persisted = append(persisted, PersistedAttachment{
			Filename:  safeName,
			RelPath:   "./" + filepath.ToSlash(filepath.Join("uploads", convID, safeName)),
			MimeType:  a.MimeType,
			SizeBytes: newSizes[i],
			AbsPath:   abs,
		})
	}
	return persisted, nil
}

// BuildPromptFilePrefix prepends one "[file: <relpath> (<mime>, <size>)]" line
// per persisted attachment to userText. With multiple files, all prefix lines
// come first, then a blank line, then the original text. With files but empty
// text, the prefix lines are returned alone (no trailing blank line).
//
// When persisted is empty, userText is returned unchanged.
func BuildPromptFilePrefix(persisted []PersistedAttachment, userText string) string {
	if len(persisted) == 0 {
		return userText
	}
	var b strings.Builder
	for _, p := range persisted {
		fmt.Fprintf(&b, "[file: %s (%s, %s)]\n", p.RelPath, p.MimeType, humanSize(p.SizeBytes))
	}
	trimmed := strings.TrimSpace(userText)
	if trimmed == "" {
		// Strip the trailing newline so the result is exactly the prefix lines.
		return strings.TrimRight(b.String(), "\n")
	}
	b.WriteString("\n")
	b.WriteString(userText)
	return b.String()
}

// cleanupOrphanUploads scans <workdir>/uploads/ for per-conversation
// directories whose newest file mtime is older than ttl, and removes them.
// Returns (kept, removed, err). Missing uploads root is a no-op.
func cleanupOrphanUploads(workdir string, ttl time.Duration) (int, int, error) {
	if workdir == "" || ttl <= 0 {
		return 0, 0, nil
	}
	root := filepath.Join(workdir, "uploads")
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
			// On error inspecting the dir, keep it (fail open — never destructive).
			kept++
			continue
		}
		if newest.Before(cutoff) {
			if err := os.RemoveAll(convDir); err == nil {
				removed++
				continue
			}
			// Removal failed — count as kept and continue.
			kept++
			continue
		}
		kept++
	}
	return kept, removed, nil
}

// newestMtimeInDir walks dir and returns the latest mtime of any file
// inside (recursively). For an empty dir, returns the dir's own mtime.
func newestMtimeInDir(dir string) (time.Time, error) {
	var newest time.Time
	if fi, err := os.Stat(dir); err == nil {
		newest = fi.ModTime()
	}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		fi, ferr := d.Info()
		if ferr != nil {
			return nil
		}
		if fi.ModTime().After(newest) {
			newest = fi.ModTime()
		}
		return nil
	})
	return newest, err
}

// humanSize formats n as B / KiB / MiB / GiB with one decimal place above KiB.
func humanSize(n int64) string {
	const (
		KiB = 1024
		MiB = 1024 * KiB
		GiB = 1024 * MiB
	)
	switch {
	case n < KiB:
		return fmt.Sprintf("%d B", n)
	case n < MiB:
		return fmt.Sprintf("%.1f KiB", float64(n)/float64(KiB))
	case n < GiB:
		return fmt.Sprintf("%.1f MiB", float64(n)/float64(MiB))
	default:
		return fmt.Sprintf("%.1f GiB", float64(n)/float64(GiB))
	}
}
