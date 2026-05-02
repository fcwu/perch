package main

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"time"
)

// ChatSchedule is one row from the chat_schedules table.
type ChatSchedule struct {
	ID             string `json:"id"`
	UserID         string `json:"user_id"`
	ConversationID string `json:"conversation_id"`
	// Hour and Minute are populated only for daily-fire schedules; nil for one-shot.
	Hour        *int   `json:"hour,omitempty"`
	Minute      *int   `json:"minute,omitempty"`
	Repeat      bool   `json:"repeat"`
	OneShotAt   int64  `json:"one_shot_at"`
	Prompt      string `json:"prompt"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   int64  `json:"created_at"`
	LastFiredAt *int64 `json:"last_fired_at,omitempty"`
}

// ErrInvalidChatSchedule is returned when a user-supplied schedule fails
// validation. The CRUD handler maps it to 400 Bad Request.
var ErrInvalidChatSchedule = errors.New("invalid chat schedule")

// validateChatScheduleInput enforces the contract: exactly one of (hour+minute)
// or one_shot_at must be set; ranges are checked; one_shot_at must be in the
// future at insert time.
func validateChatScheduleInput(prompt string, hour, minute *int, oneShotAt int64, now int64) error {
	if prompt == "" {
		return fmt.Errorf("%w: prompt is required", ErrInvalidChatSchedule)
	}
	hasDaily := hour != nil || minute != nil
	hasOneShot := oneShotAt > 0
	if hasDaily && hasOneShot {
		return fmt.Errorf("%w: cannot set both hour/minute and one_shot_at", ErrInvalidChatSchedule)
	}
	if !hasDaily && !hasOneShot {
		return fmt.Errorf("%w: must set either hour+minute or one_shot_at", ErrInvalidChatSchedule)
	}
	if hasDaily {
		if hour == nil || minute == nil {
			return fmt.Errorf("%w: hour and minute must both be set", ErrInvalidChatSchedule)
		}
		if *hour < 0 || *hour > 23 {
			return fmt.Errorf("%w: hour out of range", ErrInvalidChatSchedule)
		}
		if *minute < 0 || *minute > 59 {
			return fmt.Errorf("%w: minute out of range", ErrInvalidChatSchedule)
		}
	}
	if hasOneShot && oneShotAt <= now {
		return fmt.Errorf("%w: one_shot_at must be in the future", ErrInvalidChatSchedule)
	}
	return nil
}

// chatScheduleID generates a 16-char hex id distinct from query_session UUIDs.
func chatScheduleID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// InsertChatSchedule inserts a new chat schedule row.
func (s *Store) InsertChatSchedule(sch *ChatSchedule) error {
	if sch.ID == "" {
		sch.ID = chatScheduleID()
	}
	if sch.CreatedAt == 0 {
		sch.CreatedAt = nowMs()
	}
	var hour, minute sql.NullInt64
	if sch.Hour != nil {
		hour = sql.NullInt64{Int64: int64(*sch.Hour), Valid: true}
	}
	if sch.Minute != nil {
		minute = sql.NullInt64{Int64: int64(*sch.Minute), Valid: true}
	}
	repeat := 0
	if sch.Repeat {
		repeat = 1
	}
	enabled := 1
	if !sch.Enabled {
		enabled = 0
	}
	_, err := s.db.Exec(
		`INSERT INTO chat_schedules(id,user_id,conversation_id,hour,minute,repeat,one_shot_at,prompt,enabled,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		sch.ID, sch.UserID, sch.ConversationID, hour, minute, repeat, sch.OneShotAt, sch.Prompt, enabled, sch.CreatedAt,
	)
	return err
}

func scanChatScheduleRow(scanner interface {
	Scan(...any) error
}) (ChatSchedule, error) {
	var c ChatSchedule
	var hour, minute, lastFired sql.NullInt64
	var repeat, enabled int
	if err := scanner.Scan(&c.ID, &c.UserID, &c.ConversationID, &hour, &minute, &repeat, &c.OneShotAt, &c.Prompt, &enabled, &c.CreatedAt, &lastFired); err != nil {
		return c, err
	}
	if hour.Valid {
		v := int(hour.Int64)
		c.Hour = &v
	}
	if minute.Valid {
		v := int(minute.Int64)
		c.Minute = &v
	}
	c.Repeat = repeat != 0
	c.Enabled = enabled != 0
	if lastFired.Valid {
		v := lastFired.Int64
		c.LastFiredAt = &v
	}
	return c, nil
}

