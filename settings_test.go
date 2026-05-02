package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewSettingsManager_NoFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	sm, err := NewSettingsManager(path)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if sm == nil {
		t.Fatal("expected non-nil SettingsManager")
	}
	got := sm.Get()
	if got.Agent != nil || got.Auth != nil || got.RateLimit != nil {
		t.Errorf("expected empty settings, got %+v", got)
	}
}

func TestPatch_MergesDelta(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewSettingsManager(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rpm := 42
	delta := RuntimeSettings{
		RateLimit: &RateLimitSettings{RPM: &rpm},
	}
	_, err = sm.Patch(delta)
	if err != nil {
		t.Fatalf("Patch error: %v", err)
	}

	got := sm.Get()
	if got.RateLimit == nil || got.RateLimit.RPM == nil {
		t.Fatal("expected RateLimit.RPM to be set")
	}
	if *got.RateLimit.RPM != 42 {
		t.Errorf("expected 42, got %d", *got.RateLimit.RPM)
	}
}

func TestPatch_ChatUploadFieldsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewSettingsManager(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	quota := int64(2048)
	ttl := 3
	maxBytes := int64(1024)
	delta := RuntimeSettings{
		Chat: &ChatSettings{
			UploadMaxBytes:      &maxBytes,
			UploadDirQuotaBytes: &quota,
			UploadOrphanTTLDays: &ttl,
			UploadAllowedMime:   []string{"text/plain"},
		},
	}
	if _, err := sm.Patch(delta); err != nil {
		t.Fatalf("Patch error: %v", err)
	}

	got := sm.GetEffective()
	if got.Chat == nil {
		t.Fatal("expected Chat in effective settings")
	}
	if got.Chat.UploadDirQuotaBytes == nil || *got.Chat.UploadDirQuotaBytes != 2048 {
		t.Errorf("UploadDirQuotaBytes = %v, want 2048", got.Chat.UploadDirQuotaBytes)
	}
	if got.Chat.UploadOrphanTTLDays == nil || *got.Chat.UploadOrphanTTLDays != 3 {
		t.Errorf("UploadOrphanTTLDays = %v, want 3", got.Chat.UploadOrphanTTLDays)
	}
	if got.Chat.UploadMaxBytes == nil || *got.Chat.UploadMaxBytes != 1024 {
		t.Errorf("UploadMaxBytes = %v, want 1024", got.Chat.UploadMaxBytes)
	}
	if len(got.Chat.UploadAllowedMime) != 1 || got.Chat.UploadAllowedMime[0] != "text/plain" {
		t.Errorf("UploadAllowedMime = %v, want [text/plain]", got.Chat.UploadAllowedMime)
	}
}

func TestPatch_RestartRequiredForAuthMethod(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewSettingsManager(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	method := "password"
	delta := RuntimeSettings{
		Auth: &AuthSettings{Method: &method},
	}
	restartRequired, err := sm.Patch(delta)
	if err != nil {
		t.Fatalf("Patch error: %v", err)
	}
	if !restartRequired {
		t.Error("expected restartRequired=true for Auth.Method change")
	}
	if !sm.dirty {
		t.Error("expected sm.dirty=true")
	}
}

func TestPatch_NoRestartRequiredForImmediateField(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewSettingsManager(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rpm := 20
	delta := RuntimeSettings{
		RateLimit: &RateLimitSettings{RPM: &rpm},
	}
	restartRequired, err := sm.Patch(delta)
	if err != nil {
		t.Fatalf("Patch error: %v", err)
	}
	if restartRequired {
		t.Error("expected restartRequired=false for RateLimit.RPM change")
	}
}

func TestEffectiveRPM(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewSettingsManager(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No override: should return env default.
	if got := sm.EffectiveRPM(10); got != 10 {
		t.Errorf("expected 10, got %d", got)
	}

	// With override.
	rpm := 50
	sm.Patch(RuntimeSettings{RateLimit: &RateLimitSettings{RPM: &rpm}})
	if got := sm.EffectiveRPM(10); got != 50 {
		t.Errorf("expected 50, got %d", got)
	}
}

func TestPatch_AtomicSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	sm, err := NewSettingsManager(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rpm := 99
	if _, err := sm.Patch(RuntimeSettings{RateLimit: &RateLimitSettings{RPM: &rpm}}); err != nil {
		t.Fatalf("Patch error: %v", err)
	}

	// File must exist.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}

	// Must be valid JSON.
	var out RuntimeSettings
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("invalid JSON in settings file: %v", err)
	}
	if out.RateLimit == nil || out.RateLimit.RPM == nil || *out.RateLimit.RPM != 99 {
		t.Errorf("unexpected settings in file: %+v", out)
	}
}

func TestEffectiveBlockIPs_Fallback(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewSettingsManager(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defaultIPs := []string{"1.2.3.4"}
	if got := sm.EffectiveBlockIPs(defaultIPs); len(got) != 1 || got[0] != "1.2.3.4" {
		t.Errorf("expected default IPs, got %v", got)
	}

	// With override.
	sm.Patch(RuntimeSettings{Network: &NetworkSettings{BlockIPs: []string{"5.6.7.8", "9.10.11.12"}}})
	if got := sm.EffectiveBlockIPs(defaultIPs); len(got) != 2 || got[0] != "5.6.7.8" {
		t.Errorf("expected overridden IPs, got %v", got)
	}
}

func TestEffectivePassword_Fallback(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewSettingsManager(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := sm.EffectivePassword("envpass"); got != "envpass" {
		t.Errorf("expected envpass, got %s", got)
	}

	pw := "newpass"
	sm.Patch(RuntimeSettings{Auth: &AuthSettings{Password: &pw}})
	if got := sm.EffectivePassword("envpass"); got != "newpass" {
		t.Errorf("expected newpass, got %s", got)
	}
}

func TestEffectiveAgentRuntime_Fallback(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewSettingsManager(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := sm.EffectiveAgentRuntime("claude"); got != "claude" {
		t.Errorf("expected claude, got %s", got)
	}

	rt := "opencode"
	sm.Patch(RuntimeSettings{Agent: &AgentSettings{Runtime: &rt}})
	if got := sm.EffectiveAgentRuntime("claude"); got != "opencode" {
		t.Errorf("expected opencode, got %s", got)
	}
}
