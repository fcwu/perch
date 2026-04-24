package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

// buildTime is injected at build time via -ldflags "-X main.buildTime=..."
var buildTime = "unknown"

func main() {
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
	validMethods := map[string]bool{"none": true, "password": true, "mtls": true, "gitlab": true}
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
	// Export resolved addr so claude hook commands can POST to the right port
	// without requiring the user to set LISTEN_ADDR explicitly.
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
	// --- ACP mode (optional): DISCORD_ACP_ENABLED=true uses claude-agent-acp subprocess ---
	acpEnabled := os.Getenv("DISCORD_ACP_ENABLED") == "true"
	if acpEnabled {
		exe := os.Getenv("ACP_EXECUTABLE")
		if exe == "" {
			exe = "claude-agent-acp"
		}
		logger.Info("Discord ACP mode enabled", "executable", exe)
	}

	if im != nil && discordToken != "" {
		discordSess = newDiscordSessionManager(runtime, discordToken, discordChannel, discordAllowedDMUsers, workdir, logger.Logger)
		im.addAdapter(discordSess)
	}
	if im != nil && telegramToken != "" && telegramChatStr != "" {
		chatID, err := parseTelegramChatID(telegramChatStr)
		if err != nil {
			logger.Error("invalid TELEGRAM_CHAT_ID", "value", telegramChatStr, "err", err)
			os.Exit(1)
		}
		im.addAdapter(newTelegramAdapter(telegramToken, chatID, logger.Logger))
	}
	if im != nil {
		im.start(IMConfig{PTY: pm, ACPEnabled: acpEnabled})
	}
	if discordSess != nil {
		sched.ptyLookup = discordSess.PTYForTarget
		sched.onFire = discordSess.OnScheduledFire
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
	adminHub := newAdminHub()

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
	sm.SetSeed(buildEnvSeed())
	var userRL *UserRateLimiter
	if gitlabAuth.enabled() {
		userRL = newUserRateLimiter(rateLimitRPM)
	}

	srv := newServerWithMode(pm, auth, im, sessProvider, userSessions, gitlabAuth, adminAuth, adminHub, store, userRL, sm, mode, logger.Logger)

	// Apply rate limiting to sensitive endpoints only
	sensitivePaths := map[string]bool{"/login": true, "/bootstrap": true}
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

	// --- TLS (mtls mode only) ---
	var finalListener net.Listener = blockedListener
	if authMethod == "mtls" {
		caCertPEM, caKeyPEM, err := generateSelfSignedCert("perch-ca", nil, nil)
		if err != nil {
			logger.Error("generate CA cert", "err", err)
			os.Exit(1)
		}
		// Parse CA cert/key so the server cert can be signed by the CA.
		caBlock, _ := pem.Decode(caCertPEM)
		caCert, err := x509.ParseCertificate(caBlock.Bytes)
		if err != nil {
			logger.Error("parse CA cert", "err", err)
			os.Exit(1)
		}
		caKeyBlock, _ := pem.Decode(caKeyPEM)
		caKey, err := x509.ParseECPrivateKey(caKeyBlock.Bytes)
		if err != nil {
			logger.Error("parse CA key", "err", err)
			os.Exit(1)
		}
		serverCertPEM, serverKeyPEM, err := generateSelfSignedCert("perch-server", caCert, caKey)
		if err != nil {
			logger.Error("generate server cert", "err", err)
			os.Exit(1)
		}
		p12Data, err := generateClientP12(caCertPEM, caKeyPEM, "perch")
		if err != nil {
			logger.Error("generate client p12", "err", err)
			os.Exit(1)
		}
		srv.mux.Handle("/bootstrap", newBootstrapHandler(p12Data, "data/bootstrap.used"))
		tlsCfg, err := buildTLSConfig(authMethod, serverCertPEM, serverKeyPEM, caCertPEM)
		if err != nil {
			logger.Error("build TLS config", "err", err)
			os.Exit(1)
		}
		finalListener = tls.NewListener(blockedListener, tlsCfg)
	}

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
// base layer for GetEffective(). Only non-empty env vars populate the seed.
func buildEnvSeed() RuntimeSettings {
	var s RuntimeSettings

	// Auth
	authMethod := os.Getenv("AUTH_METHOD")
	if authMethod == "" {
		authMethod = os.Getenv("AUTH_MODE")
	}
	password := os.Getenv("PERCH_PASSWORD")
	if password == "" {
		password = os.Getenv("AUTH_PASSWORD")
	}
	if authMethod != "" || password != "" {
		s.Auth = &AuthSettings{}
		if authMethod != "" {
			s.Auth.Method = strPtr(authMethod)
		}
		if password != "" {
			s.Auth.Password = strPtr(password)
		}
	}

	// Rate limit
	if v := os.Getenv("RATE_LIMIT_RPM"); v != "" {
		var rpm int
		if _, err := fmt.Sscanf(v, "%d", &rpm); err == nil {
			s.RateLimit = &RateLimitSettings{RPM: intPtr(rpm)}
		}
	}

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
	discordACP := os.Getenv("DISCORD_ACP_ENABLED")
	acpExe := os.Getenv("ACP_EXECUTABLE")
	if discordToken != "" || discordChannel != "" || discordAllowedRaw != "" || discordACP != "" || acpExe != "" {
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
		if discordACP != "" {
			b := discordACP == "true"
			s.Discord.ACPEnabled = boolPtr(b)
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

	return s
}
