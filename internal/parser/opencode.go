// Package parser — opencode.go implements the OpenCode parser.
// Reads sessions from SQLite databases in ~/.local/share/opencode/*.db
package parser

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kuk1song/slashstage/internal/model"

	_ "modernc.org/sqlite"
)

func init() {
	Register(model.AgentOpenCode, &OpenCodeParser{})
}

// OpenCodeParser reads sessions from OpenCode SQLite databases.
type OpenCodeParser struct{}

// Discover finds all OpenCode database files.
func (p *OpenCodeParser) Discover(config model.AgentConfig) ([]DiscoveredFile, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
	}
	dataDir := filepath.Join(home, ".local", "share", "opencode")
	if !DirExists(dataDir) {
		return nil, nil
	}

	var files []DiscoveredFile
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return nil, nil
	}

	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".db") {
			files = append(files, DiscoveredFile{
				Path:      filepath.Join(dataDir, e.Name()),
				AgentType: model.AgentOpenCode,
			})
		}
	}

	return files, nil
}

// Parse reads an OpenCode SQLite database.
func (p *OpenCodeParser) Parse(path string) ([]model.ParseResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))

	db, err := sql.Open("sqlite", path+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	defer db.Close()

	// Check for session table
	var tableName string
	err = db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='session'`).Scan(&tableName)
	if err != nil {
		return nil, nil
	}

	// Query sessions
	rows, err := db.Query(`SELECT id, workspace, model, created_at FROM session ORDER BY created_at DESC`)
	if err != nil {
		// Try alternative column names
		rows, err = db.Query(`SELECT id, cwd, model, created_at FROM session ORDER BY created_at DESC`)
		if err != nil {
			return nil, nil
		}
	}
	defer rows.Close()

	var results []model.ParseResult

	for rows.Next() {
		var sessionID, workspace, sessionModel, createdAt string
		if err := rows.Scan(&sessionID, &workspace, &sessionModel, &createdAt); err != nil {
			continue
		}

		session := model.Session{
			ID:         "opencode-" + sessionID,
			AgentType:  string(model.AgentOpenCode),
			Workspace:  workspace,
			Model:      sessionModel,
			SourcePath: path,
			SourceHash: hash,
			StartedAt:  parseTimeStr(createdAt),
			LastActiveAt: parseTimeStr(createdAt),
		}

		// Fetch messages for this session
		msgRows, err := db.Query(`
			SELECT role, content, created_at 
			FROM message WHERE session_id = ? 
			ORDER BY created_at ASC`, sessionID)
		if err != nil {
			continue
		}

		var messages []model.Message
		order := 0
		for msgRows.Next() {
			var role, content, msgCreated string
			if err := msgRows.Scan(&role, &content, &msgCreated); err != nil {
				continue
			}
			if content == "" {
				continue
			}

			if session.Title == "" && (role == "user" || role == "human") {
				title := content
				if len(title) > 100 {
					title = title[:100] + "..."
				}
				session.Title = title
			}

			messages = append(messages, model.Message{
				SessionID: session.ID,
				Role:      normalizeLegacyRole(role),
				Content:   content,
				SortOrder: order,
				CreatedAt: parseTimeStr(msgCreated),
			})
			order++

			session.LastActiveAt = parseTimeStr(msgCreated)
		}
		msgRows.Close()

		session.MessageCount = len(messages)
		if len(messages) > 0 {
			results = append(results, model.ParseResult{Session: session, Messages: messages})
		}
	}

	return results, nil
}

func parseTimeStr(s string) time.Time {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t
	}
	return time.Time{}
}
