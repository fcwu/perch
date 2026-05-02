package main

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"strings"
	"unicode"
)

// newUUID returns a random UUID (version 4, RFC 4122).
func newUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// ConversationRow is a row returned by the conversation list endpoints.
type ConversationRow struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id,omitempty"`
	Title     string `json:"title"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
	Pinned    bool   `json:"pinned"`
	PinnedAt  *int64 `json:"pinned_at,omitempty"`
	Runtime   string `json:"runtime,omitempty"`
	Model     string `json:"model,omitempty"`
}

// truncateTitle returns the first maxRunes runes of s, truncating at a word boundary
// if possible. Trailing whitespace is trimmed.
func truncateTitle(s string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	// Try to find the last word boundary within maxRunes.
	cut := maxRunes
	for cut > 0 && !unicode.IsSpace(runes[cut-1]) {
		cut--
	}
	if cut == 0 {
		// No word boundary found; hard-cut at maxRunes.
		cut = maxRunes
	}
	return strings.TrimRight(string(runes[:cut]), " \t\n\r")
}

// InsertConversation inserts a new conversation row with the given runtime/model.
// title is truncated to ~60 chars at a word boundary if possible.
func (s *Store) InsertConversation(id, userID, title, runtime, model string) error {
	title = truncateTitle(title, 60)
	now := nowMs()
	_, err := s.db.Exec(
		`INSERT INTO conversations(id,user_id,title,created_at,updated_at,runtime,model) VALUES(?,?,?,?,?,?,?)`,
		id, userID, title, now, now, runtime, model,
	)
	return err
}

func scanConversationRow(scanner interface {
	Scan(...any) error
}, includeUserID bool) (ConversationRow, error) {
	var c ConversationRow
	var pinnedInt int
	var pinnedAt sql.NullInt64
	var runtime, model sql.NullString
	if includeUserID {
		if err := scanner.Scan(&c.ID, &c.UserID, &c.Title, &c.CreatedAt, &c.UpdatedAt, &pinnedInt, &pinnedAt, &runtime, &model); err != nil {
			return c, err
		}
	} else {
		if err := scanner.Scan(&c.ID, &c.Title, &c.CreatedAt, &c.UpdatedAt, &pinnedInt, &pinnedAt, &runtime, &model); err != nil {
			return c, err
		}
	}
	c.Pinned = pinnedInt != 0
	if pinnedAt.Valid {
		v := pinnedAt.Int64
		c.PinnedAt = &v
	}
	if runtime.Valid {
		c.Runtime = runtime.String
	}
	if model.Valid {
		c.Model = model.String
	}
	return c, nil
}

// ListConversationsPage returns the user's conversations split into pinned and
// recent groups. Pinned rows are returned only on the first page (before==0).
// recent holds non-pinned rows with updated_at < before, capped at limit and
// ordered by updated_at DESC. nextBefore is the smallest updated_at in recent
// (or 0 if recent is short of limit). The two-query form keeps cursor logic
// linear and avoids re-emitting pinned rows on subsequent Load More calls.
func (s *Store) ListConversationsPage(userID string, before int64, limit int) (pinned, recent []ConversationRow, nextBefore int64, err error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	if before <= 0 {
		pinRows, perr := s.db.Query(
			`SELECT id,title,created_at,updated_at,pinned,pinned_at,runtime,model FROM conversations
			 WHERE user_id=? AND pinned=1 ORDER BY pinned_at DESC`,
			userID,
		)
		if perr != nil {
			return nil, nil, 0, perr
		}
		for pinRows.Next() {
			c, scerr := scanConversationRow(pinRows, false)
			if scerr != nil {
				pinRows.Close()
				return nil, nil, 0, scerr
			}
			pinned = append(pinned, c)
		}
		pinRows.Close()
		if rerr := pinRows.Err(); rerr != nil {
			return nil, nil, 0, rerr
		}
	}

	var rows *sql.Rows
	if before > 0 {
		rows, err = s.db.Query(
			`SELECT id,title,created_at,updated_at,pinned,pinned_at,runtime,model FROM conversations
			 WHERE user_id=? AND pinned=0 AND updated_at < ?
			 ORDER BY updated_at DESC LIMIT ?`,
			userID, before, limit,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT id,title,created_at,updated_at,pinned,pinned_at,runtime,model FROM conversations
			 WHERE user_id=? AND pinned=0
			 ORDER BY updated_at DESC LIMIT ?`,
			userID, limit,
		)
	}
	if err != nil {
		return nil, nil, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		c, scerr := scanConversationRow(rows, false)
		if scerr != nil {
			return nil, nil, 0, scerr
		}
		recent = append(recent, c)
	}
	if rerr := rows.Err(); rerr != nil {
		return nil, nil, 0, rerr
	}
	if len(recent) == limit {
		nextBefore = recent[len(recent)-1].UpdatedAt
	}
	return pinned, recent, nextBefore, nil
}

