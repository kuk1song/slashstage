package db

import (
	"fmt"
	"time"
)

// SyncState tracks file hashes for incremental sync.
type SyncState struct {
	SourcePath string
	Hash       string
	AgentType  string
	LastSynced time.Time
	Skip       bool
}

// GetSyncState returns the sync state for a source file.
func (db *DB) GetSyncState(sourcePath string) (*SyncState, error) {
	var s SyncState
	var lastSynced string
	var skip int
	err := db.QueryRow(
		`SELECT source_path, hash, agent_type, last_synced, skip FROM sync_state WHERE source_path = ?`,
		sourcePath,
	).Scan(&s.SourcePath, &s.Hash, &s.AgentType, &lastSynced, &skip)
	if err != nil {
		return nil, err
	}
	s.LastSynced, _ = time.Parse(time.RFC3339, lastSynced)
	s.Skip = skip == 1
	return &s, nil
}

// SetSyncState updates the sync state for a source file.
func (db *DB) SetSyncState(sourcePath, hash, agentType string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`
		INSERT INTO sync_state (source_path, hash, agent_type, last_synced, skip)
		VALUES (?, ?, ?, ?, 0)
		ON CONFLICT(source_path) DO UPDATE SET
			hash = excluded.hash,
			agent_type = excluded.agent_type,
			last_synced = excluded.last_synced,
			skip = 0`,
		sourcePath, hash, agentType, now,
	)
	if err != nil {
		return fmt.Errorf("set sync state for %s: %w", sourcePath, err)
	}
	return nil
}

// MarkSkip marks a source file to be skipped in future syncs (parse failure).
func (db *DB) MarkSkip(sourcePath string) error {
	_, err := db.Exec(`UPDATE sync_state SET skip = 1 WHERE source_path = ?`, sourcePath)
	return err
}
