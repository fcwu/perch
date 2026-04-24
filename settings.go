package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// RuntimeSettings holds the delta overrides stored in settings.json.
// Nil pointer = not overridden (use env var value).
type RuntimeSettings struct {
	Agent     *AgentSettings     `json:"agent,omitempty"`
	Auth      *AuthSettings      `json:"auth,omitempty"`
	RateLimit *RateLimitSettings `json:"rate_limit,omitempty"`
	Network   *NetworkSettings   `json:"network,omitempty"`
	Discord   *DiscordSettings   `json:"discord,omitempty"`
	Telegram  *TelegramSettings  `json:"telegram,omitempty"`
	GitLab    *GitLabSettings    `json:"gitlab,omitempty"`
	Workspace *WorkspaceSettings `json:"workspace,omitempty"`
	Log       *LogSettings       `json:"log,omitempty"`
}

type AgentSettings struct {
	Runtime *string `json:"runtime,omitempty"` // "claude" | "opencode"
	Args    *string `json:"args,omitempty"`
}

type AuthSettings struct {
	Method   *string `json:"method,omitempty"`   // restart-required
	Password *string `json:"password,omitempty"` // immediate
}

type RateLimitSettings struct {
	RPM *int `json:"rpm,omitempty"` // immediate
}

type NetworkSettings struct {
	BlockIPs []string `json:"block_ips,omitempty"` // immediate
}

type DiscordSettings struct {
	BotToken       *string  `json:"bot_token,omitempty"`        // restart-required
	ChannelID      *string  `json:"channel_id,omitempty"`       // restart-required
	AllowedUserIDs []string `json:"allowed_user_ids,omitempty"` // immediate
	ACPEnabled     *bool    `json:"acp_enabled,omitempty"`      // restart-required
	ACPExecutable  *string  `json:"acp_executable,omitempty"`   // restart-required
}

type TelegramSettings struct {
	BotToken *string `json:"bot_token,omitempty"` // restart-required
	ChatID   *string `json:"chat_id,omitempty"`   // restart-required
}

type GitLabSettings struct {
	URL          *string  `json:"url,omitempty"`           // restart-required
	ClientID     *string  `json:"client_id,omitempty"`     // restart-required
	ClientSecret *string  `json:"client_secret,omitempty"` // restart-required
	RedirectURI  *string  `json:"redirect_uri,omitempty"`  // restart-required
	AllowedIDs   []string `json:"allowed_ids,omitempty"`   // restart-required
	AdminIDs     []string `json:"admin_ids,omitempty"`     // restart-required
}

type WorkspaceSettings struct {
	SyncEnabled    *bool   `json:"sync_enabled,omitempty"`    // immediate
	SyncInterval   *string `json:"sync_interval,omitempty"`   // immediate
	GitToken       *string `json:"git_token,omitempty"`       // immediate
	NotifyChannel  *string `json:"notify_channel,omitempty"`  // immediate
	SyncSubmodules *bool   `json:"sync_submodules,omitempty"` // immediate
}

type LogSettings struct {
	Format *string `json:"format,omitempty"` // next-session
}

// SettingsManager loads, stores, and applies runtime settings.
type SettingsManager struct {
	path    string
	mu      sync.RWMutex
	current RuntimeSettings // current effective overrides
	dirty   bool            // true if any restart-required field was changed since last start
}

// NewSettingsManager creates a manager. Loads existing settings.json if present.
func NewSettingsManager(path string) (*SettingsManager, error) {
	sm := &SettingsManager{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return sm, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, &sm.current); err != nil {
		return nil, err
	}
	return sm, nil
}

// Get returns a copy of current settings (safe to read concurrently).
func (sm *SettingsManager) Get() RuntimeSettings {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.current
}

