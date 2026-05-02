package main

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const tinyPDF = "%PDF-1.4\n1 0 obj<<>>endobj\ntrailer<<>>\n%%EOF\n"

func b64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// --- 2.3: PDF magic + text heuristic tests ---

func TestMagicMime_PDF(t *testing.T) {
	if got := MagicMime([]byte(tinyPDF)); got != "application/pdf" {
		t.Errorf("PDF magic = %q, want application/pdf", got)
	}
}

func TestLooksLikeText_AcceptsPlainAscii(t *testing.T) {
	if !looksLikeText([]byte("hello world\nhi\n")) {
		t.Error("expected plain ASCII to look like text")
	}
}

func TestLooksLikeText_AcceptsCJK(t *testing.T) {
	if !looksLikeText([]byte("中文也要支援\n日本語\nこんにちは\n")) {
		t.Error("expected CJK UTF-8 to look like text")
	}
}

func TestLooksLikeText_RejectsBinary(t *testing.T) {
	bin := []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD}
	if looksLikeText(bin) {
		t.Error("expected binary to be rejected")
	}
}

func TestLooksLikeText_RejectsHighControlRatio(t *testing.T) {
	// 1 control char per ASCII char → way over 1%
	mix := strings.Repeat("a\x01", 100)
	if looksLikeText([]byte(mix)) {
		t.Error("expected high control-char ratio to be rejected")
	}
}

func TestValidateAttachments_PDFAccepted(t *testing.T) {
	atts := []Attachment{{Filename: "doc.pdf", MimeType: "application/pdf", DataBase64: b64(tinyPDF)}}
	lim := AttachmentLimits{MaxBytes: 1024, MaxFiles: 4, AllowedMime: []string{"application/pdf"}}
	if err := ValidateAttachments(atts, lim); err != nil {
		t.Errorf("PDF should validate: %v", err)
	}
}

func TestValidateAttachments_TextAcceptedViaHeuristic(t *testing.T) {
	atts := []Attachment{
		{Filename: "log.log", MimeType: "text/x-log", DataBase64: b64("2026-05-01 INFO ok\nthing happened\n")},
		{Filename: "data.csv", MimeType: "text/csv", DataBase64: b64("a,b,c\n1,2,3\n")},
		{Filename: "j.json", MimeType: "application/json", DataBase64: b64(`{"k":"v"}`)},
		{Filename: "n.md", MimeType: "text/markdown", DataBase64: b64("# hi\n\nbody\n")},
	}
	lim := AttachmentLimits{MaxBytes: 1024, MaxFiles: 10, AllowedMime: []string{"text/x-log", "text/csv", "application/json", "text/markdown"}}
	if err := ValidateAttachments(atts, lim); err != nil {
		t.Errorf("text heuristic: %v", err)
	}
}

func TestValidateAttachments_TextRejectsBinary(t *testing.T) {
	bin := []byte{0x00, 0x01, 0x02, 0xFF}
	atts := []Attachment{{Filename: "evil.txt", MimeType: "text/plain", DataBase64: base64.StdEncoding.EncodeToString(bin)}}
	lim := AttachmentLimits{MaxBytes: 1024, MaxFiles: 4, AllowedMime: []string{"text/plain"}}
	err := ValidateAttachments(atts, lim)
	if err == nil || !strings.Contains(err.Error(), "not valid text") {
		t.Errorf("expected binary-as-text rejection, got: %v", err)
	}
}

// --- 3.4: filename sanitization ---

func TestSanitizeAttachmentFilename_Traversal(t *testing.T) {
	cases := []string{"../etc/passwd", "..\\windows\\system", "./foo", "../../x"}
	for _, in := range cases {
		got, err := SanitizeAttachmentFilename(in)
		if err != nil {
			continue // some inputs reduce to "" and should error — fine
		}
		if strings.Contains(got, "..") || strings.ContainsAny(got, "/\\") {
			t.Errorf("traversal not stripped: %q → %q", in, got)
		}
	}
}

func TestSanitizeAttachmentFilename_NUL(t *testing.T) {
	got, err := SanitizeAttachmentFilename("a\x00b.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, "\x00") {
		t.Errorf("NUL not stripped: %q", got)
	}
}

func TestSanitizeAttachmentFilename_LengthCap(t *testing.T) {
	long := strings.Repeat("a", 300) + ".txt"
	got, err := SanitizeAttachmentFilename(long)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) > maxAttachmentFilenameLen {
		t.Errorf("len = %d > %d", len(got), maxAttachmentFilenameLen)
	}
	if !strings.HasSuffix(got, ".txt") {
		t.Errorf("extension not preserved: %q", got)
	}
}

