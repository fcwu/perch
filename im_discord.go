package main

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// mentionRe matches Discord user mention tokens like <@123456789> or <@!123456789>.
var mentionRe = regexp.MustCompile(`<@!?\d+>`)

const (
	emojiEyes   = "👀"
	emojiGear   = "⚙️"
	emojiCheck  = "✅"
	emojiCross  = "❌"
	emojiSpeech = "💬"

	discordMaxLen = 1900 // leave room below Discord's 2000-char hard limit
)

// workingEmojis are cycled as "still working" reactions when PTY output is detected.
var workingEmojis = []string{"⌨️", "💭", "✍️"}

// tableBlockRe matches two or more consecutive Markdown table lines (starting with |).
var tableBlockRe = regexp.MustCompile(`(?m)(?:^\|[^\n]*\n){2,}`)

// channelSessionID derives a deterministic UUID v5-like string from a Discord channel ID.
func channelSessionID(channelID string) string {
	h := sha1.Sum([]byte("perch-discord-v1:" + channelID))
	b := h[:16]
	b[6] = (b[6] & 0x0f) | 0x50 // version 5
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// displayWidth returns the terminal display width of s, counting CJK/wide runes as 2.
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		if isWideRune(r) {
			w += 2
		} else {
			w++
		}
	}
	return w
}

// isWideRune reports whether r occupies two terminal columns (CJK and fullwidth ranges).
func isWideRune(r rune) bool {
	return (r >= 0x1100 && r <= 0x115F) || // Hangul Jamo
		r == 0x2329 || r == 0x232A ||
		(r >= 0x2E80 && r <= 0x303E) || // CJK Radicals / Kangxi
		(r >= 0x3040 && r <= 0x33FF) || // Hiragana, Katakana, CJK Symbols
		(r >= 0x3400 && r <= 0x4DBF) || // CJK Extension A
		(r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0xAC00 && r <= 0xD7AF) || // Hangul Syllables
		(r >= 0xF900 && r <= 0xFAFF) || // CJK Compatibility Ideographs
		(r >= 0xFE10 && r <= 0xFE4F) || // Vertical / CJK Compatibility Forms
		(r >= 0xFF01 && r <= 0xFF60) || // Fullwidth ASCII
		(r >= 0xFFE0 && r <= 0xFFE6) // Fullwidth Signs
}

// padRight pads s on the right with spaces to reach the target display width.
func padRight(s string, width int) string {
	dw := displayWidth(s)
	if dw >= width {
		return s
	}
	return s + strings.Repeat(" ", width-dw)
}

// isSepCell reports whether a table cell is a Markdown separator (e.g. ---, :---:).
func isSepCell(s string) bool {
	return strings.ContainsRune(s, '-') && strings.Trim(s, "-: ") == ""
}

