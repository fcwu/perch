package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Store wraps a SQLite DB for persisting query sessions and tool events.
// A mutex serializes all writes so concurrent goroutines don't deadlock.
type Store struct {
	db  *sql.DB
	mu  sync.Mutex
}

// OpenStore opens (or creates) the SQLite database at path and applies the schema.
func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// Single writer connection prevents SQLITE_BUSY under concurrent load.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	if err := migrateStore(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func migrateStore(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS query_sessions (
	id         TEXT PRIMARY KEY,
	user_id    TEXT NOT NULL,
	username   TEXT NOT NULL,
	query      TEXT NOT NULL,
	response   TEXT,
	started_at INTEGER NOT NULL,
	ended_at   INTEGER,
	status     TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS tool_events (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id  TEXT NOT NULL REFERENCES query_sessions(id),
	tool_name   TEXT NOT NULL,
	input_json  TEXT,
	output_json TEXT,
	started_at  INTEGER NOT NULL,
	ended_at    INTEGER
);
CREATE TABLE IF NOT EXISTS conversations (
	id         TEXT PRIMARY KEY,
	user_id    TEXT NOT NULL,
	title      TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS chat_schedules (
	id              TEXT PRIMARY KEY,
	user_id         TEXT NOT NULL,
	conversation_id TEXT NOT NULL REFERENCES conversations(id),
	hour            INTEGER,
	minute          INTEGER,
	repeat          INTEGER NOT NULL DEFAULT 0,
	one_shot_at     INTEGER NOT NULL DEFAULT 0,
	prompt          TEXT NOT NULL,
	enabled         INTEGER NOT NULL DEFAULT 1,
	created_at      INTEGER NOT NULL,
	last_fired_at   INTEGER
);
CREATE INDEX IF NOT EXISTS idx_qs_user_id    ON query_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_qs_started_at ON query_sessions(started_at);
CREATE INDEX IF NOT EXISTS idx_te_session_id ON tool_events(session_id);
CREATE INDEX IF NOT EXISTS idx_conv_user_id    ON conversations(user_id);
CREATE INDEX IF NOT EXISTS idx_conv_updated_at ON conversations(updated_at);
CREATE INDEX IF NOT EXISTS idx_chat_schedules_user_conv ON chat_schedules(user_id, conversation_id);
`)
	if err != nil {
		return err
	}
	if err := migrateQuerySessionsColumns(db); err != nil {
		return err
	}
	return migrateConversationsColumns(db)
}

// addColumnIfMissing inspects PRAGMA table_info(table) and runs `ALTER TABLE
// table ADD COLUMN ...` only when the column is absent. SQLite does not support
// `IF NOT EXISTS` on ALTER TABLE, so this idempotent helper is the migration
// primitive used by the per-table migrators.
func addColumnIfMissing(db *sql.DB, table, column, decl string) error {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var dflt interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, decl))
	return err
}