// ListChatSchedules returns enabled and disabled schedules for the user+conv,
// ordered by created_at ASC.
func (s *Store) ListChatSchedules(userID, conversationID string) ([]ChatSchedule, error) {
	rows, err := s.db.Query(
		`SELECT id,user_id,conversation_id,hour,minute,repeat,one_shot_at,prompt,enabled,created_at,last_fired_at FROM chat_schedules
		 WHERE user_id=? AND conversation_id=? ORDER BY created_at ASC`,
		userID, conversationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChatSchedule
	for rows.Next() {
		c, scerr := scanChatScheduleRow(rows)
		if scerr != nil {
			return nil, scerr
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetChatSchedule returns one schedule by id, scoped to (user, conv).
func (s *Store) GetChatSchedule(id, userID, conversationID string) (*ChatSchedule, error) {
	row := s.db.QueryRow(
		`SELECT id,user_id,conversation_id,hour,minute,repeat,one_shot_at,prompt,enabled,created_at,last_fired_at FROM chat_schedules
		 WHERE id=? AND user_id=? AND conversation_id=?`,
		id, userID, conversationID,
	)
	c, err := scanChatScheduleRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// DeleteChatSchedule removes the row only when (id, user, conv) all match.
// Returns true when a row was deleted.
func (s *Store) DeleteChatSchedule(id, userID, conversationID string) (bool, error) {
	res, err := s.db.Exec(
		`DELETE FROM chat_schedules WHERE id=? AND user_id=? AND conversation_id=?`,
		id, userID, conversationID,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// LoadAllChatSchedules returns every chat_schedules row across all users —
// used by the scheduler on boot to populate its in-memory job map.
func (s *Store) LoadAllChatSchedules() ([]ChatSchedule, error) {
	rows, err := s.db.Query(
		`SELECT id,user_id,conversation_id,hour,minute,repeat,one_shot_at,prompt,enabled,created_at,last_fired_at FROM chat_schedules`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChatSchedule
	for rows.Next() {
		c, scerr := scanChatScheduleRow(rows)
		if scerr != nil {
			return nil, scerr
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// TouchChatScheduleFired updates last_fired_at on the schedule row.
func (s *Store) TouchChatScheduleFired(id string, firedAt int64) error {
	_, err := s.db.Exec(`UPDATE chat_schedules SET last_fired_at=? WHERE id=?`, firedAt, id)
	return err
}

// DeleteChatSchedulesByConversation removes all schedules for the given (user,
// conv). Used by the conversation DELETE cascade and by tests.
func (s *Store) DeleteChatSchedulesByConversation(conversationID, userID string) error {
	_, err := s.db.Exec(
		`DELETE FROM chat_schedules WHERE conversation_id=? AND user_id=?`,
		conversationID, userID,
	)
	return err
}

// ListChatSchedulesAdmin returns chat schedules across all users with optional
// filters by user_id and conversation_id, paginated.
func (s *Store) ListChatSchedulesAdmin(user, conv string, page, limit int) ([]ChatSchedule, int, error) {
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
	if conv != "" {
		where += " AND conversation_id=?"
		args = append(args, conv)
	}
	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM chat_schedules WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	listArgs := append(append([]any{}, args...), limit, offset)
	rows, err := s.db.Query(
		"SELECT id,user_id,conversation_id,hour,minute,repeat,one_shot_at,prompt,enabled,created_at,last_fired_at FROM chat_schedules WHERE "+where+
			" ORDER BY created_at DESC LIMIT ? OFFSET ?",
		listArgs...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []ChatSchedule
	for rows.Next() {
		c, scerr := scanChatScheduleRow(rows)
		if scerr != nil {
			return nil, 0, scerr
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

// --- HTTP handlers ---

// handleListChatSchedules handles GET /api/conversations/{id}/schedules.
func (s *Server) handleListChatSchedules(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	convID := r.PathValue("id")
	if convID == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	userID := s.resolveUserID(r)

	conv, err := s.store.GetConversation(convID, userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if conv == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	list, err := s.store.ListChatSchedules(userID, convID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []ChatSchedule{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"schedules": list})
}

// handleCreateChatSchedule handles POST /api/conversations/{id}/schedules.
// Body: {prompt, hour?, minute?, repeat?, one_shot_at?}.
func (s *Server) handleCreateChatSchedule(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	convID := r.PathValue("id")
	if convID == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	userID := s.resolveUserID(r)

	conv, err := s.store.GetConversation(convID, userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if conv == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	var req struct {
		Prompt    string `json:"prompt"`
		Hour      *int   `json:"hour,omitempty"`
		Minute    *int   `json:"minute,omitempty"`
		Repeat    bool   `json:"repeat,omitempty"`
		OneShotAt int64  `json:"one_shot_at,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: invalid JSON", http.StatusBadRequest)
		return
	}
	if err := validateChatScheduleInput(req.Prompt, req.Hour, req.Minute, req.OneShotAt, time.Now().UnixMilli()); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	sch := &ChatSchedule{
		UserID:         userID,
		ConversationID: convID,
		Hour:           req.Hour,
		Minute:         req.Minute,
		Repeat:         req.Repeat,
		OneShotAt:      req.OneShotAt,
		Prompt:         req.Prompt,
		Enabled:        true,
	}
	if err := s.store.InsertChatSchedule(sch); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if s.scheduler != nil {
		s.scheduler.ReloadChatSchedules()
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(sch)
}

// handleDeleteChatSchedule handles DELETE /api/conversations/{id}/schedules/{job_id}.
func (s *Server) handleDeleteChatSchedule(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	convID := r.PathValue("id")
	jobID := r.PathValue("job_id")
	if convID == "" || jobID == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	userID := s.resolveUserID(r)
	ok, err := s.store.DeleteChatSchedule(jobID, userID, convID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if s.scheduler != nil {
		s.scheduler.ReloadChatSchedules()
	}
	w.WriteHeader(http.StatusNoContent)
}