// alignTable re-aligns the column widths of a Markdown table block so that
// | characters line up. Wide (CJK) characters are counted as 2 columns.
func alignTable(table string) string {
	lines := strings.Split(strings.TrimRight(table, "\n"), "\n")
	if len(lines) == 0 {
		return table
	}

	// Parse each line into trimmed cells (drop the leading/trailing empty parts
	// that come from splitting "| a | b |" on "|").
	rows := make([][]string, len(lines))
	for i, line := range lines {
		parts := strings.Split(line, "|")
		if len(parts) >= 2 {
			parts = parts[1 : len(parts)-1]
		}
		cells := make([]string, len(parts))
		for j, p := range parts {
			cells[j] = strings.TrimSpace(p)
		}
		rows[i] = cells
	}

	maxCols := 0
	for _, row := range rows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}
	if maxCols == 0 {
		return table
	}

	// Determine column widths: separator rows contribute ≥ 3 dashes.
	widths := make([]int, maxCols)
	for _, row := range rows {
		sep := len(row) > 0
		for _, cell := range row {
			if !isSepCell(cell) {
				sep = false
				break
			}
		}
		for j, cell := range row {
			if j >= maxCols {
				break
			}
			w := displayWidth(cell)
			if sep && w < 3 {
				w = 3
			}
			if w > widths[j] {
				widths[j] = w
			}
		}
	}

	// Reconstruct with aligned columns.
	var sb strings.Builder
	for i, row := range rows {
		sep := len(row) > 0
		for _, cell := range row {
			if !isSepCell(cell) {
				sep = false
				break
			}
		}
		sb.WriteString("|")
		for j := 0; j < maxCols; j++ {
			cell := ""
			if j < len(row) {
				cell = row[j]
			}
			w := widths[j]
			if sep {
				sb.WriteString(" " + strings.Repeat("-", w) + " |")
			} else {
				sb.WriteString(" " + padRight(cell, w) + " |")
			}
		}
		if i < len(rows)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// convertTablesToCodeBlocks wraps Markdown table blocks in ``` for nicer Discord rendering,
// and re-aligns columns so | characters line up.
func convertTablesToCodeBlocks(text string) string {
	return tableBlockRe.ReplaceAllStringFunc(text, func(m string) string {
		return "```\n" + alignTable(m) + "\n```\n"
	})
}

// splitForDiscord converts tables and splits text into ≤ discordMaxLen chunks.
// It never splits inside a ``` code block.
func splitForDiscord(text string) []string {
	text = convertTablesToCodeBlocks(text)
	if len(text) <= discordMaxLen {
		return []string{text}
	}
	var chunks []string
	lines := strings.Split(text, "\n")
	var buf strings.Builder
	inCode := false
	flush := func() {
		s := strings.TrimRight(buf.String(), "\n")
		if s != "" {
			chunks = append(chunks, s)
		}
		buf.Reset()
	}
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inCode = !inCode
		}
		add := line + "\n"
		if !inCode && buf.Len() > 0 && buf.Len()+len(add) > discordMaxLen {
			flush()
		}
		buf.WriteString(add)
	}
	flush()
	return chunks
}


type discordPending struct {
	MessageID string
	ChannelID string
	GuildID   string
	AutoReply bool
}

// discordSession is one per Discord channel; all sessions use ACP subprocess mode.
type discordSession struct {
	channelID   string
	runtime     AgentRuntime
	sessionUUID string
	acpProcess  *ACPProcess // lazy-started on first Prompt()
	imgStore    *imageStore
	workdir     string

	mu   sync.Mutex
	last *discordPending
}

func newDiscordSession(runtime AgentRuntime, channelID, workdir string, imgStore *imageStore, mcpServers []map[string]any, logger *slog.Logger) *discordSession {
	proc := NewACPProcess(runtime.ACPExecutable, runtime.ACPArgs, workdir, logger)
	if runtime.SupportsMCP && len(mcpServers) > 0 {
		asAny := make([]any, len(mcpServers))
		for i, s := range mcpServers {
			asAny[i] = s
		}
		proc.SetMCPServers(asAny)
	}
	return &discordSession{
		channelID:  channelID,
		runtime:    runtime,
		acpProcess: proc,
		imgStore:   imgStore,
		workdir:    workdir,
	}
}

// SessionView is the JSON representation of a live Discord session.
type SessionView struct {
	ChannelID   string `json:"channel_id"`
	SessionUUID string `json:"session_uuid"`
}

// DiscordSessionManager listens on Discord and routes each channel to its own ACP session.
type DiscordSessionManager struct {
	runtime          AgentRuntime
	token            string
	allowedChannelID string
	allowedDMUserIDs map[string]struct{} // nil/empty = DM disabled
	logger           *slog.Logger
	workdir          string
	settings         *SettingsManager // optional; nil = use built-in defaults
	imgStore         *imageStore
	mcpServersFor    func(rt AgentRuntime, userID, conversationID string) []map[string]any

	mu             sync.Mutex
	dgo            *discordgo.Session
	sessions       map[string]*discordSession // channelID → session
	channelPrivate map[string]bool            // channelID → isPrivate, cached
}

func newDiscordSessionManager(runtime AgentRuntime, token, channelID string, allowedDMUsers []string, workdir string, logger *slog.Logger) *DiscordSessionManager {
	dmIDs := make(map[string]struct{}, len(allowedDMUsers))
	for _, id := range allowedDMUsers {
		dmIDs[id] = struct{}{}
	}
	return &DiscordSessionManager{
		runtime:          runtime,
		token:            token,
		allowedChannelID: channelID,
		allowedDMUserIDs: dmIDs,
		logger:           logger,
		workdir:          workdir,
		sessions:         make(map[string]*discordSession),
		channelPrivate:   make(map[string]bool),
		imgStore:         newImageStore(workdir, logger),
	}
}