func TestSanitizeAttachmentFilename_CJK(t *testing.T) {
	got, err := SanitizeAttachmentFilename("會議紀錄 2026.pdf")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(got, "會議紀錄") {
		t.Errorf("CJK lost: %q", got)
	}
	if !strings.HasSuffix(got, ".pdf") {
		t.Errorf("ext lost: %q", got)
	}
}

func TestSanitizeAttachmentFilename_Empty(t *testing.T) {
	if _, err := SanitizeAttachmentFilename(""); err == nil {
		t.Error("expected error for empty name")
	}
	if _, err := SanitizeAttachmentFilename(".."); err == nil {
		t.Error("expected error for ..")
	}
}

func TestNextAvailableFilename_DupSuffix(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.txt"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	got := NextAvailableFilename(dir, "x.txt")
	if got != "x (2).txt" {
		t.Errorf("dup1 = %q, want x (2).txt", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "x (2).txt"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	got = NextAvailableFilename(dir, "x.txt")
	if got != "x (3).txt" {
		t.Errorf("dup2 = %q, want x (3).txt", got)
	}
}

func TestResolveAttachmentPath_BlocksTraversal(t *testing.T) {
	dir := t.TempDir()
	// Despite passing through SanitizeAttachmentFilename in production, we test
	// the defence-in-depth here directly.
	if _, _, err := ResolveAttachmentPath(dir, "c1", "../escape.txt"); err == nil {
		t.Error("expected traversal block")
	}
	if _, _, err := ResolveAttachmentPath(dir, "c1", "ok.txt"); err != nil {
		t.Errorf("legit name should resolve: %v", err)
	}
	if _, _, err := ResolveAttachmentPath(dir, "../bad", "ok.txt"); err == nil {
		t.Error("convID with .. should reject")
	}
}

// --- 4.4: WriteAttachmentsToDisk ---

func TestWriteAttachmentsToDisk_Success(t *testing.T) {
	work := t.TempDir()
	atts := []Attachment{{Filename: "a.txt", MimeType: "text/plain", DataBase64: b64("hello")}}
	lim := AttachmentLimits{DirQuotaBytes: 1024}
	got, err := WriteAttachmentsToDisk(context.Background(), work, "c1", atts, lim)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if len(got) != 1 || got[0].Filename != "a.txt" {
		t.Fatalf("unexpected: %+v", got)
	}
	if got[0].RelPath != "./uploads/c1/a.txt" {
		t.Errorf("relpath = %q", got[0].RelPath)
	}
	abs := filepath.Join(work, "uploads", "c1", "a.txt")
	body, err := os.ReadFile(abs)
	if err != nil || string(body) != "hello" {
		t.Errorf("file contents wrong: %q err=%v", body, err)
	}
}

func TestWriteAttachmentsToDisk_QuotaRollback(t *testing.T) {
	work := t.TempDir()
	atts := []Attachment{
		{Filename: "a.txt", MimeType: "text/plain", DataBase64: b64("hello")},
		{Filename: "b.txt", MimeType: "text/plain", DataBase64: b64("world!!!")},
	}
	lim := AttachmentLimits{DirQuotaBytes: 5} // a.txt alone is 5 bytes; both together = 13 > 5
	_, err := WriteAttachmentsToDisk(context.Background(), work, "c1", atts, lim)
	if err == nil {
		t.Fatal("expected quota error")
	}
	if !errors.Is(err, ErrUploadQuotaExceeded) {
		t.Errorf("expected ErrUploadQuotaExceeded, got: %v", err)
	}
	// On quota failure (pre-write check), no file should exist yet.
	convDir := filepath.Join(work, "uploads", "c1")
	if entries, _ := os.ReadDir(convDir); len(entries) > 0 {
		t.Errorf("expected no files after quota reject, got %d", len(entries))
	}
}

func TestWriteAttachmentsToDisk_ImageBypass(t *testing.T) {
	work := t.TempDir()
	atts := []Attachment{{Filename: "x.png", MimeType: "image/png", DataBase64: b64("AAA")}}
	got, err := WriteAttachmentsToDisk(context.Background(), work, "c1", atts, AttachmentLimits{DirQuotaBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("image should bypass disk path, got %d persisted", len(got))
	}
	if _, err := os.Stat(filepath.Join(work, "uploads", "c1")); !os.IsNotExist(err) {
		t.Error("image-only request should not even create the conv dir")
	}
}

func TestWriteAttachmentsToDisk_DupFilenameSuffix(t *testing.T) {
	work := t.TempDir()
	atts1 := []Attachment{{Filename: "x.txt", MimeType: "text/plain", DataBase64: b64("aaa")}}
	if _, err := WriteAttachmentsToDisk(context.Background(), work, "c1", atts1, AttachmentLimits{DirQuotaBytes: 1024}); err != nil {
		t.Fatal(err)
	}
	atts2 := []Attachment{{Filename: "x.txt", MimeType: "text/plain", DataBase64: b64("bbb")}}
	got, err := WriteAttachmentsToDisk(context.Background(), work, "c1", atts2, AttachmentLimits{DirQuotaBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Filename != "x (2).txt" {
		t.Errorf("dup name = %q, want 'x (2).txt'", got[0].Filename)
	}
}

// --- 4.5: BuildPromptFilePrefix ---

func TestBuildPromptFilePrefix_OneFilePlusText(t *testing.T) {
	p := []PersistedAttachment{{Filename: "a.csv", RelPath: "./uploads/c1/a.csv", MimeType: "text/csv", SizeBytes: 142}}
	got := BuildPromptFilePrefix(p, "look at this")
	want := "[file: ./uploads/c1/a.csv (text/csv, 142 B)]\n\nlook at this"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestBuildPromptFilePrefix_MultipleFiles(t *testing.T) {
	p := []PersistedAttachment{
		{Filename: "a.csv", RelPath: "./uploads/c1/a.csv", MimeType: "text/csv", SizeBytes: 100},
		{Filename: "b.pdf", RelPath: "./uploads/c1/b.pdf", MimeType: "application/pdf", SizeBytes: 1024 * 1024},
	}
	got := BuildPromptFilePrefix(p, "x")
	if !strings.HasPrefix(got, "[file: ./uploads/c1/a.csv") {
		t.Errorf("first prefix wrong: %q", got)
	}
	if !strings.Contains(got, "[file: ./uploads/c1/b.pdf (application/pdf, 1.0 MiB)]") {
		t.Errorf("second prefix wrong: %q", got)
	}
	if !strings.HasSuffix(got, "\n\nx") {
		t.Errorf("trailing text wrong: %q", got)
	}
}

func TestBuildPromptFilePrefix_FilesOnlyNoText(t *testing.T) {
	p := []PersistedAttachment{{Filename: "a.csv", RelPath: "./uploads/c1/a.csv", MimeType: "text/csv", SizeBytes: 10}}
	got := BuildPromptFilePrefix(p, "  ")
	want := "[file: ./uploads/c1/a.csv (text/csv, 10 B)]"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestBuildPromptFilePrefix_NoFiles(t *testing.T) {
	if got := BuildPromptFilePrefix(nil, "hi"); got != "hi" {
		t.Errorf("no files = %q, want hi", got)
	}
}

func TestHumanSize(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KiB"},
		{1500, "1.5 KiB"},
		{1024 * 1024, "1.0 MiB"},
		{int64(1.5 * 1024 * 1024), "1.5 MiB"},
		{1024 * 1024 * 1024, "1.0 GiB"},
	}
	for _, c := range cases {
		if got := humanSize(c.n); got != c.want {
			t.Errorf("humanSize(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

// --- 6.4: cleanupOrphanUploads ---

func TestCleanupOrphanUploads_StaleRemoved(t *testing.T) {
	work := t.TempDir()
	uploads := filepath.Join(work, "uploads")
	staleDir := filepath.Join(uploads, "stale")
	freshDir := filepath.Join(uploads, "fresh")
	for _, d := range []string{staleDir, freshDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	staleFile := filepath.Join(staleDir, "old.txt")
	freshFile := filepath.Join(freshDir, "new.txt")
	for _, f := range []string{staleFile, freshFile} {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Backdate stale dir.
	old := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(staleFile, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(staleDir, old, old); err != nil {
		t.Fatal(err)
	}

	kept, removed, err := cleanupOrphanUploads(work, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 || kept != 1 {
		t.Errorf("kept=%d removed=%d, want 1/1", kept, removed)
	}
	if _, err := os.Stat(staleDir); !os.IsNotExist(err) {
		t.Error("stale dir should have been removed")
	}
	if _, err := os.Stat(freshDir); err != nil {
		t.Errorf("fresh dir should remain: %v", err)
	}
}

func TestCleanupOrphanUploads_MissingRoot(t *testing.T) {
	work := t.TempDir() // no uploads/ subdir
	kept, removed, err := cleanupOrphanUploads(work, 7*24*time.Hour)
	if err != nil {
		t.Errorf("missing root should be no-op, got: %v", err)
	}
	if kept != 0 || removed != 0 {
		t.Errorf("expected 0/0, got %d/%d", kept, removed)
	}
}