// ListConversations is retained as a thin compatibility wrapper that returns
// the merged pinned+recent slice for callers that don't need the page split.
func (s *Store) ListConversations(userID string) ([]ConversationRow, error) {
	pinned, recent, _, err := s.ListConversationsPage(userID, 0, 50)
	if err != nil {
		return nil, err
	}
	return append(pinned, recent...), nil
}

// GetConversation returns the conversation row by id, scoped to userID. When
// userID is empty, the lookup is unscoped (admin path).
func (s *Store) GetConversation(id, userID string) (*ConversationRow, error) {
	var row *sql.Row
	if userID == "" {
		row = s.db.QueryRow(
			`SELECT id,user_id,title,created_at,updated_at,pinned,pinned_at,runtime,model FROM conversations WHERE id=?`,
			id,
		)
	} else {
		row = s.db.QueryRow(
			`SELECT id,user_id,title,created_at,updated_at,pinned,pinned_at,runtime,model FROM conversations WHERE id=? AND user_id=?`,
			id, userID,
		)
	}
	c, err := scanConversationRow(row, true)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// UpdateConversationPin toggles pinned/pinned_at on the user's conversation.
// Returns (false, nil) if the row does not exist or belongs to another user.
func (s *Store) UpdateConversationPin(id, userID string, pinned bool) (bool, error) {
	now := nowMs()
	var res sql.Result
	var err error
	if pinned {
		res, err = s.db.Exec(
			`UPDATE conversations SET pinned=1, pinned_at=? WHERE id=? AND user_id=?`,
			now, id, userID,
		)
	} else {
		res, err = s.db.Exec(
			`UPDATE conversations SET pinned=0, pinned_at=NULL WHERE id=? AND user_id=?`,
			id, userID,
		)
	}
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// UpdateConversationRuntime updates runtime and/or model on the user's
// conversation. Empty strings leave the corresponding column untouched.
// Returns (false, nil) if no row matched.
func (s *Store) UpdateConversationRuntime(id, userID, runtime, model string) (bool, error) {
	if runtime == "" && model == "" {
		return false, nil
	}
	sets := []string{}
	args := []any{}
	if runtime != "" {
		sets = append(sets, "runtime=?")
		args = append(args, runtime)
	}
	if model != "" {
		sets = append(sets, "model=?")
		args = append(args, model)
	}
	args = append(args, id, userID)
	q := "UPDATE conversations SET " + strings.Join(sets, ", ") + " WHERE id=? AND user_id=?"
	res, err := s.db.Exec(q, args...)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// BackfillConversationRuntime sets runtime and model on the row if either is
// currently NULL. Used for write-once lazy backfill on legacy rows.
func (s *Store) BackfillConversationRuntime(id, runtime, model string) error {
	_, err := s.db.Exec(
		`UPDATE conversations SET runtime=COALESCE(runtime,?), model=COALESCE(model,?) WHERE id=?`,
		runtime, model, id,
	)
	return err
}

// DeleteConversation deletes the conversation, its query_sessions turns, and
// its chat_schedules rows. Returns (false, nil) if not found.
func (s *Store) DeleteConversation(id, userID string) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	// Check existence first.
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM conversations WHERE id=? AND user_id=?`, id, userID).Scan(&count); err != nil {
		return false, err
	}
	if count == 0 {
		return false, nil
	}

	// Delete chat_schedules for this conversation.
	if _, err := tx.Exec(`DELETE FROM chat_schedules WHERE conversation_id=? AND user_id=?`, id, userID); err != nil {
		return false, err
	}
	// Delete associated sessions.
	if _, err := tx.Exec(`DELETE FROM query_sessions WHERE conversation_id=?`, id); err != nil {
		return false, err
	}
	// Delete the conversation row.
	if _, err := tx.Exec(`DELETE FROM conversations WHERE id=? AND user_id=?`, id, userID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// TouchConversation updates updated_at to now for the given conversation.
func (s *Store) TouchConversation(id string) error {
	_, err := s.db.Exec(
		`UPDATE conversations SET updated_at=? WHERE id=?`,
		nowMs(), id,
	)
	return err
}

// InsertSessionWithConversation inserts a query_session row that belongs to a
// conversation with the default 'user' source.
func (s *Store) InsertSessionWithConversation(id, userID, username, query, conversationID string) error {
	return s.InsertSessionWithSource(id, userID, username, query, conversationID, "user")
}

// InsertSessionWithSource inserts a query_session row carrying a source tag
// (typically "user" or "schedule"). Empty source defaults to "user".
func (s *Store) InsertSessionWithSource(id, userID, username, query, conversationID, source string) error {
	if source == "" {
		source = "user"
	}
	_, err := s.db.Exec(
		`INSERT INTO query_sessions(id,user_id,username,query,started_at,status,conversation_id,source) VALUES(?,?,?,?,?,'running',?,?)`,
		id, userID, username, query, nowMs(), conversationID, source,
	)
	return err
}

// ListConversationsAdmin returns conversations across all users with optional
// filters: user (exact user_id match), q (LIKE on title), from/to (range on
// updated_at). Pagination is page/limit (1-based). Returns rows + total count.
func (s *Store) ListConversationsAdmin(user, q string, from, to int64, page, limit int) ([]ConversationRow, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	where := "1=1"
	args := []any{}
	if user != "" {
		where += " AND user_id=?"
		args = append(args, user)
	}
	if q != "" {
		where += " AND title LIKE ?"
		args = append(args, "%"+q+"%")
	}
	if from > 0 {
		where += " AND updated_at>=?"
		args = append(args, from)
	}
	if to > 0 {
		where += " AND updated_at<=?"
		args = append(args, to)
	}

	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM conversations WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listArgs := append(append([]any{}, args...), limit, offset)
	rows, err := s.db.Query(
		"SELECT id,user_id,title,created_at,updated_at,pinned,pinned_at,runtime,model FROM conversations WHERE "+where+
			" ORDER BY updated_at DESC LIMIT ? OFFSET ?",
		listArgs...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []ConversationRow
	for rows.Next() {
		c, scerr := scanConversationRow(rows, true)
		if scerr != nil {
			return nil, 0, scerr
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

// ConversationMessage is one persisted turn returned by message-list endpoints.
type ConversationMessage struct {
	ID        string  `json:"id"`
	UserID    string  `json:"user_id,omitempty"`
	Query     string  `json:"query"`
	Response  *string `json:"response,omitempty"`
	Status    string  `json:"status"`
	Source    string  `json:"source"`
	StartedAt int64   `json:"started_at"`
	EndedAt   *int64  `json:"ended_at,omitempty"`
}

// ListMessagesByConversation returns query_sessions belonging to the
// conversation, ordered by started_at ASC. limit<=0 means no cap (used by the
// ACP history loader); otherwise pagination via page/limit (1-based).
func (s *Store) ListMessagesByConversation(conversationID string, page, limit int) ([]ConversationMessage, int, error) {
	if limit < 0 {
		limit = 0
	}
	if limit > 0 && page <= 0 {
		page = 1
	}
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM query_sessions WHERE conversation_id=?`, conversationID).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := `SELECT id,user_id,query,response,status,source,started_at,ended_at FROM query_sessions WHERE conversation_id=? ORDER BY started_at ASC`
	args := []any{conversationID}
	if limit > 0 {
		q += " LIMIT ? OFFSET ?"
		args = append(args, limit, (page-1)*limit)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []ConversationMessage
	for rows.Next() {
		var m ConversationMessage
		var resp sql.NullString
		var ended sql.NullInt64
		if err := rows.Scan(&m.ID, &m.UserID, &m.Query, &resp, &m.Status, &m.Source, &m.StartedAt, &ended); err != nil {
			return nil, 0, err
		}
		if resp.Valid {
			s := resp.String
			m.Response = &s
		}
		if ended.Valid {
			v := ended.Int64
			m.EndedAt = &v
		}
		if m.Source == "" {
			m.Source = "user"
		}
		out = append(out, m)
	}
	return out, total, rows.Err()
}

// UpdateSessionDoneAndTouch marks the session done and touches the conversation.
func (s *Store) UpdateSessionDoneAndTouch(id, response, conversationID string) error {
	if err := s.UpdateSessionDone(id, response); err != nil {
		return err
	}
	if conversationID != "" {
		return s.TouchConversation(conversationID)
	}
	return nil
}