// SetSettings wires the settings manager so the adapter can read attachment
// limits at request time. Safe to call before or after Start.
func (d *DiscordSessionManager) SetSettings(sm *SettingsManager) {
	d.settings = sm
}

// SetMCPServers wires the per-channel MCP server descriptor builder. Each new
// Discord channel session will receive these descriptors before the ACP
// subprocess starts, giving Claude access to perch's self-hosted MCP tools
// (schedule_message, list_schedules, cancel_schedule). Safe to call before or
// after Start; only affects sessions created afterwards.
func (d *DiscordSessionManager) SetMCPServers(fn func(rt AgentRuntime, userID, conversationID string) []map[string]any) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.mcpServersFor = fn
}

// attachmentLimits returns the effective limits (or built-in defaults if no
// settings manager is wired).
func (d *DiscordSessionManager) attachmentLimits() AttachmentLimits {
	if d.settings == nil {
		return EffectiveAttachmentLimits(nil)
	}
	return EffectiveAttachmentLimits(d.settings.GetEffective().Chat)
}

// isPrivateChannel returns true if the channel is not visible to @everyone.
// The result is cached so the Discord API is called at most once per channel.
func (d *DiscordSessionManager) isPrivateChannel(s *discordgo.Session, channelID string) bool {
	d.mu.Lock()
	if v, ok := d.channelPrivate[channelID]; ok {
		d.mu.Unlock()
		return v
	}
	d.mu.Unlock()

	ch, err := s.Channel(channelID)
	if err != nil {
		d.logger.Warn("Discord isPrivateChannel: channel lookup failed", "channel", channelID, "err", err)
		return false // treat as public on error
	}
	isPrivate := false
	for _, ow := range ch.PermissionOverwrites {
		// @everyone role has the same ID as the guild
		if ow.ID == ch.GuildID && ow.Type == discordgo.PermissionOverwriteTypeRole {
			if ow.Deny&discordgo.PermissionViewChannel != 0 {
				isPrivate = true
			}
			break
		}
	}

	d.mu.Lock()
	d.channelPrivate[channelID] = isPrivate
	d.mu.Unlock()
	return isPrivate
}

func (d *DiscordSessionManager) Start(_ IMConfig) error {
	session, err := discordgo.New("Bot " + d.token)
	if err != nil {
		return err
	}
	session.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages | discordgo.IntentsMessageContent
	session.AddHandler(d.onMessage)
	if err := session.Open(); err != nil {
		return err
	}
	d.mu.Lock()
	d.dgo = session
	d.mu.Unlock()
	d.logger.Info("Discord bot connected (ACP mode)")
	return nil
}

func (d *DiscordSessionManager) Stop() {
	d.mu.Lock()
	s := d.dgo
	sessions := make([]*discordSession, 0, len(d.sessions))
	for _, sess := range d.sessions {
		sessions = append(sessions, sess)
	}
	d.mu.Unlock()

	if s != nil {
		s.Close()
	}
	for _, sess := range sessions {
		if sess.acpProcess != nil {
			sess.acpProcess.Stop()
		}
	}
}

