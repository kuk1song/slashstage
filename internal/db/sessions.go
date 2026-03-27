package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/kuk1song/slashstage/internal/model"
)

// UpsertSession inserts or updates a session (keyed by ID).
func (db *DB) UpsertSession(s *model.Session) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`
		INSERT INTO sessions (id, project_id, agent_type, title, workspace, model,
		                      message_count, tokens_in, tokens_out,
		                      started_at, last_active_at, source_path, source_hash,
		                      created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			project_id     = excluded.project_id,
			title          = excluded.title,
			workspace      = excluded.workspace,
			model          = excluded.model,
			message_count  = excluded.message_count,
			tokens_in      = excluded.tokens_in,
			tokens_out     = excluded.tokens_out,
			last_active_at = excluded.last_active_at,
			source_hash    = excluded.source_hash,
			updated_at     = excluded.updated_at`,
		s.ID, s.ProjectID, s.AgentType, s.Title, s.Workspace, s.Model,
		s.MessageCount, s.TokensIn, s.TokensOut,
		s.StartedAt.UTC().Format(time.RFC3339),
		s.LastActiveAt.UTC().Format(time.RFC3339),
		s.SourcePath, s.SourceHash, now, now,
	)
	if err != nil {
		return fmt.Errorf("upsert session %s: %w", s.ID, err)
	}
	return nil
}

// ListSessionsByProject returns all sessions for a project.
func (db *DB) ListSessionsByProject(projectID int64) ([]model.Session, error) {
	rows, err := db.Query(`
		SELECT id, project_id, agent_type, title, workspace, model,
		       message_count, tokens_in, tokens_out,
		       started_at, last_active_at, source_path, source_hash,
		       created_at, updated_at
		FROM sessions WHERE project_id = ?
		ORDER BY last_active_at DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list sessions for project %d: %w", projectID, err)
	}
	defer rows.Close()
	return scanSessions(rows)
}

// ListUnassignedSessions returns sessions not linked to any project.
func (db *DB) ListUnassignedSessions() ([]model.Session, error) {
	rows, err := db.Query(`
		SELECT id, project_id, agent_type, title, workspace, model,
		       message_count, tokens_in, tokens_out,
		       started_at, last_active_at, source_path, source_hash,
		       created_at, updated_at
		FROM sessions WHERE project_id IS NULL
		ORDER BY last_active_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list unassigned sessions: %w", err)
	}
	defer rows.Close()
	return scanSessions(rows)
}

// GetSession returns a session by ID.
func (db *DB) GetSession(id string) (*model.Session, error) {
	row := db.QueryRow(`
		SELECT id, project_id, agent_type, title, workspace, model,
		       message_count, tokens_in, tokens_out,
		       started_at, last_active_at, source_path, source_hash,
		       created_at, updated_at
		FROM sessions WHERE id = ?`, id)
	return scanSession(row)
}

func scanSession(row *sql.Row) (*model.Session, error) {
	var s model.Session
	var projectID sql.NullInt64
	var startedAt, lastActive, createdAt, updatedAt string
	err := row.Scan(
		&s.ID, &projectID, &s.AgentType, &s.Title, &s.Workspace, &s.Model,
		&s.MessageCount, &s.TokensIn, &s.TokensOut,
		&startedAt, &lastActive, &s.SourcePath, &s.SourceHash,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	if projectID.Valid {
		s.ProjectID = &projectID.Int64
	}
	s.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	s.LastActiveAt, _ = time.Parse(time.RFC3339, lastActive)
	s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	s.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &s, nil
}

func scanSessions(rows *sql.Rows) ([]model.Session, error) {
	var sessions []model.Session
	for rows.Next() {
		var s model.Session
		var projectID sql.NullInt64
		var startedAt, lastActive, createdAt, updatedAt string
		err := rows.Scan(
			&s.ID, &projectID, &s.AgentType, &s.Title, &s.Workspace, &s.Model,
			&s.MessageCount, &s.TokensIn, &s.TokensOut,
			&startedAt, &lastActive, &s.SourcePath, &s.SourceHash,
			&createdAt, &updatedAt,
		)
		if err != nil {
			return nil, err
		}
		if projectID.Valid {
			s.ProjectID = &projectID.Int64
		}
		s.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
		s.LastActiveAt, _ = time.Parse(time.RFC3339, lastActive)
		s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		s.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}
