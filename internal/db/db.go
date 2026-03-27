// Package db provides SQLite database access for SlashStage.
package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// DB wraps *sql.DB with SlashStage-specific methods.
type DB struct {
	*sql.DB
	path string
}

// DefaultDBPath returns the default database path: ~/.slashstage/data.db
func DefaultDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".slashstage", "data.db"), nil
}

// Open opens (or creates) the SlashStage SQLite database.
func Open(dbPath string) (*DB, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create db directory %s: %w", dir, err)
	}

	// Open SQLite with modernc.org/sqlite (pure Go, no CGO)
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open database: %w", err)
	}

	// Apply schema
	if _, err := sqlDB.Exec(schemaSQL); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("cannot apply schema: %w", err)
	}

	return &DB{DB: sqlDB, path: dbPath}, nil
}

// Path returns the database file path.
func (db *DB) Path() string {
	return db.path
}
