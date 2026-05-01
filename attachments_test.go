package main

import (
	"encoding/base64"
	"strings"
	"testing"
)

// 1×1 transparent PNG.
const tinyPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNgAAIAAAUAAen63NgAAAAASUVORK5CYII="

// 1×1 white JPEG (q=50).
const tinyJPEGBase64 = "/9j/4AAQSkZJRgABAQAASABIAAD/2wBDAAEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQH/2wBDAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQH/wAARCAABAAEDASIAAhEBAxEB/8QAFQABAQAAAAAAAAAAAAAAAAAAAAr/xAAUEAEAAAAAAAAAAAAAAAAAAAAA/8QAFAEBAAAAAAAAAAAAAAAAAAAAAP/EABQRAQAAAAAAAAAAAAAAAAAAAAD/2gAMAwEAAhEDEQA/AL+AB//Z"

func TestMagicMime_PNG(t *testing.T) {
	d, _ := base64.StdEncoding.DecodeString(tinyPNGBase64)
	if got := MagicMime(d); got != "image/png" {
		t.Errorf("PNG magic = %q, want image/png", got)
	}
}

func TestMagicMime_JPEG(t *testing.T) {
	d, _ := base64.StdEncoding.DecodeString(tinyJPEGBase64)
	if got := MagicMime(d); got != "image/jpeg" {
		t.Errorf("JPEG magic = %q, want image/jpeg", got)
	}
}

func TestMagicMime_GIF(t *testing.T) {
	gif87 := []byte("GIF87a\x00\x00")
	if got := MagicMime(gif87); got != "image/gif" {
		t.Errorf("GIF87a magic = %q, want image/gif", got)
	}
	gif89 := []byte("GIF89a\x00\x00")
	if got := MagicMime(gif89); got != "image/gif" {
		t.Errorf("GIF89a magic = %q, want image/gif", got)
	}
}

func TestMagicMime_WEBP(t *testing.T) {
	webp := []byte("RIFF\x00\x00\x00\x00WEBPVP8 ")
	if got := MagicMime(webp); got != "image/webp" {
		t.Errorf("WEBP magic = %q, want image/webp", got)
	}
}

func TestMagicMime_Unknown(t *testing.T) {
	if got := MagicMime([]byte("not an image")); got != "" {
		t.Errorf("text magic = %q, want empty", got)
	}
}

func TestValidateAttachments_Empty(t *testing.T) {
	if err := ValidateAttachments(nil, AttachmentLimits{MaxFiles: 4}); err != nil {
		t.Errorf("nil atts: %v", err)
	}
}

func TestValidateAttachments_Happy(t *testing.T) {
	atts := []Attachment{{Filename: "ok.png", MimeType: "image/png", DataBase64: tinyPNGBase64}}
	lim := AttachmentLimits{
		MaxBytes:    1024,
		MaxFiles:    4,
		AllowedMime: []string{"image/png"},
	}
	if err := ValidateAttachments(atts, lim); err != nil {
		t.Errorf("happy path: %v", err)
	}
}

func TestValidateAttachments_StripsDataURIPrefix(t *testing.T) {
	atts := []Attachment{{
		Filename:   "ok.png",
		MimeType:   "image/png",
		DataBase64: "data:image/png;base64," + tinyPNGBase64,
	}}
	lim := AttachmentLimits{MaxBytes: 1024, MaxFiles: 4, AllowedMime: []string{"image/png"}}
	if err := ValidateAttachments(atts, lim); err != nil {
		t.Errorf("dataURI prefix: %v", err)
	}
	if strings.HasPrefix(atts[0].DataBase64, "data:") {
		t.Errorf("expected dataURI prefix stripped, got %q", atts[0].DataBase64[:20])
	}
}

func TestValidateAttachments_TooMany(t *testing.T) {
	atts := []Attachment{
		{Filename: "1.png", MimeType: "image/png", DataBase64: tinyPNGBase64},
		{Filename: "2.png", MimeType: "image/png", DataBase64: tinyPNGBase64},
	}
	lim := AttachmentLimits{MaxBytes: 1024, MaxFiles: 1, AllowedMime: []string{"image/png"}}
	err := ValidateAttachments(atts, lim)
	if err == nil || !strings.Contains(err.Error(), "too many") {
		t.Errorf("expected too-many error, got: %v", err)
	}
}