func (d *DiscordSessionManager) onMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	d.logger.Debug("Discord onMessage received", "channel", m.ChannelID, "guild", m.GuildID, "author", m.Author.Username, "content", m.Content, "mentions", len(m.Mentions))
	if m.Author == nil || m.Author.Bot {
		return
	}
	if d.allowedChannelID != "" && m.ChannelID != d.allowedChannelID {
		return
	}

	isDM := m.GuildID == ""
	if isDM {
		// DM is deny-by-default: only respond if the sender is in the allowlist.
		if _, ok := d.allowedDMUserIDs[m.Author.ID]; !ok {
			return
		}
	}
	isPrivate := !isDM && d.isPrivateChannel(s, m.ChannelID)

	var botID string
	if s.State.User != nil {
		botID = s.State.User.ID
	}
	d.logger.Debug("Discord onMessage routing", "isDM", isDM, "isPrivate", isPrivate, "botID", botID, "mentions", len(m.Mentions))

	content := m.Content
	if !isDM && !isPrivate {
		// Public Guild channel: require @mention.
		// Discord may not populate m.Mentions for bots without Message Content
		// Intent fully active, so also check the raw content for <@BOTID> or
		// <@!BOTID> tokens as a reliable fallback.
		mentioned := false
		for _, user := range m.Mentions {
			if s.State.User != nil && user.ID == s.State.User.ID {
				mentioned = true
				break
			}
		}
		if !mentioned && botID != "" {
			mentioned = strings.Contains(content, "<@"+botID+">") ||
				strings.Contains(content, "<@!"+botID+">")
		}
		d.logger.Debug("Discord onMessage public channel check", "mentioned", mentioned, "stateUser_nil", s.State.User == nil, "botID", botID)
		if !mentioned {
			return
		}
		// Strip all mention prefixes (e.g. "<@1234567890> ")
		content = strings.TrimSpace(mentionRe.ReplaceAllString(content, ""))
		if content == "" {
			return
		}
	}

	sess := d.getOrCreateSession(m.ChannelID)
	imageBlocks, persisted, fetchFailed, disallowed := d.processAttachments(m.ChannelID, m.Attachments)
	go sess.handleWithACP(s, m.ChannelID, m.ID, content, imageBlocks, persisted, fetchFailed, disallowed, d.logger)
}

// disallowedAttachment captures a Discord attachment whose ContentType is
// not in the chat allow-list, for surfacing back to the user.
type disallowedAttachment struct {
	Filename string
	Mime     string
}

// processAttachments downloads inbound Discord attachments and routes them
// per the chat allow-list:
//   - images stay in memory and become ACP image content blocks
//   - non-image MIMEs are persisted under <workdir>/uploads/<channelID>/
//     and returned as PersistedAttachment for the prompt prefix
//   - disallowed MIMEs are dropped and reported back to the user
func (d *DiscordSessionManager) processAttachments(channelID string, atts []*discordgo.MessageAttachment) ([]ACPContent, []PersistedAttachment, []string, []disallowedAttachment) {
	if len(atts) == 0 {
		return nil, nil, nil, nil
	}
	lim := d.attachmentLimits()
	allow := map[string]bool{}
	for _, m := range lim.AllowedMime {
		allow[m] = true
	}

	var imageBlocks []ACPContent
	var persisted []PersistedAttachment
	var failed []string
	var disallowed []disallowedAttachment
	totalKept := 0
	for _, a := range atts {
		if a == nil {
			continue
		}
		if !allow[a.ContentType] {
			d.logger.Info("Discord ACP: drop attachment with disallowed MIME", "filename", a.Filename, "ct", a.ContentType)
			disallowed = append(disallowed, disallowedAttachment{Filename: a.Filename, Mime: a.ContentType})
			continue
		}
		if int64(a.Size) > lim.MaxBytes {
			d.logger.Warn("Discord ACP: drop oversized attachment", "filename", a.Filename, "size", a.Size, "limit", lim.MaxBytes)
			continue
		}
		if lim.MaxFiles > 0 && totalKept >= lim.MaxFiles {
			d.logger.Warn("Discord ACP: drop attachment over max-files", "filename", a.Filename, "limit", lim.MaxFiles)
			continue
		}
		data, err := fetchURLBytes(a.URL, lim.MaxBytes+1)
		if err != nil {
			d.logger.Warn("Discord ACP: fetch attachment failed", "filename", a.Filename, "err", err)
			failed = append(failed, a.Filename)
			continue
		}
		if int64(len(data)) > lim.MaxBytes {
			d.logger.Warn("Discord ACP: drop oversized attachment after fetch", "filename", a.Filename, "size", len(data), "limit", lim.MaxBytes)
			continue
		}

		if IsImageMIME(a.ContentType) {
			got := MagicMime(data)
			if got != a.ContentType {
				d.logger.Warn("Discord ACP: drop image with mime/magic mismatch", "filename", a.Filename, "claimed", a.ContentType, "magic", got)
				continue
			}
			imageBlocks = append(imageBlocks, ACPContent{
				Type:     "image",
				Data:     base64.StdEncoding.EncodeToString(data),
				MimeType: got,
			})
			totalKept++
			continue
		}

		// Non-image: validate (PDF magic or text heuristic) then persist to disk.
		if a.ContentType == "application/pdf" {
			if MagicMime(data) != "application/pdf" {
				d.logger.Warn("Discord ACP: drop PDF with magic mismatch", "filename", a.Filename)
				continue
			}
		} else if textMimeSet[a.ContentType] {
			if !looksLikeText(data) {
				d.logger.Warn("Discord ACP: drop text attachment with binary content", "filename", a.Filename, "ct", a.ContentType)
				continue
			}
		}

		// Build a one-shot []Attachment to reuse the disk-write rollback logic.
		one := []Attachment{{
			Filename:   a.Filename,
			MimeType:   a.ContentType,
			DataBase64: base64.StdEncoding.EncodeToString(data),
		}}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		written, err := WriteAttachmentsToDisk(ctx, d.workdir, channelID, one, lim)
		cancel()
		if err != nil {
			d.logger.Warn("Discord ACP: persist attachment failed", "filename", a.Filename, "err", err)
			failed = append(failed, a.Filename)
			continue
		}
		persisted = append(persisted, written...)
		totalKept++
	}
	if len(imageBlocks) > 0 || len(persisted) > 0 || len(failed) > 0 || len(disallowed) > 0 {
		d.logger.Info("Discord ACP: attachments processed",
			"images", len(imageBlocks),
			"files", len(persisted),
			"failed", len(failed),
			"disallowed", len(disallowed),
		)
	}
	return imageBlocks, persisted, failed, disallowed
}

