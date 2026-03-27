package db

import (
	"fmt"
	"time"

	"github.com/kuk1song/slashstage/internal/model"
)

// InsertMessages batch-inserts messages for a session.
// It deletes existing messages first (full replace on re-sync).
func (db *DB) InsertMessages(sessionID string, messages []model.Message) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Delete existing messages for this session
	if _, err := tx.Exec(`DELETE FROM messages WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("delete old messages: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO messages (session_id, role, content, tool_name, tool_input,
		                      tokens_in, tokens_out, created_at, sort_order)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, m := range messages {
		createdAt := m.CreatedAt.UTC().Format(time.RFC3339)
		if m.CreatedAt.IsZero() {
			createdAt = time.Now().UTC().Format(time.RFC3339)
		}
		_, err := stmt.Exec(
			sessionID, string(m.Role), m.Content, m.ToolName, m.ToolInput,
			m.TokensIn, m.TokensOut, createdAt, m.SortOrder,
		)
		if err != nil {
			return fmt.Errorf("insert message %d: %w", m.SortOrder, err)
		}
	}

	return tx.Commit()
}

// GetMessages returns all messages for a session, ordered by sort_order.
func (db *DB) GetMessages(sessionID string) ([]model.Message, error) {
	rows, err := db.Query(`
		SELECT id, session_id, role, content, tool_name, tool_input,
		       tokens_in, tokens_out, created_at, sort_order
		FROM messages WHERE session_id = ?
		ORDER BY sort_order ASC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get messages for session %s: %w", sessionID, err)
	}
	defer rows.Close()

	var messages []model.Message
	for rows.Next() {
		var m model.Message
		var createdAt string
		err := rows.Scan(
			&m.ID, &m.SessionID, &m.Role, &m.Content, &m.ToolName, &m.ToolInput,
			&m.TokensIn, &m.TokensOut, &createdAt, &m.SortOrder,
		)
		if err != nil {
			return nil, err
		}
		m.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// SearchMessages performs full-text search across all messages.
func (db *DB) SearchMessages(query string, limit int) ([]model.Message, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.Query(`
		SELECT m.id, m.session_id, m.role, m.content, m.tool_name, m.tool_input,
		       m.tokens_in, m.tokens_out, m.created_at, m.sort_order
		FROM messages m
		JOIN messages_fts fts ON m.id = fts.rowid
		WHERE messages_fts MATCH ?
		ORDER BY rank
		LIMIT ?`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search messages: %w", err)
	}
	defer rows.Close()

	var messages []model.Message
	for rows.Next() {
		var m model.Message
		var createdAt string
		err := rows.Scan(
			&m.ID, &m.SessionID, &m.Role, &m.Content, &m.ToolName, &m.ToolInput,
			&m.TokensIn, &m.TokensOut, &createdAt, &m.SortOrder,
		)
		if err != nil {
			return nil, err
		}
		m.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		messages = append(messages, m)
	}
	return messages, rows.Err()
}
