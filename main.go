package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// buildTime is injected at build time via -ldflags "-X main.buildTime=..."
var buildTime = "unknown"

func main() {
	// Sub-command dispatch: `./perch mcp` runs the stdio MCP server and exits
	// without touching the HTTP server or scheduler. All other invocations
	// fall through to the standard server bootstrap.
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		os.Exit(runMCPServer())
		return
	}

	logger := newLogger(nil, nil)
	runtime, err := loadAgentRuntime()
	if err != nil {
		logger.Error("invalid AGENT_RUNTIME", "err", err)
		os.Exit(1)
	}

	// --- Settings manager (loaded early so restart-required fields are effective) ---
	sm, err := NewSettingsManager("/data/settings.json")
	if err != nil {
		logger.Warn("settings manager init failed", "err", err)
		sm = &SettingsManager{path: "/data/settings.json"}
	}

	// --- Operating mode ---
	mode, err := parseOperatingMode()
	if err != nil {
		logger.Error("invalid PERCH_MODE", "err", err)
		os.Exit(1)
	}
	if mode == ModeMulti {
		if err := validateMultiModeEnv(); err != nil {
			logger.Error("PERCH_MODE=multi configuration error", "err", err)
			os.Exit(1)
		}
	}

	// --- Auth method (AUTH_METHOD, with fallback to legacy AUTH_MODE, then settings.json) ---
	authMethod := sm.EffectiveAuthMethod(os.Getenv("AUTH_METHOD"))
	if authMethod == "" {
		authMethod = sm.EffectiveAuthMethod(os.Getenv("AUTH_MODE")) // backward compat
	}
	if authMethod == "" {
		authMethod = "none"
	}
	if authMethod == "mtls" {
		logger.Error("AUTH_METHOD=mtls is no longer supported; use none, password, or gitlab")
		os.Exit(1)
	}
	validMethods := map[string]bool{"none": true, "password": true, "gitlab": true}
	if !validMethods[authMethod] {
		logger.Error("invalid AUTH_METHOD", "value", authMethod)
		os.Exit(1)
	}
	if mode == ModeMulti {
		// Multi-user always uses GitLab; AUTH_METHOD is ignored.
		authMethod = "gitlab"
	}

	password := os.Getenv("PERCH_PASSWORD")
	if password == "" {
		password = os.Getenv("AUTH_PASSWORD") // backward compat
	}
	if authMethod == "password" && password == "" {
		logger.Error("AUTH_METHOD=password requires PERCH_PASSWORD")
		os.Exit(1)
	}

	if authMethod == "gitlab" && mode == ModeSingle {
		// Single-user GitLab: requires GitLab vars.
		if err := validateMultiModeEnv(); err != nil {
			logger.Error("AUTH_METHOD=gitlab configuration error", "err", err)
			os.Exit(1)
		}
	}

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	os.Setenv("LISTEN_ADDR", addr)

	blockedIPs := strings.Fields(os.Getenv("BLOCK_IPS"))

	// --- PTY ---
	pm := newPTYManager()
	workdir := os.Getenv("CLAUDE_WORKDIR")
	if workdir == "" {
		if _, err := os.Stat("/workspace"); err == nil {
			workdir = "/workspace"
		}
	}
	go pm.start(runtime.Command, runtime.MainArgs(), workdir, logger.Logger, runtime.DefaultEnv, runtime.SessionEnv("")...)

	// --- Scheduler ---
	sched := newScheduler(pm, workdir, logger.Logger)
	sched.loadFromFile()
	go sched.run()
	go sched.watch()

	// --- IM bots (optional) ---
	var im *IMManager
	var discordSess *DiscordSessionManager
	discordToken := sm.EffectiveDiscordToken(os.Getenv("DISCORD_BOT_TOKEN"))
	discordChannel := os.Getenv("DISCORD_CHANNEL_ID")
	discordAllowedDMRaw := os.Getenv("DISCORD_ALLOWED_USER_IDS")
	telegramToken := sm.EffectiveTelegramToken(os.Getenv("TELEGRAM_BOT_TOKEN"))
	telegramChatStr := os.Getenv("TELEGRAM_CHAT_ID")

	var discordAllowedDMUsers []string
	for _, id := range strings.Split(discordAllowedDMRaw, ",") {
		if id := strings.TrimSpace(id); id != "" {
			discordAllowedDMUsers = append(discordAllowedDMUsers, id)
		}
	}

	if discordToken != "" || telegramToken != "" && telegramChatStr != "" {
		im = newIMManager(logger.Logger)
	}
	if im != nil && discordToken != "" {
		discordSess = newDiscordSessionManager(runtime, discordToken, discordChannel, discordAllowedDMUsers, workdir, logger.Logger)
		discordSess.SetSettings(sm)
		im.addAdapter(discordSess)
	}
	if im != nil && telegramToken != "" && telegramChatStr != "" {
		chatID, err := parseTelegramChatID(telegramChatStr)
		if err != nil {
			logger.Error("invalid TELEGRAM_CHAT_ID", "value", telegramChatStr, "err", err)
			os.Exit(1)
		}
		im.addAdapter(newTelegramAdapter(runtime, telegramToken, chatID, workdir, logger.Logger))
	}
	if im != nil {
		im.start(IMConfig{})
	}
	if discordSess != nil {
		sched.dispatch = discordSess.DispatchScheduled
	}

	// --- Auth ---
	var auth *AuthMiddleware
	if authMethod != "gitlab" {
		auth = newAuthMiddleware(authMethod, password)
	}

	// --- Rate limiter ---
	rl := newRateLimiter(2, 5)

	// --- Server ---
	var sessProvider SessionProvider
	if discordSess != nil {
		sessProvider = discordSess
	}
	gitlabAuth := newGitLabAuth(mode, authMethod)
	adminAuth := newAdminAuth()
	adminHub := newManagementHub()

	// Open SQLite store if DB_PATH is set (or default to /data/perch.db).
	// Set DB_PATH= (empty) to disable history.
	var store *Store
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "/data/perch.db"
	}
	var storeErr error
	store, storeErr = OpenStore(dbPath)
	if storeErr != nil {
		logger.Warn("failed to open store, history disabled", "path", dbPath, "err", storeErr)
	}

	userSessions := newUserSessionManager(runtime, workdir, logger.Logger, store, adminHub)
	// Per-user rate limiter
	rateLimitRPM := 10
	if v := os.Getenv("RATE_LIMIT_RPM"); v != "" {
		fmt.Sscanf(v, "%d", &rateLimitRPM)
	}
	sm.SetSeed(buildEnvSeed(runtime))
	var userRL *UserRateLimiter
	if gitlabAuth.enabled() {
		userRL = newUserRateLimiter(rateLimitRPM)
	}

	srv := newServerWithMode(pm, auth, im, sessProvider, userSessions, gitlabAuth, adminAuth, adminHub, store, userRL, sm, mode, logger.Logger)
	chatACP := newACPUserSessionManager(runtime, workdir, store, adminHub, logger.Logger)
	chatACP.SetSettings(sm)
	srv.chatSessions = chatACP
	srv.defaultRuntime = runtime
	srv.pool = chatACP.Pool()
	srv.scheduler = sched

	// Resolve the perch binary path once so we can hand it to ACP runtimes
	// as the MCP server command. Falls back to "perch" on PATH if the lookup
	// fails — in development this is good enough.
	perchBin, exeErr := os.Executable()
	if exeErr != nil {
		logger.Warn("os.Executable failed; MCP servers may not launch", "err", exeErr)
		perchBin = "perch"
	}

	chatACP.SetMCPServers(func(rt AgentRuntime, userID, convID string) []map[string]any {
		if !rt.SupportsMCP {
			return nil
		}
		return []map[string]any{
			{
				"type":    "stdio",
				"command": perchBin,
				"args":    []string{"mcp"},
				"env": map[string]any{
					"PERCH_USER_ID": userID,
					"PERCH_CONV_ID": convID,
					"PERCH_DB_PATH": dbPath,
				},
			},
		}
	})

	// Wire the chat-schedules dispatcher into the scheduler and load existing
	// rows on boot. The scheduler ticker handles both legacy JSONL jobs and
	// chat_schedules from the store.
	if store != nil {
		sched.SetChatFire(store, chatACP.RunScheduledPrompt)
		sched.LoadChatSchedules()
	}

	// Cleanup orphaned per-conversation upload directories on startup.
	if cs := EffectiveAttachmentLimits(sm.GetEffective().Chat); cs.OrphanTTLDays > 0 {
		ttl := time.Duration(cs.OrphanTTLDays) * 24 * time.Hour
		if kept, removed, err := cleanupOrphanUploads(workdir, ttl); err != nil {
			logger.Logger.Warn("uploads orphan cleanup failed", "err", err)
		} else {
			logger.Logger.Info("uploads orphan cleanup", "kept", kept, "removed", removed, "ttl_days", cs.OrphanTTLDays)
		}
	}

	// Apply rate limiting to sensitive endpoints only
	sensitivePaths := map[string]bool{"/login": true}
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sensitivePaths[r.URL.Path] {
			rl.wrap(srv).ServeHTTP(w, r)
		} else {
			srv.ServeHTTP(w, r)
		}
	})

	// --- Listener ---
	baseListener, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Error("listen", "addr", addr, "err", err)
		os.Exit(1)
	}
	bl := newIPBlockList(blockedIPs)
	blockedListener := wrapListener(baseListener, bl)

	var finalListener net.Listener = blockedListener

	httpSrv := &http.Server{Handler: finalHandler}

	// --- Signal handling ---
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	// --- Workspace git auto-sync (optional) ---
	syncCfg := LoadSyncConfig()
	if syncCfg.Enabled {
		if err := injectGitToken(syncCfg.GitToken, syncCfg.WorkspacePath, syncCfg.UID, syncCfg.GID, logger.Logger); err != nil {
			logger.Warn("workspace_sync: credential injection error", "err", err)
		}
		var notifyFn NotifyFunc
		if im != nil && syncCfg.NotifyChannelID != "" {
			notifyFn = func(errType string, msg string) {
				if err := im.SendText(syncCfg.NotifyChannelID, msg); err != nil {
					logger.Warn("workspace_sync: discord notify failed", "errType", errType, "err", err)
				}
			}
		}
		StartWorkspaceSync(ctx, syncCfg, logger.Logger, notifyFn)
	}

	go func() {
		logger.Info("perch listening", "addr", addr, "auth", authMethod, "mode", mode, "built", buildTime)
		if err := httpSrv.Serve(finalListener); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "err", err)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")
	if im != nil {
		im.stop()
	}
	httpSrv.Shutdown(context.Background())
	if store != nil {
		store.Close()
	}
}

