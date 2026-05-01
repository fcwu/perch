package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// Default limits applied when no env var or settings override is present.
const (
	defaultUploadMaxBytes = 10 * 1024 * 1024 // 10 MiB
	defaultUploadMaxFiles = 4
)

var defaultUploadAllowedMime = []string{"image/png", "image/jpeg", "image/gif", "image/webp"}

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
	MaxBytes     int64
	MaxFiles     int
	AllowedMime  []string
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
		got := MagicMime(decoded)
		if got == "" {
			return fmt.Errorf("attachment %d (%s): unrecognised file format (magic bytes don't match any allowed type)", i, att.Filename)
		}
		if got != att.MimeType {
			return fmt.Errorf("attachment %d (%s): claimed %q but magic bytes say %q", i, att.Filename, att.MimeType, got)
		}
		att.DataBase64 = raw
	}
	return nil
}

// MagicMime inspects the leading bytes of decoded payload and returns the
// matching MIME for supported image formats. Returns "" when no match.
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
	}
	return ""
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
		MaxBytes:    int64(defaultUploadMaxBytes),
		MaxFiles:    defaultUploadMaxFiles,
		AllowedMime: append([]string{}, defaultUploadAllowedMime...),
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
	return lim
}
