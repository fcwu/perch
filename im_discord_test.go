package main

import (
	"log/slog"
	"regexp"
	"strings"
	"testing"
)

var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func TestAlignTable(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "basic ascii",
			input: "| Col1 | Col2 |\n" +
				"| --- | --- |\n" +
				"| short | a longer value |\n",
			want: "| Col1  | Col2           |\n" +
				"| ----- | -------------- |\n" +
				"| short | a longer value |",
		},
		{
			name: "cjk wide chars",
			input: "| 欄位 | 說明 |\n" +
				"| --- | --- |\n" +
				"| 技術 | 分散式系統 |\n",
			want: "| 欄位 | 說明       |\n" +
				"| ---- | ---------- |\n" +
				"| 技術 | 分散式系統 |",
		},
		{
			name: "mixed ascii and cjk",
			input: "| Name | 說明 | Count |\n" +
				"| --- | --- | --- |\n" +
				"| foo | 很長的說明文字 | 42 |\n",
			want: "| Name | 說明           | Count |\n" +
				"| ---- | -------------- | ----- |\n" +
				"| foo  | 很長的說明文字 | 42    |",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := alignTable(tc.input)
			if got != tc.want {
				t.Errorf("alignTable mismatch:\nwant:\n%s\n\ngot:\n%s", tc.want, got)
			}
		})
	}
}

func TestSplitForDiscordNoSplit(t *testing.T) {
	text := "short message"
	chunks := splitForDiscord(text)
	if len(chunks) != 1 || chunks[0] != text {
		t.Fatalf("expected single unchanged chunk, got %v", chunks)
	}
}

func TestSplitForDiscordTableWrapped(t *testing.T) {
	table := "| A | B |\n| --- | --- |\n| 1 | 2 |\n"
	chunks := splitForDiscord(table)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if !strings.HasPrefix(chunks[0], "```") {
		t.Errorf("table not wrapped in code block: %q", chunks[0])
	}
}

func TestSplitForDiscordLongText(t *testing.T) {
	// Build a text > discordMaxLen with clear paragraph breaks.
	para := strings.Repeat("x", 500) + "\n\n"
	text := para + para + para + para + para // 5 × ~502 = ~2510 chars
	chunks := splitForDiscord(text)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks for long text, got %d", len(chunks))
	}
	for i, c := range chunks {
		if len(c) > discordMaxLen {
			t.Errorf("chunk %d exceeds discordMaxLen (%d): len=%d", i, discordMaxLen, len(c))
		}
	}
}

func TestSplitForDiscordNoSplitInsideCodeBlock(t *testing.T) {
	// A code block under discordMaxLen should never be split.
	code := "```\n" + strings.Repeat("line\n", 50) + "```"
	if len(code) > discordMaxLen {
		t.Skip("test code block is unexpectedly large")
	}
	chunks := splitForDiscord(code)
	if len(chunks) != 1 {
		t.Errorf("code block split unexpectedly into %d chunks", len(chunks))
	}
}

// TestParseDiscordTarget guards T31/T32: the scheduler accepts both the
// canonical "discord:<channelID>" target and the looser
// "discord:channel:<channelID>" form (mirrors <#channelID> mention syntax),
// since the latter slipped through the schedule-add flow and produced a 400
// "not snowflake" from the Discord REST API.
func TestParseDiscordTarget(t *testing.T) {
	cases := []struct {
		in     string
		wantID string
		wantOK bool
	}{
		{"discord:1496278101166915694", "1496278101166915694", true},
		{"discord:channel:1496278101166915694", "1496278101166915694", true},
		{"discord:", "", false},
		{"discord:channel:", "", false},
		{"telegram:42", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		gotID, gotOK := parseDiscordTarget(c.in)
		if gotID != c.wantID || gotOK != c.wantOK {
			t.Errorf("parseDiscordTarget(%q) = (%q,%v), want (%q,%v)", c.in, gotID, gotOK, c.wantID, c.wantOK)
		}
	}
}

func TestChannelSessionIDFormat(t *testing.T) {
	id := channelSessionID("123456789")
	if !uuidRE.MatchString(id) {
		t.Fatalf("not a UUID: %q", id)
	}
}

func TestChannelSessionIDDeterministic(t *testing.T) {
	a := channelSessionID("abc")
	b := channelSessionID("abc")
	if a != b {
		t.Fatalf("not deterministic: %q vs %q", a, b)
	}
}

func TestChannelSessionIDUnique(t *testing.T) {
	if channelSessionID("111") == channelSessionID("222") {
		t.Fatal("different channels got the same UUID")
	}
}

func TestWriteSessionNotFound(t *testing.T) {
	mgr := newDiscordSessionManager(AgentRuntime{}, "tok", "", nil, t.TempDir(), slog.Default())
	err := mgr.WriteSession("nonexistent", []byte("hello"))
	if err == nil {
		t.Fatal("expected error for missing session, got nil")
	}
}

func TestWriteSessionExistsACP(t *testing.T) {
	mgr := newDiscordSessionManager(AgentRuntime{}, "tok", "", nil, t.TempDir(), slog.Default())
	mgr.mu.Lock()
	mgr.sessions["ch1"] = &discordSession{channelID: "ch1", acpProcess: NewACPProcess("", "", slog.Default())}
	mgr.mu.Unlock()
	// ACP session always returns an error (no writable PTY).
	err := mgr.WriteSession("ch1", []byte("hello"))
	if err == nil {
		t.Fatal("expected error for ACP session, got nil")
	}
	if err.Error() == "session not found: ch1" {
		t.Fatalf("got session-not-found but session was inserted: %v", err)
	}
}
