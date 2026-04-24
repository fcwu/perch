package main

import (
	"crypto/rand"
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

// ConversationRow is a row returned by ListConversations.
type ConversationRow struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
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

// InsertConversation inserts a new conversation row.
// title is the first 60 chars of the first user message (truncated at word boundary if possible).
func (s *Store) InsertConversation(id, userID, title string) error {
	title = truncateTitle(title, 60)
	now := nowMs()
	_, err := s.db.Exec(
		`INSERT INTO conversations(id,user_id,title,created_at,updated_at) VALUES(?,?,?,?,?)`,
		id, userID, title, now, now,
	)
	return err
}

// ListConversations returns conversations for userID ordered by updated_at DESC, limit 50.
func (s *Store) ListConversations(userID string) ([]ConversationRow, error) {
	rows, err := s.db.Query(
		`SELECT id,title,created_at,updated_at FROM conversations WHERE user_id=? ORDER BY updated_at DESC LIMIT 50`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ConversationRow
	for rows.Next() {
		var c ConversationRow
		if err := rows.Scan(&c.ID, &c.Title, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteConversation deletes the conversation and all its query_sessions turns.
// Returns (false, nil) if not found.
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

// InsertSessionWithConversation inserts a query_session row that belongs to a conversation.
func (s *Store) InsertSessionWithConversation(id, userID, username, query, conversationID string) error {
	_, err := s.db.Exec(
		`INSERT INTO query_sessions(id,user_id,username,query,started_at,status,conversation_id) VALUES(?,?,?,?,?,'running',?)`,
		id, userID, username, query, nowMs(), conversationID,
	)
	return err
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

