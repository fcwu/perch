package main

import (
	"encoding/json"
	"net/http"
	"syscall"
	"time"
)

// redactString replaces a non-empty string pointer value with "••••".
func redactString(s *string) *string {
	if s != nil && *s != "" {
		v := "••••"
		return &v
	}
	return s
}

// redactSettings returns a copy of settings with sensitive fields replaced by "••••".
func redactSettings(s RuntimeSettings) RuntimeSettings {
	// Auth.Password
	if s.Auth != nil {
		auth := *s.Auth
		auth.Password = redactString(auth.Password)
		s.Auth = &auth
	}
	// Discord.BotToken
	if s.Discord != nil {
		discord := *s.Discord
		discord.BotToken = redactString(discord.BotToken)
		s.Discord = &discord
	}
	// Telegram.BotToken
	if s.Telegram != nil {
		telegram := *s.Telegram
		telegram.BotToken = redactString(telegram.BotToken)
		s.Telegram = &telegram
	}
	// GitLab.ClientSecret
	if s.GitLab != nil {
		gitlab := *s.GitLab
		gitlab.ClientSecret = redactString(gitlab.ClientSecret)
		s.GitLab = &gitlab
	}
	// Workspace.GitToken
	if s.Workspace != nil {
		workspace := *s.Workspace
		workspace.GitToken = redactString(workspace.GitToken)
		s.Workspace = &workspace
	}
	return s
}

// handleGetSettings handles GET /api/settings — admin only.
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	if s.sm == nil {
		http.Error(w, "settings not configured", http.StatusServiceUnavailable)
		return
	}
	current := s.sm.GetEffective()
	redacted := redactSettings(current)
	s.sm.mu.RLock()
	dirty := s.sm.dirty
	s.sm.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"settings":         redacted,
		"restart_required": dirty,
	})
}

// handlePatchSettings handles PATCH /api/settings — admin only.
func (s *Server) handlePatchSettings(w http.ResponseWriter, r *http.Request) {
	if s.sm == nil {
		http.Error(w, "settings not configured", http.StatusServiceUnavailable)
		return
	}
	var delta RuntimeSettings
	if err := json.NewDecoder(r.Body).Decode(&delta); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	restartRequired, err := s.sm.Patch(delta)
	if err != nil {
		http.Error(w, "failed to save settings: "+err.Error(), http.StatusInternalServerError)
		return
	}
	current := s.sm.Get()
	redacted := redactSettings(current)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"settings":         redacted,
		"restart_required": restartRequired,
	})
}

// handleAdminRestart handles POST /api/admin/restart — admin only.
// Returns HTTP 202 immediately and sends SIGTERM to PID 1 in a goroutine.
func (s *Server) handleAdminRestart(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusAccepted)
	go func() {
		time.Sleep(100 * time.Millisecond) // brief delay to let response flush
		syscall.Kill(1, syscall.SIGTERM)
	}()
}