func TestValidateAttachments_DisallowedMime(t *testing.T) {
	atts := []Attachment{{Filename: "x.svg", MimeType: "image/svg+xml", DataBase64: tinyPNGBase64}}
	lim := AttachmentLimits{MaxBytes: 1024, MaxFiles: 4, AllowedMime: []string{"image/png", "image/jpeg"}}
	err := ValidateAttachments(atts, lim)
	if err == nil || !strings.Contains(err.Error(), "not in allow-list") {
		t.Errorf("expected disallow-list error, got: %v", err)
	}
}

func TestValidateAttachments_OversizedRejected(t *testing.T) {
	atts := []Attachment{{Filename: "big.png", MimeType: "image/png", DataBase64: tinyPNGBase64}}
	lim := AttachmentLimits{MaxBytes: 10, MaxFiles: 4, AllowedMime: []string{"image/png"}}
	err := ValidateAttachments(atts, lim)
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Errorf("expected oversized error, got: %v", err)
	}
}

func TestValidateAttachments_MagicMismatch(t *testing.T) {
	// Client claims jpeg but bytes are PNG.
	atts := []Attachment{{Filename: "lie.jpg", MimeType: "image/jpeg", DataBase64: tinyPNGBase64}}
	lim := AttachmentLimits{MaxBytes: 1024, MaxFiles: 4, AllowedMime: []string{"image/png", "image/jpeg"}}
	err := ValidateAttachments(atts, lim)
	if err == nil || !strings.Contains(err.Error(), "magic bytes") {
		t.Errorf("expected magic-mismatch error, got: %v", err)
	}
}

func TestValidateAttachments_InvalidBase64(t *testing.T) {
	atts := []Attachment{{Filename: "bad.png", MimeType: "image/png", DataBase64: "@@@not-base64@@@"}}
	lim := AttachmentLimits{MaxBytes: 1024, MaxFiles: 4, AllowedMime: []string{"image/png"}}
	err := ValidateAttachments(atts, lim)
	if err == nil || !strings.Contains(err.Error(), "invalid base64") {
		t.Errorf("expected base64 error, got: %v", err)
	}
}

func TestAttachmentsToACPBlocks(t *testing.T) {
	atts := []Attachment{
		{Filename: "a.png", MimeType: "image/png", DataBase64: "AAAA"},
		{Filename: "b.jpg", MimeType: "image/jpeg", DataBase64: "BBBB"},
	}
	blocks := AttachmentsToACPBlocks(atts)
	if len(blocks) != 2 {
		t.Fatalf("len = %d, want 2", len(blocks))
	}
	if blocks[0].Type != "image" || blocks[0].Source.MediaType != "image/png" || blocks[0].Source.Data != "AAAA" {
		t.Errorf("block[0] = %+v", blocks[0])
	}
	if blocks[1].Source.MediaType != "image/jpeg" || blocks[1].Source.Data != "BBBB" {
		t.Errorf("block[1] = %+v", blocks[1])
	}
}

func TestEffectiveAttachmentLimits_Defaults(t *testing.T) {
	lim := EffectiveAttachmentLimits(nil)
	if lim.MaxBytes != int64(defaultUploadMaxBytes) || lim.MaxFiles != defaultUploadMaxFiles {
		t.Errorf("defaults wrong: %+v", lim)
	}
	if len(lim.AllowedMime) != len(defaultUploadAllowedMime) {
		t.Errorf("default allow-list len wrong: %v", lim.AllowedMime)
	}
}

func TestEffectiveAttachmentLimits_Override(t *testing.T) {
	bytes := int64(2048)
	files := 1
	cs := &ChatSettings{
		UploadMaxBytes:    &bytes,
		UploadMaxFiles:    &files,
		UploadAllowedMime: []string{"image/png"},
	}
	lim := EffectiveAttachmentLimits(cs)
	if lim.MaxBytes != 2048 || lim.MaxFiles != 1 || len(lim.AllowedMime) != 1 {
		t.Errorf("override wrong: %+v", lim)
	}
}