// fetchURLBytes performs a GET on url with a hard byte limit (response body
// truncated/aborted at maxBytes+1 to detect oversized payloads cheaply).
func fetchURLBytes(url string, maxBytes int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBytes))
}

// handleWithACP sends message to the claude-agent-acp subprocess and routes the reply to Discord.
// It runs in its own goroutine and never returns an error (logs instead).
// imageBlocks are pre-fetched image content blocks; persisted are non-image
// attachments already written to disk and referenced via the prompt prefix;
// fetchFailed and disallowed list filenames whose handling needs to surface
// to the user in the reply.
func (sess *discordSession) handleWithACP(s *discordgo.Session, channelID, messageID, content string, imageBlocks []ACPContent, persisted []PersistedAttachment, fetchFailed []string, disallowed []disallowedAttachment, logger *slog.Logger) {
	// One message at a time per session — drop concurrent duplicates.
	sess.mu.Lock()
	if sess.last != nil {
		sess.mu.Unlock()
		logger.Debug("Discord ACP: session busy, dropping duplicate", "channel", channelID, "msgID", messageID)
		return
	}
	sess.last = &discordPending{MessageID: messageID, ChannelID: channelID}
	sess.mu.Unlock()
	defer func() {
		sess.mu.Lock()
		if sess.last != nil && sess.last.MessageID == messageID {
			sess.last = nil
		}
		sess.mu.Unlock()
	}()

	if err := s.MessageReactionAdd(channelID, messageID, emojiEyes); err != nil {
		logger.Warn("Discord ACP: add 👀 reaction failed", "err", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), acpRunTimeout())
	defer cancel()

	// Ensure the subprocess is running (lazy start / auto-recovery after crash).
	if err := sess.acpProcess.EnsureRunning(ctx); err != nil {
		logger.Warn("Discord ACP: process start failed", "channel", channelID, "err", err)
		_ = s.MessageReactionRemove(channelID, messageID, emojiEyes, "@me")
		_ = s.MessageReactionAdd(channelID, messageID, emojiCross)
		_, _ = s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
			Content:   "❌ Agent unavailable: " + err.Error(),
			Reference: &discordgo.MessageReference{MessageID: messageID},
		})
		return
	}

	// Build the text block: file prefixes (when non-image attachments were
	// persisted to disk) + the user's message.
	textBody := BuildPromptFilePrefix(persisted, content)
	blocks := make([]ACPContent, 0, 1+len(imageBlocks))
	if textBody != "" {
		blocks = append(blocks, ACPContent{Type: "text", Text: textBody})
	} else if len(imageBlocks) > 0 {
		// ACP requires at least one block; keep an empty text block as the
		// implicit "describe these images" prompt so claude-agent-acp accepts the call.
		blocks = append(blocks, ACPContent{Type: "text", Text: ""})
	}
	blocks = append(blocks, imageBlocks...)
	discordSessionStart := time.Now()
	text, err := sess.acpProcess.PromptWithContent(ctx, blocks, nil, nil, nil)
	_ = s.MessageReactionRemove(channelID, messageID, emojiEyes, "@me")

	if err != nil {
		logger.Warn("Discord ACP: prompt failed", "channel", channelID, "err", err)
		_ = s.MessageReactionAdd(channelID, messageID, emojiCross)
		errMsg := err.Error()
		if ctx.Err() == context.DeadlineExceeded {
			errMsg = "⏱️ Agent timed out."
		}
		_, _ = s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
			Content:   "❌ " + errMsg,
			Reference: &discordgo.MessageReference{MessageID: messageID},
		})
		return
	}

	// Extract [image: ...] tokens from the response and store images.
	text, imgAttachments := extractImages(text, sess.imgStore, sess.workdir, channelID)
	if sess.imgStore != nil {
		already := make(map[string]bool, len(imgAttachments))
		for _, a := range imgAttachments {
			already[a.Caption] = true
		}
		extra := collectPlaywrightScreenshots(discordSessionStart, sess.imgStore, channelID, logger, already)
		imgAttachments = append(imgAttachments, extra...)
	}

	_ = s.MessageReactionAdd(channelID, messageID, emojiSpeech)
	if len(fetchFailed) > 0 {
		text = text + "\n\n> 附件 " + strings.Join(fetchFailed, ", ") + " 下載失敗，未送進 Claude"
	}
	for _, da := range disallowed {
		text = text + "\n\n> 附件 " + da.Filename + " 不支援此類型 (" + da.Mime + ")"
	}
	if text == "" && len(imgAttachments) == 0 {
		return // nothing to send
	}
	chunks := splitForDiscord(text)
	logger.Info("Discord ACP: sending reply", "channel", channelID, "replyTo", messageID, "chunks", len(chunks), "images", len(imgAttachments))

	// Build Discord file list from image attachments.
	var discordFiles []*discordgo.File
	var oversizeNote string
	for _, att := range imgAttachments {
		// att.URL is "/api/images/<convID>/<filename>"; derive abs path.
		parts := strings.Split(strings.TrimPrefix(att.URL, "/api/images/"), "/")
		if len(parts) != 2 {
			continue
		}
		absPath := sess.imgStore.AbsPath(parts[0], parts[1])
		fi, err := os.Stat(absPath)
		if err != nil {
			logger.Warn("Discord ACP: image stat failed", "path", absPath, "err", err)
			continue
		}
		if fi.Size() > maxImageBytes {
			oversizeNote += "\n(圖片過大，無法傳送至 Discord: " + att.Caption + ")"
			continue
		}
		f, err := os.Open(absPath)
		if err != nil {
			logger.Warn("Discord ACP: image open failed", "path", absPath, "err", err)
			continue
		}
		discordFiles = append(discordFiles, &discordgo.File{Name: att.Caption, Reader: f})
	}
	if oversizeNote != "" {
		if len(chunks) > 0 {
			chunks[len(chunks)-1] += oversizeNote
		} else {
			chunks = append(chunks, oversizeNote)
		}
	}

	for i, chunk := range chunks {
		var files []*discordgo.File
		if i == len(chunks)-1 {
			files = discordFiles // attach images only on the last chunk
		}
		send := &discordgo.MessageSend{Content: chunk, Files: files}
		if i == 0 {
			send.Reference = &discordgo.MessageReference{MessageID: messageID}
		}
		if _, err := s.ChannelMessageSendComplex(channelID, send); err != nil {
			logger.Warn("Discord ACP: send reply failed", "channel", channelID, "part", i, "err", err)
		}
	}
	// If there was no text at all, send images-only reply.
	if len(chunks) == 0 && len(discordFiles) > 0 {
		send := &discordgo.MessageSend{
			Files:     discordFiles,
			Reference: &discordgo.MessageReference{MessageID: messageID},
		}
		if _, err := s.ChannelMessageSendComplex(channelID, send); err != nil {
			logger.Warn("Discord ACP: send images-only reply failed", "channel", channelID, "err", err)
		}
	}
	// Close file handles after send.
	for _, df := range discordFiles {
		if c, ok := df.Reader.(interface{ Close() error }); ok {
			_ = c.Close()
		}
	}
}