// Patch merges delta into current settings, saves atomically, and returns
// whether any restart-required fields were changed.
func (sm *SettingsManager) Patch(delta RuntimeSettings) (restartRequired bool, err error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	changed := false

	// Agent settings (next-session tier)
	if delta.Agent != nil {
		if sm.current.Agent == nil {
			sm.current.Agent = &AgentSettings{}
		}
		if delta.Agent.Runtime != nil {
			sm.current.Agent.Runtime = delta.Agent.Runtime
		}
		if delta.Agent.Args != nil {
			sm.current.Agent.Args = delta.Agent.Args
		}
	}

	// Auth settings
	if delta.Auth != nil {
		if sm.current.Auth == nil {
			sm.current.Auth = &AuthSettings{}
		}
		if delta.Auth.Method != nil {
			sm.current.Auth.Method = delta.Auth.Method
			changed = true // restart-required
		}
		if delta.Auth.Password != nil {
			sm.current.Auth.Password = delta.Auth.Password
		}
	}

	// RateLimit settings (immediate)
	if delta.RateLimit != nil {
		if sm.current.RateLimit == nil {
			sm.current.RateLimit = &RateLimitSettings{}
		}
		if delta.RateLimit.RPM != nil {
			sm.current.RateLimit.RPM = delta.RateLimit.RPM
		}
	}

	// Network settings (immediate)
	if delta.Network != nil {
		if sm.current.Network == nil {
			sm.current.Network = &NetworkSettings{}
		}
		if delta.Network.BlockIPs != nil {
			sm.current.Network.BlockIPs = delta.Network.BlockIPs
		}
	}

	// Discord settings
	if delta.Discord != nil {
		if sm.current.Discord == nil {
			sm.current.Discord = &DiscordSettings{}
		}
		if delta.Discord.BotToken != nil {
			sm.current.Discord.BotToken = delta.Discord.BotToken
			changed = true // restart-required
		}
		if delta.Discord.ChannelID != nil {
			sm.current.Discord.ChannelID = delta.Discord.ChannelID
			changed = true // restart-required
		}
		if delta.Discord.AllowedUserIDs != nil {
			sm.current.Discord.AllowedUserIDs = delta.Discord.AllowedUserIDs
		}
		if delta.Discord.ACPEnabled != nil {
			sm.current.Discord.ACPEnabled = delta.Discord.ACPEnabled
			changed = true // restart-required
		}
		if delta.Discord.ACPExecutable != nil {
			sm.current.Discord.ACPExecutable = delta.Discord.ACPExecutable
			changed = true // restart-required
		}
	}

	// Telegram settings (restart-required)
	if delta.Telegram != nil {
		if sm.current.Telegram == nil {
			sm.current.Telegram = &TelegramSettings{}
		}
		if delta.Telegram.BotToken != nil {
			sm.current.Telegram.BotToken = delta.Telegram.BotToken
			changed = true
		}
		if delta.Telegram.ChatID != nil {
			sm.current.Telegram.ChatID = delta.Telegram.ChatID
			changed = true
		}
	}

	// GitLab settings (restart-required)
	if delta.GitLab != nil {
		if sm.current.GitLab == nil {
			sm.current.GitLab = &GitLabSettings{}
		}
		if delta.GitLab.URL != nil {
			sm.current.GitLab.URL = delta.GitLab.URL
			changed = true
		}
		if delta.GitLab.ClientID != nil {
			sm.current.GitLab.ClientID = delta.GitLab.ClientID
			changed = true
		}
		if delta.GitLab.ClientSecret != nil {
			sm.current.GitLab.ClientSecret = delta.GitLab.ClientSecret
			changed = true
		}
		if delta.GitLab.RedirectURI != nil {
			sm.current.GitLab.RedirectURI = delta.GitLab.RedirectURI
			changed = true
		}
		if delta.GitLab.AllowedIDs != nil {
			sm.current.GitLab.AllowedIDs = delta.GitLab.AllowedIDs
			changed = true
		}
		if delta.GitLab.AdminIDs != nil {
			sm.current.GitLab.AdminIDs = delta.GitLab.AdminIDs
			changed = true
		}
	}

	// Workspace settings (immediate)
	if delta.Workspace != nil {
		if sm.current.Workspace == nil {
			sm.current.Workspace = &WorkspaceSettings{}
		}
		if delta.Workspace.SyncEnabled != nil {
			sm.current.Workspace.SyncEnabled = delta.Workspace.SyncEnabled
		}
		if delta.Workspace.SyncInterval != nil {
			sm.current.Workspace.SyncInterval = delta.Workspace.SyncInterval
		}
		if delta.Workspace.GitToken != nil {
			sm.current.Workspace.GitToken = delta.Workspace.GitToken
		}
		if delta.Workspace.NotifyChannel != nil {
			sm.current.Workspace.NotifyChannel = delta.Workspace.NotifyChannel
		}
		if delta.Workspace.SyncSubmodules != nil {
			sm.current.Workspace.SyncSubmodules = delta.Workspace.SyncSubmodules
		}
	}

	// Log settings (next-session)
	if delta.Log != nil {
		if sm.current.Log == nil {
			sm.current.Log = &LogSettings{}
		}
		if delta.Log.Format != nil {
			sm.current.Log.Format = delta.Log.Format
		}
	}

	if changed {
		sm.dirty = true
	}

	if err := sm.save(); err != nil {
		return sm.dirty, err
	}
	return sm.dirty, nil
}

