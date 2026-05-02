package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Default limits applied when no env var or settings override is present.
const (
	defaultUploadMaxBytes        = 10 * 1024 * 1024  // 10 MiB
	defaultUploadMaxFiles        = 4
	defaultUploadDirQuotaBytes   = 500 * 1024 * 1024 // 500 MiB per conversation
	defaultUploadOrphanTTLDays   = 7
)

var defaultUploadAllowedMime = []string{
	// images: inline ACP image content blocks
	"image/png", "image/jpeg", "image/gif", "image/webp",
	// text-class files: persisted to disk, agent reads with its tools
	"text/plain", "text/markdown", "text/csv", "text/x-log",
	"application/json", "application/x-ndjson",
	// PDF: persisted to disk, agent reads with pdftotext / Read
	"application/pdf",
}

// Attachment is a single inbound file the client wants to forward to the agent.
// The wire format on /api/chat is {filename, mime_type, data_base64}.
type Attachment struct {
	Filename   string `json:"filename"`
	MimeType   string `json:"mime_type"`
	DataBase64 string `json:"data_base64"`
}

// AttachmentLimits is the validated subset of effective Chat settings used by
// ValidateAttachments. Build it with effectiveAttachmentLimits.
type AttachmentLimits struct {
	MaxBytes        int64
	MaxFiles        int
	AllowedMime     []string
	DirQuotaBytes   int64 // per-conversation uploads/<conv-id>/ total cap
	OrphanTTLDays   int   // startup-scan: dirs older than this are removed
}

// ValidateAttachments runs server-side checks on every attachment. Errors are
// fatal for the whole request — partial success is not allowed because that
// would mean the client cannot tell what the agent actually saw.
//
// Validation order: count, then per-attachment MIME / decode / size / magic.
// On success, each Attachment in atts is mutated so DataBase64 contains only
// the raw base64 payload (any "data:image/png;base64," prefix is stripped).
func ValidateAttachments(atts []Attachment, lim AttachmentLimits) error {
	if len(atts) == 0 {
		return nil
	}
	if lim.MaxFiles > 0 && len(atts) > lim.MaxFiles {
		return fmt.Errorf("too many attachments: %d > %d", len(atts), lim.MaxFiles)
	}
	for i := range atts {
		att := &atts[i]
		if att.MimeType == "" {
			return fmt.Errorf("attachment %d: missing mime_type", i)
		}
		if !mimeAllowed(att.MimeType, lim.AllowedMime) {
			return fmt.Errorf("attachment %d (%s): mime_type %q not in allow-list", i, att.Filename, att.MimeType)
		}
		// Strip any "data:<mime>;base64," prefix the client may have left in.
		raw := att.DataBase64
		if i := strings.Index(raw, ","); i >= 0 && strings.HasPrefix(raw, "data:") {
			raw = raw[i+1:]
		}
		decoded, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return fmt.Errorf("attachment %d (%s): invalid base64: %v", i, att.Filename, err)
		}
		if lim.MaxBytes > 0 && int64(len(decoded)) > lim.MaxBytes {
			return fmt.Errorf("attachment %d (%s): %d bytes > %d limit", i, att.Filename, len(decoded), lim.MaxBytes)
		}
		if textMimeSet[att.MimeType] {
			// No magic bytes for text-class MIMEs — validate via heuristic
			// (UTF-8 valid + control-char ratio < 1%) and accept the client's claim.
			if !looksLikeText(decoded) {
				return fmt.Errorf("attachment %d (%s): claimed %q but content is not valid text (binary or invalid UTF-8)", i, att.Filename, att.MimeType)
			}
		} else {
			got := MagicMime(decoded)
			if got == "" {
				return fmt.Errorf("attachment %d (%s): unrecognised file format (magic bytes don't match any allowed type)", i, att.Filename)
			}
			if got != att.MimeType {
				return fmt.Errorf("attachment %d (%s): claimed %q but magic bytes say %q", i, att.Filename, att.MimeType, got)
			}
		}
		att.DataBase64 = raw
	}
	return nil
}