// migrateQuerySessionsColumns adds columns to query_sessions that were added
// after the initial schema. Idempotent: re-running on an already-migrated DB is
// a no-op.
func migrateQuerySessionsColumns(db *sql.DB) error {
	if err := addColumnIfMissing(db, "query_sessions", "conversation_id", "TEXT"); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_qs_conv_id ON query_sessions(conversation_id)`); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "query_sessions", "source", "TEXT NOT NULL DEFAULT 'user'"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "query_sessions", "images_json", "TEXT"); err != nil {
		return err
	}
	return nil
}

// migrateConversationsColumns adds the pinning + per-conversation runtime
// columns introduced for the multi-agent chat work.
func migrateConversationsColumns(db *sql.DB) error {
	if err := addColumnIfMissing(db, "conversations", "pinned", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "conversations", "pinned_at", "INTEGER"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "conversations", "runtime", "TEXT"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, "conversations", "model", "TEXT"); err != nil {
		return err
	}
	return nil
}

func nowMs() int64 { return time.Now().UnixMilli() }

func (s *Store) InsertSession(id, userID, username, query string) error {
	_, err := s.db.Exec(
		`INSERT INTO query_sessions(id,user_id,username,query,started_at,status) VALUES(?,?,?,?,?,'running')`,
		id, userID, username, query, nowMs(),
	)
	return err
}

func (s *Store) UpdateSessionDone(id, response string, images ...ImageAttachment) error {
	var imagesJSON *string
	if len(images) > 0 {
		if b, err := json.Marshal(images); err == nil {
			s := string(b)
			imagesJSON = &s
		}
	}
	_, err := s.db.Exec(
		`UPDATE query_sessions SET response=?,images_json=?,ended_at=?,status='done' WHERE id=?`,
		response, imagesJSON, nowMs(), id,
	)
	return err
}

func (s *Store) UpdateSessionTimeout(id string) error {
	_, err := s.db.Exec(
		`UPDATE query_sessions SET ended_at=?,status='timeout' WHERE id=?`,
		nowMs(), id,
	)
	return err
}

func (s *Store) UpdateSessionError(id, errMsg string) error {
	_, err := s.db.Exec(
		`UPDATE query_sessions SET response=?,ended_at=?,status='error' WHERE id=?`,
		errMsg, nowMs(), id,
	)
	return err
}

func (s *Store) InsertToolEvent(sessionID, toolName, inputJSON string) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO tool_events(session_id,tool_name,input_json,started_at) VALUES(?,?,?,?)`,
		sessionID, toolName, inputJSON, nowMs(),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateToolEventEnd(eventID int64, inputJSON, outputJSON string) error {
	_, err := s.db.Exec(
		`UPDATE tool_events SET input_json=?,output_json=?,ended_at=? WHERE id=?`,
		inputJSON, outputJSON, nowMs(), eventID,
	)
	return err
}

// ConversationTurn represents a completed query/response pair from a prior session.
type ConversationTurn struct {
	Query    string
	Response string
}

// SessionRow is a row returned by ListSessions.
type SessionRow struct {
	ID         string
	UserID     string
	Username   string
	Query      string
	Status     string
	StartedAt  int64
	EndedAt    *int64
	DurationMs *int64
}

// GetRecentHistory returns up to limit completed turns for userID with started_at >= since (ms).
func (s *Store) GetRecentHistory(userID string, since int64, limit int) ([]ConversationTurn, error) {
	rows, err := s.db.Query(
		`SELECT query,response FROM query_sessions WHERE user_id=? AND status='done' AND started_at>=? ORDER BY started_at ASC LIMIT ?`,
		userID, since, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ConversationTurn
	for rows.Next() {
		var t ConversationTurn
		if err := rows.Scan(&t.Query, &t.Response); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListSessions returns paginated sessions matching optional filters.
func (s *Store) ListSessions(user, q string, from, to int64, page, limit int) ([]SessionRow, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	where := "1=1"
	args := []any{}
	if user != "" {
		where += " AND username=?"
		args = append(args, user)
	}
	if q != "" {
		where += " AND query LIKE ?"
		args = append(args, "%"+q+"%")
	}
	if from > 0 {
		where += " AND started_at>=?"
		args = append(args, from)
	}
	if to > 0 {
		where += " AND started_at<=?"
		args = append(args, to)
	}

	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM query_sessions WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limitArgs := append(args, limit, offset)
	rows, err := s.db.Query(
		"SELECT id,user_id,username,query,status,started_at,ended_at FROM query_sessions WHERE "+where+" ORDER BY started_at DESC LIMIT ? OFFSET ?",
		limitArgs...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []SessionRow
	for rows.Next() {
		var r SessionRow
		if err := rows.Scan(&r.ID, &r.UserID, &r.Username, &r.Query, &r.Status, &r.StartedAt, &r.EndedAt); err != nil {
			return nil, 0, err
		}
		if r.EndedAt != nil {
			ms := *r.EndedAt - r.StartedAt
			r.DurationMs = &ms
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

// ToolEventRow is a single tool event.
type ToolEventRow struct {
	ID         int64
	SessionID  string
	ToolName   string
	InputJSON  *string
	OutputJSON *string
	StartedAt  int64
	EndedAt    *int64
}

// SessionDetail includes the full session and its tool events.
type SessionDetail struct {
	SessionRow
	Response   *string
	ToolEvents []ToolEventRow
}

// GetSession returns the full session detail including tool events.
func (s *Store) GetSession(id string) (*SessionDetail, error) {
	var d SessionDetail
	err := s.db.QueryRow(
		"SELECT id,user_id,username,query,response,status,started_at,ended_at FROM query_sessions WHERE id=?", id,
	).Scan(&d.ID, &d.UserID, &d.Username, &d.Query, &d.Response, &d.Status, &d.StartedAt, &d.EndedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if d.EndedAt != nil {
		ms := *d.EndedAt - d.StartedAt
		d.DurationMs = &ms
	}

	rows, err := s.db.Query(
		"SELECT id,session_id,tool_name,input_json,output_json,started_at,ended_at FROM tool_events WHERE session_id=? ORDER BY started_at",
		id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var te ToolEventRow
		if err := rows.Scan(&te.ID, &te.SessionID, &te.ToolName, &te.InputJSON, &te.OutputJSON, &te.StartedAt, &te.EndedAt); err != nil {
			return nil, err
		}
		d.ToolEvents = append(d.ToolEvents, te)
	}
	return &d, rows.Err()
}

// UsageStats contains analytics data.
type UsageStats struct {
	Users          []UserStat  `json:"users"`
	TopTools       []ToolStat  `json:"top_tools"`
	TotalQueries   int         `json:"total_queries"`
	TotalDurationMs int64      `json:"total_duration_ms"`
}

type UserStat struct {
	Username      string  `json:"username"`
	QueryCount    int     `json:"query_count"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
}

type ToolStat struct {
	Tool  string `json:"tool"`
	Count int    `json:"count"`
}

// GetUsageStats returns aggregated usage statistics for the given time range.
func (s *Store) GetUsageStats(from, to int64) (*UsageStats, error) {
	stats := &UsageStats{
		Users:    []UserStat{},
		TopTools: []ToolStat{},
	}

	where := "status='done'"
	args := []any{}
	if from > 0 {
		where += " AND started_at>=?"
		args = append(args, from)
	}
	if to > 0 {
		where += " AND started_at<=?"
		args = append(args, to)
	}

	rows, err := s.db.Query(
		"SELECT username,COUNT(*) as cnt,AVG(ended_at-started_at) as avg_dur FROM query_sessions WHERE "+where+" GROUP BY user_id ORDER BY cnt DESC",
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var u UserStat
		if err := rows.Scan(&u.Username, &u.QueryCount, &u.AvgDurationMs); err != nil {
			return nil, err
		}
		stats.TotalQueries += u.QueryCount
		stats.TotalDurationMs += int64(u.AvgDurationMs * float64(u.QueryCount))
		stats.Users = append(stats.Users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	toolWhere := "s.status='done'"
	if from > 0 {
		toolWhere += " AND s.started_at>=?"
	}
	if to > 0 {
		toolWhere += " AND s.started_at<=?"
	}
	toolRows, err := s.db.Query(
		`SELECT t.tool_name,COUNT(*) as cnt FROM tool_events t
		 JOIN query_sessions s ON t.session_id=s.id WHERE `+toolWhere+` GROUP BY t.tool_name ORDER BY cnt DESC LIMIT 10`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer toolRows.Close()
	for toolRows.Next() {
		var ts ToolStat
		if err := toolRows.Scan(&ts.Tool, &ts.Count); err != nil {
			return nil, err
		}
		stats.TopTools = append(stats.TopTools, ts)
	}
	return stats, toolRows.Err()
}
