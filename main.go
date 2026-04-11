package main

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func main() {
	logger := newLogger(nil, nil)

	// --- Env validation ---
	authMode := os.Getenv("AUTH_MODE")
	if authMode == "" {
		authMode = "none"
	}
	validModes := map[string]bool{"none": true, "password": true, "mtls": true}
	if !validModes[authMode] {
		logger.Error("invalid AUTH_MODE", "value", authMode)
		os.Exit(1)
	}

	password := os.Getenv("AUTH_PASSWORD")
	if authMode == "password" && password == "" {
		logger.Error("AUTH_MODE=password requires AUTH_PASSWORD")
		os.Exit(1)
	}

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8443"
	}

	blockedIPs := strings.Fields(os.Getenv("BLOCK_IPS"))

	// --- PTY ---
	pm := newPTYManager()
	workdir := os.Getenv("CLAUDE_WORKDIR")
	if workdir == "" {
		if _, err := os.Stat("/workspace"); err == nil {
			workdir = "/workspace"
		}
	}
	go pm.start("claude", []string{}, workdir, logger.Logger)

	// --- Scheduler ---
	sched := newScheduler(pm, workdir)
	sched.loadFromFile()
	go sched.run()

	// --- IM bots (optional) ---
	var im *IMManager
	discordToken := os.Getenv("DISCORD_BOT_TOKEN")
	discordChannel := os.Getenv("DISCORD_CHANNEL_ID")
	telegramToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	telegramChatStr := os.Getenv("TELEGRAM_CHAT_ID")

	if discordToken != "" && discordChannel != "" || telegramToken != "" && telegramChatStr != "" {
		im = newIMManager(logger.Logger)
	}
	if im != nil && discordToken != "" && discordChannel != "" {
		im.addAdapter(newDiscordAdapter(discordToken, discordChannel, logger.Logger))
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
		im.start(pm)
	}

	// --- Auth ---
	auth := newAuthMiddleware(authMode, password)

	// --- Rate limiter ---
	rl := newRateLimiter(2, 5)

	// --- Server ---
	srv := newServer(pm, auth, sched, im, logger.Logger)

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
	if authMode == "mtls" {
		caCertPEM, caKeyPEM, err := generateSelfSignedCert("perch-ca", nil)
		if err != nil {
			logger.Error("generate CA cert", "err", err)
			os.Exit(1)
		}
		serverCertPEM, serverKeyPEM, err := generateSelfSignedCert("perch-server", nil)
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
		tlsCfg, err := buildTLSConfig(authMode, serverCertPEM, serverKeyPEM, caCertPEM)
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

	go func() {
		logger.Info("perch listening", "addr", addr, "auth", authMode)
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
}