func (d *DiscordSessionManager) getOrCreateSession(channelID string) *discordSession {
	d.mu.Lock()
	defer d.mu.Unlock()
	if sess, ok := d.sessions[channelID]; ok {
		return sess
	}
	var mcp []map[string]any
	if d.mcpServersFor != nil {
		// userID is a stable identifier for Discord traffic; convID is the
		// channel ID so per-conversation MCP state (schedules, etc.) is keyed
		// to the channel.
		mcp = d.mcpServersFor(d.runtime, "discord", channelID)
	}
	sess := newDiscordSession(d.runtime, channelID, d.workdir, d.imgStore, mcp, d.logger)
	d.sessions[channelID] = sess
	return sess
}

// ListSessions is a no-op since all Discord sessions are ACP (no viewable PTY stream).
func (d *DiscordSessionManager) ListSessions() []SessionView {
	return nil
}

// SubscribeSession always returns false — Discord sessions use ACP, not PTY.
func (d *DiscordSessionManager) SubscribeSession(_ string) (<-chan []byte, func(), bool) {
	return nil, nil, false
}

// DispatchScheduled handles a scheduler-fired job for a Discord channel target.
// Posts a visible header then routes the prompt via the channel's ACP session.
// Returns true when handled, false if the target is not a Discord channel.
func (d *DiscordSessionManager) DispatchScheduled(target, message string) bool {
	channelID, ok := parseDiscordTarget(target)
	if !ok {
		return false
	}

	d.mu.Lock()
	dgo := d.dgo
	d.mu.Unlock()
	if dgo == nil {
		return false
	}

	sent, err := dgo.ChannelMessageSend(channelID, "📅 local schedule > "+message)
	if err != nil {
		d.logger.Warn("Discord schedule header send failed", "channel", channelID, "err", err)
		return false
	}

	sess := d.getOrCreateSession(channelID)
	go sess.handleWithACP(dgo, channelID, sent.ID, message, nil, nil, nil, nil, d.logger)
	return true
}

