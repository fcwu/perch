package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

// newTestDiscordManagerWithWorkdir builds a DiscordSessionManager rooted at a
// tmp workdir so disk-write tests can verify on-disk state.
func newTestDiscordManagerWithWorkdir(t *testing.T) *DiscordSessionManager {
	t.Helper()
	work := t.TempDir()
	return &DiscordSessionManager{
		workdir:        work,
		logger:         slog.Default(),
		sessions:       map[string]*discordSession{},
		channelPrivate: map[string]bool{},
	}
}

func TestDiscordProcessAttachments_ImagePersistedSeparation(t *testing.T) {
	d := newTestDiscordManagerWithWorkdir(t)

	// HTTP server returning a tiny PNG and a tiny PDF on different paths.
	pngBytes := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00}
	pdfBytes := []byte("%PDF-1.4\nminimal\n%%EOF\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".png") {
			w.Write(pngBytes)
			return
		}
		if strings.HasSuffix(r.URL.Path, ".pdf") {
			w.Write(pdfBytes)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	atts := []*discordgo.MessageAttachment{
		{Filename: "shot.png", ContentType: "image/png", Size: len(pngBytes), URL: srv.URL + "/shot.png"},
		{Filename: "spec.pdf", ContentType: "application/pdf", Size: len(pdfBytes), URL: srv.URL + "/spec.pdf"},
	}
	imgs, persisted, failed, disallowed := d.processAttachments("ch-001", atts)
	if len(imgs) != 1 || imgs[0].MimeType != "image/png" {
		t.Errorf("images = %+v, want one PNG", imgs)
	}
	if len(persisted) != 1 || persisted[0].Filename != "spec.pdf" {
		t.Errorf("persisted = %+v, want one PDF", persisted)
	}
	if len(failed) != 0 || len(disallowed) != 0 {
		t.Errorf("unexpected failed=%v disallowed=%v", failed, disallowed)
	}
	// PDF on disk under uploads/ch-001/spec.pdf
	pdfPath := filepath.Join(d.workdir, "uploads", "ch-001", "spec.pdf")
	if _, err := os.Stat(pdfPath); err != nil {
		t.Errorf("PDF not on disk: %v", err)
	}
}

func TestDiscordProcessAttachments_DisallowedMime(t *testing.T) {
	d := newTestDiscordManagerWithWorkdir(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("fake video bytes"))
	}))
	defer srv.Close()

	atts := []*discordgo.MessageAttachment{
		{Filename: "demo.mp4", ContentType: "video/mp4", Size: 16, URL: srv.URL + "/demo.mp4"},
	}
	imgs, persisted, failed, disallowed := d.processAttachments("ch-x", atts)
	if len(imgs) != 0 || len(persisted) != 0 || len(failed) != 0 {
		t.Errorf("expected only disallowed entry, got imgs=%d persisted=%d failed=%d", len(imgs), len(persisted), len(failed))
	}
	if len(disallowed) != 1 || disallowed[0].Mime != "video/mp4" {
		t.Errorf("disallowed = %+v, want video/mp4", disallowed)
	}
}

func TestDiscordProcessAttachments_FetchFailureRollsBack(t *testing.T) {
	d := newTestDiscordManagerWithWorkdir(t)

	// Server always 500s.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	atts := []*discordgo.MessageAttachment{
		{Filename: "log.log", ContentType: "text/x-log", Size: 16, URL: srv.URL + "/log.log"},
	}
	_, persisted, failed, _ := d.processAttachments("ch-y", atts)
	if len(persisted) != 0 {
		t.Errorf("expected no persisted on fetch fail, got %d", len(persisted))
	}
	if len(failed) != 1 || failed[0] != "log.log" {
		t.Errorf("failed = %+v", failed)
	}
	// No partial dir / file on disk
	if _, err := os.Stat(filepath.Join(d.workdir, "uploads", "ch-y", "log.log")); err == nil {
		t.Error("partial file should not exist after fetch failure")
	}
}
