## 1. Store — History Query

- [x] 1.1 Add `ConversationTurn` struct `{Query, Response string}` to `store.go`
- [x] 1.2 Implement `Store.GetRecentHistory(userID string, since int64, limit int) ([]ConversationTurn, error)` — `SELECT query,response FROM query_sessions WHERE user_id=? AND status='done' AND started_at>=? ORDER BY started_at ASC LIMIT ?`
- [x] 1.3 Add unit test for `GetRecentHistory`: returns turns within window, excludes older rows, respects limit cap

## 2. Server — History Injection into Agent Query

- [x] 2.1 Implement `buildPrompt(history []ConversationTurn, query string) string` — formats history as `<conversation_history>…</conversation_history>\n\n<query>` prefix; returns plain query when history is empty
- [x] 2.2 Define constant `conversationWindow = 24 * time.Hour` and `conversationMaxTurns = 20` in `user_session.go`
- [x] 2.3 Update `StartSession` to accept `newConversation bool`; when `false`, call `store.GetRecentHistory(userID, now-conversationWindow, conversationMaxTurns)` and pass result to `buildPrompt`; when `true`, skip history lookup
- [x] 2.4 Add unit test for `buildPrompt`: empty history returns raw query; N turns produce correct prefix format

## 3. Server — API Changes

- [x] 3.1 Extend the `POST /api/chat` request struct with `NewConversation bool \`json:"new_conversation,omitempty"\``
- [x] 3.2 Pass `req.NewConversation` from `handleChatAPI` to `StartSession`; `userID` comes from auth context — no other changes to the request
- [x] 3.3 Verify Discord (`im_discord.go`), Telegram (`im_telegram.go`), and scheduler (`scheduler.go`) call sites pass `newConversation: false`

## 4. Server — Tests

- [x] 4.1 Integration-test `handleChatAPI`: second request from same user within 24h receives history-prefixed prompt
- [x] 4.2 Integration-test `handleChatAPI`: request with `new_conversation: true` skips history

## 5. Frontend — Conversation State

- [x] 5.1 Replace `submittedQuery`/`markdownBuf` with `messages: {role: 'user'|'assistant', content: string}[]` state array
- [x] 5.2 Add `newConversation` boolean flag in state (default `false`); set `true` when user clicks "New conversation"
- [x] 5.3 On submit: append user message to `messages`, include `new_conversation` in POST body, reset flag to `false` after sending
- [x] 5.4 On streaming `pty` events: update the last assistant message's `content` in-place (streamed output)
- [x] 5.5 On `done` event: mark last assistant message as complete

## 6. Frontend — Thread UI

- [x] 6.1 Render `messages` as a vertically scrolled thread — user bubbles right-aligned, assistant bubbles left-aligned (reuse existing bubble styles)
- [x] 6.2 Auto-scroll to bottom after each new message or content update
- [x] 6.3 Add "New conversation" button in the input bar; clicking it clears `messages` to `[]` and sets `newConversation` flag
- [x] 6.4 Show "Thinking…" spinner only in the last (pending) assistant message slot

## 7. Verification

- [x] 7.1 Manual test: ask two follow-up questions in the web UI and confirm the agent references prior context
- [x] 7.2 Manual test: "New conversation" clears thread; next query has no prior context
- [x] 7.3 Manual test: Discord-triggered query is unaffected
- [x] 7.4 Run full test suite (`go test ./...`) — all existing tests pass