// save writes current settings atomically (temp file + rename).
func (sm *SettingsManager) save() error {
	data, err := json.MarshalIndent(sm.current, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(sm.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, "settings-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		// Clean up temp file if rename failed
		os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, sm.path)
}

// EffectiveRPM returns rate limit RPM: settings override or env var fallback.
func (sm *SettingsManager) EffectiveRPM(envDefault int) int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.current.RateLimit != nil && sm.current.RateLimit.RPM != nil {
		return *sm.current.RateLimit.RPM
	}
	return envDefault
}

// EffectiveBlockIPs returns block IPs list: settings override or env var fallback.
func (sm *SettingsManager) EffectiveBlockIPs(envDefault []string) []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.current.Network != nil && sm.current.Network.BlockIPs != nil {
		return sm.current.Network.BlockIPs
	}
	return envDefault
}

// EffectivePassword returns auth password: settings override or env var fallback.
func (sm *SettingsManager) EffectivePassword(envDefault string) string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.current.Auth != nil && sm.current.Auth.Password != nil {
		return *sm.current.Auth.Password
	}
	return envDefault
}

// EffectiveAgentArgs returns agent extra args: settings override or env var fallback.
func (sm *SettingsManager) EffectiveAgentArgs(envDefault string) string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.current.Agent != nil && sm.current.Agent.Args != nil {
		return *sm.current.Agent.Args
	}
	return envDefault
}

// EffectiveAgentRuntime returns agent runtime: settings override or env var fallback.
func (sm *SettingsManager) EffectiveAgentRuntime(envDefault string) string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.current.Agent != nil && sm.current.Agent.Runtime != nil {
		return *sm.current.Agent.Runtime
	}
	return envDefault
}

// EffectiveAuthMethod returns auth.method from settings if set, else envDefault.
func (sm *SettingsManager) EffectiveAuthMethod(envDefault string) string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.current.Auth != nil && sm.current.Auth.Method != nil {
		return *sm.current.Auth.Method
	}
	return envDefault
}

// EffectiveDiscordToken returns the Discord bot token from settings if set, else envDefault.
func (sm *SettingsManager) EffectiveDiscordToken(envDefault string) string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.current.Discord != nil && sm.current.Discord.BotToken != nil {
		return *sm.current.Discord.BotToken
	}
	return envDefault
}

// EffectiveTelegramToken returns the Telegram bot token from settings if set, else envDefault.
func (sm *SettingsManager) EffectiveTelegramToken(envDefault string) string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.current.Telegram != nil && sm.current.Telegram.BotToken != nil {
		return *sm.current.Telegram.BotToken
	}
	return envDefault
}
