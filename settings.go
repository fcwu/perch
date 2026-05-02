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
	Chat      *ChatSettings      `json:"chat,omitempty"`
}

type ChatSettings struct {
	UploadMaxBytes      *int64   `json:"upload_max_bytes,omitempty"`       // immediate
	UploadMaxFiles      *int     `json:"upload_max_files,omitempty"`       // immediate
	UploadAllowedMime   []string `json:"upload_allowed_mime,omitempty"`    // immediate
	UploadDirQuotaBytes *int64   `json:"upload_dir_quota_bytes,omitempty"` // immediate
	UploadOrphanTTLDays *int     `json:"upload_orphan_ttl_days,omitempty"` // immediate
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
	base    RuntimeSettings // env-var seed (set once at startup via SetSeed)
	current RuntimeSettings // settings.json overrides only
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

// SetSeed stores the env-var defaults so GetEffective can return the full picture.
func (sm *SettingsManager) SetSeed(base RuntimeSettings) {
	sm.mu.Lock()
	sm.base = base
	sm.mu.Unlock()
}

// GetEffective returns env-var base merged with settings.json overrides.
// Use this for the GET /api/settings response so the UI always sees real values.
func (sm *SettingsManager) GetEffective() RuntimeSettings {
	sm.mu.RLock()
	base := sm.base
	cur := sm.current
	sm.mu.RUnlock()
	return mergeSettings(base, cur)
}

// mergeSettings returns base with any non-nil fields from override applied.
func mergeSettings(base, override RuntimeSettings) RuntimeSettings {
	r := base

	if override.Agent != nil {
		if r.Agent == nil {
			r.Agent = &AgentSettings{}
		}
		a := *r.Agent
		if override.Agent.Runtime != nil {
			a.Runtime = override.Agent.Runtime
		}
		if override.Agent.Args != nil {
			a.Args = override.Agent.Args
		}
		r.Agent = &a
	}

	if override.Auth != nil {
		if r.Auth == nil {
			r.Auth = &AuthSettings{}
		}
		a := *r.Auth
		if override.Auth.Method != nil {
			a.Method = override.Auth.Method
		}
		if override.Auth.Password != nil {
			a.Password = override.Auth.Password
		}
		r.Auth = &a
	}

	if override.RateLimit != nil {
		if r.RateLimit == nil {
			r.RateLimit = &RateLimitSettings{}
		}
		rl := *r.RateLimit
		if override.RateLimit.RPM != nil {
			rl.RPM = override.RateLimit.RPM
		}
		r.RateLimit = &rl
	}

	if override.Network != nil {
		if r.Network == nil {
			r.Network = &NetworkSettings{}
		}
		n := *r.Network
		if override.Network.BlockIPs != nil {
			n.BlockIPs = override.Network.BlockIPs
		}
		r.Network = &n
	}

	if override.Discord != nil {
		if r.Discord == nil {
			r.Discord = &DiscordSettings{}
		}
		d := *r.Discord
		if override.Discord.BotToken != nil {
			d.BotToken = override.Discord.BotToken
		}
		if override.Discord.ChannelID != nil {
			d.ChannelID = override.Discord.ChannelID
		}
		if override.Discord.AllowedUserIDs != nil {
			d.AllowedUserIDs = override.Discord.AllowedUserIDs
		}
		if override.Discord.ACPExecutable != nil {
			d.ACPExecutable = override.Discord.ACPExecutable
		}
		r.Discord = &d
	}

	if override.Telegram != nil {
		if r.Telegram == nil {
			r.Telegram = &TelegramSettings{}
		}
		t := *r.Telegram
		if override.Telegram.BotToken != nil {
			t.BotToken = override.Telegram.BotToken
		}
		if override.Telegram.ChatID != nil {
			t.ChatID = override.Telegram.ChatID
		}
		r.Telegram = &t
	}

	if override.GitLab != nil {
		if r.GitLab == nil {
			r.GitLab = &GitLabSettings{}
		}
		g := *r.GitLab
		if override.GitLab.URL != nil {
			g.URL = override.GitLab.URL
		}
		if override.GitLab.ClientID != nil {
			g.ClientID = override.GitLab.ClientID
		}
		if override.GitLab.ClientSecret != nil {
			g.ClientSecret = override.GitLab.ClientSecret
		}
		if override.GitLab.RedirectURI != nil {
			g.RedirectURI = override.GitLab.RedirectURI
		}
		if override.GitLab.AllowedIDs != nil {
			g.AllowedIDs = override.GitLab.AllowedIDs
		}
		if override.GitLab.AdminIDs != nil {
			g.AdminIDs = override.GitLab.AdminIDs
		}
		r.GitLab = &g
	}

	if override.Workspace != nil {
		if r.Workspace == nil {
			r.Workspace = &WorkspaceSettings{}
		}
		w := *r.Workspace
		if override.Workspace.SyncEnabled != nil {
			w.SyncEnabled = override.Workspace.SyncEnabled
		}
		if override.Workspace.SyncInterval != nil {
			w.SyncInterval = override.Workspace.SyncInterval
		}
		if override.Workspace.GitToken != nil {
			w.GitToken = override.Workspace.GitToken
		}
		if override.Workspace.NotifyChannel != nil {
			w.NotifyChannel = override.Workspace.NotifyChannel
		}
		if override.Workspace.SyncSubmodules != nil {
			w.SyncSubmodules = override.Workspace.SyncSubmodules
		}
		r.Workspace = &w
	}

	if override.Log != nil {
		if r.Log == nil {
			r.Log = &LogSettings{}
		}
		l := *r.Log
		if override.Log.Format != nil {
			l.Format = override.Log.Format
		}
		r.Log = &l
	}

	if override.Chat != nil {
		if r.Chat == nil {
			r.Chat = &ChatSettings{}
		}
		c := *r.Chat
		if override.Chat.UploadMaxBytes != nil {
			c.UploadMaxBytes = override.Chat.UploadMaxBytes
		}
		if override.Chat.UploadMaxFiles != nil {
			c.UploadMaxFiles = override.Chat.UploadMaxFiles
		}
		if override.Chat.UploadAllowedMime != nil {
			c.UploadAllowedMime = override.Chat.UploadAllowedMime
		}
		if override.Chat.UploadDirQuotaBytes != nil {
			c.UploadDirQuotaBytes = override.Chat.UploadDirQuotaBytes
		}
		if override.Chat.UploadOrphanTTLDays != nil {
			c.UploadOrphanTTLDays = override.Chat.UploadOrphanTTLDays
		}
		r.Chat = &c
	}

	return r
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }
func int64Ptr(i int64) *int64 { return &i }
func boolPtr(b bool) *bool    { return &b }

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

	if delta.Chat != nil {
		if sm.current.Chat == nil {
			sm.current.Chat = &ChatSettings{}
		}
		if delta.Chat.UploadMaxBytes != nil {
			sm.current.Chat.UploadMaxBytes = delta.Chat.UploadMaxBytes
		}
		if delta.Chat.UploadMaxFiles != nil {
			sm.current.Chat.UploadMaxFiles = delta.Chat.UploadMaxFiles
		}
		if delta.Chat.UploadAllowedMime != nil {
			sm.current.Chat.UploadAllowedMime = delta.Chat.UploadAllowedMime
		}
		if delta.Chat.UploadDirQuotaBytes != nil {
			sm.current.Chat.UploadDirQuotaBytes = delta.Chat.UploadDirQuotaBytes
		}
		if delta.Chat.UploadOrphanTTLDays != nil {
			sm.current.Chat.UploadOrphanTTLDays = delta.Chat.UploadOrphanTTLDays
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