// parseDiscordTarget extracts the channel ID from a scheduler target string.
// Accepts both the canonical "discord:<channelID>" form and the looser
// "discord:channel:<channelID>" form that mirrors Discord's <#channelID>
// mention syntax (T31/T32: tolerant parsing prevents 400 "not snowflake").
func parseDiscordTarget(target string) (string, bool) {
	const prefix = "discord:"
	if !strings.HasPrefix(target, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(target[len(prefix):], "channel:")
	if rest == "" {
		return "", false
	}
	return rest, true
}

// ResizeSession is a no-op (Discord sessions use ACP, not PTY).
func (d *DiscordSessionManager) ResizeSession(_ string, _, _ uint16) {}

// SendToChannel sends a plain-text message to a Discord channel by ID.
// It is independent of allowedChannelID and intended for out-of-band notifications.
func (d *DiscordSessionManager) SendToChannel(channelID string, msg string) error {
	d.mu.Lock()
	dgo := d.dgo
	d.mu.Unlock()
	if dgo == nil {
		return fmt.Errorf("discord session not ready")
	}
	_, err := dgo.ChannelMessageSend(channelID, msg)
	return err
}

// WriteSession always returns an error — Discord sessions use ACP, not PTY.
func (d *DiscordSessionManager) WriteSession(channelID string, _ []byte) error {
	d.mu.Lock()
	_, ok := d.sessions[channelID]
	d.mu.Unlock()
	if !ok {
		return fmt.Errorf("session not found: %s", channelID)
	}
	return fmt.Errorf("session %s uses ACP (no writable PTY)", channelID)
}