// MagicMime inspects the leading bytes of decoded payload and returns the
// matching MIME for supported binary formats with reliable magic bytes.
// Returns "" for text-class MIMEs (which have no shared magic bytes); use
// looksLikeText to validate those, then trust the client-claimed MIME.
func MagicMime(decoded []byte) string {
	switch {
	case len(decoded) >= 8 && bytes.HasPrefix(decoded, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}):
		return "image/png"
	case len(decoded) >= 3 && bytes.HasPrefix(decoded, []byte{0xFF, 0xD8, 0xFF}):
		return "image/jpeg"
	case len(decoded) >= 6 && (bytes.HasPrefix(decoded, []byte("GIF87a")) || bytes.HasPrefix(decoded, []byte("GIF89a"))):
		return "image/gif"
	case len(decoded) >= 12 && bytes.Equal(decoded[0:4], []byte("RIFF")) && bytes.Equal(decoded[8:12], []byte("WEBP")):
		return "image/webp"
	case len(decoded) >= 5 && bytes.HasPrefix(decoded, []byte("%PDF-")):
		return "application/pdf"
	}
	return ""
}

// textMimeSet is the set of MIMEs validated via looksLikeText (no magic bytes).
var textMimeSet = map[string]bool{
	"text/plain":             true,
	"text/markdown":          true,
	"text/csv":               true,
	"text/x-log":             true,
	"application/json":       true,
	"application/x-ndjson":   true,
}

// looksLikeText returns true when the leading bytes look like UTF-8 text:
// valid UTF-8 with control-char ratio (excluding \t \n \r) below 1% on the
// first 8 KiB. This is intentionally permissive — the goal is to keep
// binary-masquerading-as-text out, not to distinguish CSV from JSON.
func looksLikeText(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	probe := b
	if len(probe) > 8*1024 {
		probe = probe[:8*1024]
	}
	if !utf8.Valid(probe) {
		return false
	}
	var ctrl int
	for _, r := range string(probe) {
		if r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		// Reject NUL outright (no legitimate text file has NUL).
		if r == 0 {
			return false
		}
		if r < 0x20 || r == 0x7F {
			ctrl++
		}
	}
	// Compare against rune count of the probe (more accurate for non-ASCII).
	runeCount := utf8.RuneCountInString(string(probe))
	if runeCount == 0 {
		return false
	}
	return ctrl*100 < runeCount // strictly < 1%
}

// mimeAllowed returns true when mime is in allow (exact match, case-sensitive
// per RFC 2046; allow values are normalised to lower case).
func mimeAllowed(mime string, allow []string) bool {
	for _, a := range allow {
		if mime == a {
			return true
		}
	}
	return false
}

// AttachmentsToACPBlocks converts validated attachments to ACP image content
// blocks. Caller is responsible for prepending a text block when present.
func AttachmentsToACPBlocks(atts []Attachment) []ACPContent {
	blocks := make([]ACPContent, 0, len(atts))
	for _, a := range atts {
		blocks = append(blocks, ACPContent{
			Type:     "image",
			Data:     a.DataBase64,
			MimeType: a.MimeType,
		})
	}
	return blocks
}

// errEmptyPrompt is returned by callers when neither text nor attachments is
// present after validation — surfaced as 400 Bad Request.
var errEmptyPrompt = errors.New("prompt is empty (no text and no attachments)")

// EffectiveAttachmentLimits picks the active limits from settings, falling back
// to the package defaults when fields are unset.
func EffectiveAttachmentLimits(cs *ChatSettings) AttachmentLimits {
	lim := AttachmentLimits{
		MaxBytes:      int64(defaultUploadMaxBytes),
		MaxFiles:      defaultUploadMaxFiles,
		AllowedMime:   append([]string{}, defaultUploadAllowedMime...),
		DirQuotaBytes: int64(defaultUploadDirQuotaBytes),
		OrphanTTLDays: defaultUploadOrphanTTLDays,
	}
	if cs == nil {
		return lim
	}
	if cs.UploadMaxBytes != nil && *cs.UploadMaxBytes > 0 {
		lim.MaxBytes = *cs.UploadMaxBytes
	}
	if cs.UploadMaxFiles != nil && *cs.UploadMaxFiles > 0 {
		lim.MaxFiles = *cs.UploadMaxFiles
	}
	if len(cs.UploadAllowedMime) > 0 {
		lim.AllowedMime = append([]string{}, cs.UploadAllowedMime...)
	}
	if cs.UploadDirQuotaBytes != nil && *cs.UploadDirQuotaBytes > 0 {
		lim.DirQuotaBytes = *cs.UploadDirQuotaBytes
	}
	if cs.UploadOrphanTTLDays != nil && *cs.UploadOrphanTTLDays > 0 {
		lim.OrphanTTLDays = *cs.UploadOrphanTTLDays
	}
	return lim
}
