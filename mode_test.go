package main

import (
	"testing"
)

func TestParseOperatingModeDefault(t *testing.T) {
	t.Setenv("PERCH_MODE", "")
	m, err := parseOperatingMode()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m != ModeSingle {
		t.Errorf("expected single, got %q", m)
	}
}

func TestParseOperatingModeInvalid(t *testing.T) {
	t.Setenv("PERCH_MODE", "invalid")
	_, err := parseOperatingMode()
	if err == nil {
		t.Fatal("expected error for invalid PERCH_MODE, got nil")
	}
}

func TestValidateMultiModeEnvMissingVars(t *testing.T) {
	t.Setenv("GITLAB_CLIENT_ID", "")
	t.Setenv("GITLAB_CLIENT_SECRET", "")
	t.Setenv("GITLAB_URL", "")
	if err := validateMultiModeEnv(); err == nil {
		t.Fatal("expected error when GitLab vars missing, got nil")
	}
}

func TestValidateMultiModeEnvComplete(t *testing.T) {
	t.Setenv("GITLAB_CLIENT_ID", "cid")
	t.Setenv("GITLAB_CLIENT_SECRET", "csecret")
	t.Setenv("GITLAB_URL", "https://gitlab.example.com")
	if err := validateMultiModeEnv(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