// buildEnvSeed reads environment variables into a RuntimeSettings used as the
// base layer for GetEffective(). Hardcoded defaults are always populated;
// env vars override them when set.
func buildEnvSeed(runtime AgentRuntime) RuntimeSettings {
	var s RuntimeSettings

	// Agent defaults
	s.Agent = &AgentSettings{Runtime: strPtr(runtime.Name)}
	if runtime.ArgsEnv != "" {
		if v := os.Getenv(runtime.ArgsEnv); v != "" {
			s.Agent.Args = strPtr(v)
		}
	}

	// Auth
	authMethod := os.Getenv("AUTH_METHOD")
	if authMethod == "" {
		authMethod = os.Getenv("AUTH_MODE")
	}
	if authMethod == "" {
		authMethod = "none"
	}
	password := os.Getenv("PERCH_PASSWORD")
	if password == "" {
		password = os.Getenv("AUTH_PASSWORD")
	}
	s.Auth = &AuthSettings{Method: strPtr(authMethod)}
	if password != "" {
		s.Auth.Password = strPtr(password)
	}

	// Rate limit — default 10 RPM, overridden by env var
	rpm := 10
	if v := os.Getenv("RATE_LIMIT_RPM"); v != "" {
		fmt.Sscanf(v, "%d", &rpm)
	}
	s.RateLimit = &RateLimitSettings{RPM: intPtr(rpm)}

	// Log format — default "text"
	logFormat := "text"
	if os.Getenv("LOG_FORMAT") == "json" {
		logFormat = "json"
	}
	s.Log = &LogSettings{Format: strPtr(logFormat)}

	// Chat upload limits — defaults from attachments.go
	chat := ChatSettings{
		UploadMaxBytes:      int64Ptr(int64(defaultUploadMaxBytes)),
		UploadMaxFiles:      intPtr(defaultUploadMaxFiles),
		UploadAllowedMime:   append([]string{}, defaultUploadAllowedMime...),
		UploadDirQuotaBytes: int64Ptr(int64(defaultUploadDirQuotaBytes)),
		UploadOrphanTTLDays: intPtr(defaultUploadOrphanTTLDays),
	}
	if v := os.Getenv("CHAT_UPLOAD_MAX_BYTES"); v != "" {
		var n int64
		fmt.Sscanf(v, "%d", &n)
		if n > 0 {
			chat.UploadMaxBytes = int64Ptr(n)
		}
	}
	if v := os.Getenv("CHAT_UPLOAD_MAX_FILES"); v != "" {
		var n int
		fmt.Sscanf(v, "%d", &n)
		if n > 0 {
			chat.UploadMaxFiles = intPtr(n)
		}
	}
	if v := os.Getenv("CHAT_UPLOAD_ALLOWED_MIME"); v != "" {
		var mimes []string
		for _, m := range strings.Split(v, ",") {
			m = strings.TrimSpace(m)
			if m != "" {
				mimes = append(mimes, m)
			}
		}
		if len(mimes) > 0 {
			chat.UploadAllowedMime = mimes
		}
	}
	if v := os.Getenv("CHAT_UPLOAD_DIR_QUOTA_BYTES"); v != "" {
		var n int64
		fmt.Sscanf(v, "%d", &n)
		if n > 0 {
			chat.UploadDirQuotaBytes = int64Ptr(n)
		}
	}
	if v := os.Getenv("CHAT_UPLOAD_ORPHAN_TTL_DAYS"); v != "" {
		var n int
		fmt.Sscanf(v, "%d", &n)
		if n > 0 {
			chat.UploadOrphanTTLDays = intPtr(n)
		}
	}
	s.Chat = &chat

	// Network
	if v := os.Getenv("BLOCK_IPS"); v != "" {
		var ips []string
		for _, ip := range strings.Fields(v) {
			if ip != "" {
				ips = append(ips, ip)
			}
		}
		if len(ips) > 0 {
			s.Network = &NetworkSettings{BlockIPs: ips}
		}
	}

	// Discord
	discordToken := os.Getenv("DISCORD_BOT_TOKEN")
	discordChannel := os.Getenv("DISCORD_CHANNEL_ID")
	discordAllowedRaw := os.Getenv("DISCORD_ALLOWED_USER_IDS")
	acpExe := os.Getenv("ACP_EXECUTABLE")
	if discordToken != "" || discordChannel != "" || discordAllowedRaw != "" || acpExe != "" {
		s.Discord = &DiscordSettings{}
		if discordToken != "" {
			s.Discord.BotToken = strPtr(discordToken)
		}
		if discordChannel != "" {
			s.Discord.ChannelID = strPtr(discordChannel)
		}
		if discordAllowedRaw != "" {
			var ids []string
			for _, id := range strings.Split(discordAllowedRaw, ",") {
				if id := strings.TrimSpace(id); id != "" {
					ids = append(ids, id)
				}
			}
			s.Discord.AllowedUserIDs = ids
		}
		if acpExe != "" {
			s.Discord.ACPExecutable = strPtr(acpExe)
		}
	}

	// Telegram
	telegramToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	telegramChat := os.Getenv("TELEGRAM_CHAT_ID")
	if telegramToken != "" || telegramChat != "" {
		s.Telegram = &TelegramSettings{}
		if telegramToken != "" {
			s.Telegram.BotToken = strPtr(telegramToken)
		}
		if telegramChat != "" {
			s.Telegram.ChatID = strPtr(telegramChat)
		}
	}

	// GitLab
	gitlabURL := os.Getenv("GITLAB_URL")
	gitlabClientID := os.Getenv("GITLAB_CLIENT_ID")
	gitlabClientSecret := os.Getenv("GITLAB_CLIENT_SECRET")
	gitlabRedirect := os.Getenv("GITLAB_REDIRECT_URI")
	if gitlabURL != "" || gitlabClientID != "" || gitlabClientSecret != "" || gitlabRedirect != "" {
		s.GitLab = &GitLabSettings{}
		if gitlabURL != "" {
			s.GitLab.URL = strPtr(gitlabURL)
		}
		if gitlabClientID != "" {
			s.GitLab.ClientID = strPtr(gitlabClientID)
		}
		if gitlabClientSecret != "" {
			s.GitLab.ClientSecret = strPtr(gitlabClientSecret)
		}
		if gitlabRedirect != "" {
			s.GitLab.RedirectURI = strPtr(gitlabRedirect)
		}
	}

	// Workspace
	wsSyncEnabled := os.Getenv("WORKSPACE_GIT_SYNC_ENABLED")
	wsSyncInterval := os.Getenv("WORKSPACE_GIT_SYNC_INTERVAL")
	wsGitToken := os.Getenv("WORKSPACE_GIT_TOKEN")
	wsNotifyChannel := os.Getenv("WORKSPACE_GIT_SYNC_NOTIFY_CHANNEL")
	wsSubmodules := os.Getenv("WORKSPACE_GIT_SYNC_SUBMODULES")
	if wsSyncEnabled != "" || wsSyncInterval != "" || wsGitToken != "" || wsNotifyChannel != "" || wsSubmodules != "" {
		s.Workspace = &WorkspaceSettings{}
		if wsSyncEnabled != "" {
			b := wsSyncEnabled == "true" || wsSyncEnabled == "1"
			s.Workspace.SyncEnabled = boolPtr(b)
		}
		if wsSyncInterval != "" {
			s.Workspace.SyncInterval = strPtr(wsSyncInterval)
		}
		if wsGitToken != "" {
			s.Workspace.GitToken = strPtr(wsGitToken)
		}
		if wsNotifyChannel != "" {
			s.Workspace.NotifyChannel = strPtr(wsNotifyChannel)
		}
		if wsSubmodules != "" {
			b := wsSubmodules == "true" || wsSubmodules == "1"
			s.Workspace.SyncSubmodules = boolPtr(b)
		}
	}

	return s
}
