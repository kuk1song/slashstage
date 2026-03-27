// Package parser — codex.go implements the Codex CLI parser.
// Reads sessions from JSONL files in ~/.codex/sessions/*.jsonl
package parser

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kuk1song/slashstage/internal/model"
)

func init() {
	Register(model.AgentCodex, &CodexParser{})
}

// CodexParser reads sessions from Codex CLI JSONL files.
type CodexParser struct{}

// Discover finds all Codex CLI session JSONL files.
func (p *CodexParser) Discover(config model.AgentConfig) ([]DiscoveredFile, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
	}
	sessionsDir := filepath.Join(home, ".codex", "sessions")
	if !DirExists(sessionsDir) {
		return nil, nil
	}

	var files []DiscoveredFile
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil, nil
	}

	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			files = append(files, DiscoveredFile{
				Path:      filepath.Join(sessionsDir, e.Name()),
				AgentType: model.AgentCodex,
			})
		}
	}
	return files, nil
}

// Parse reads a Codex CLI JSONL session file.
func (p *CodexParser) Parse(path string) ([]model.ParseResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")

	session := model.Session{
		ID:         sessionID,
		AgentType:  string(model.AgentCodex),
		SourcePath: path,
		SourceHash: hash,
		StartedAt:  time.Now(),
		LastActiveAt: time.Now(),
	}

	var messages []model.Message
	order := 0

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		// Extract CWD for workspace
		if raw, ok := entry["cwd"]; ok && session.Workspace == "" {
			var cwd string
			json.Unmarshal(raw, &cwd)
			session.Workspace = cwd
		}

		// Extract role
		var role string
		if raw, ok := entry["role"]; ok {
			json.Unmarshal(raw, &role)
		}
		if role == "" {
			continue
		}

		// Extract content
		content := extractJSONContent(entry["content"])
		if content == "" {
			if raw, ok := entry["text"]; ok {
				json.Unmarshal(raw, &content)
			}
		}
		if content == "" {
			continue
		}

		// Extract model
		if raw, ok := entry["model"]; ok && session.Model == "" {
			json.Unmarshal(raw, &session.Model)
		}

		// Title from first user message
		if session.Title == "" && (role == "user" || role == "human") {
			title := content
			if len(title) > 100 {
				title = title[:100] + "..."
			}
			session.Title = title
		}

		messages = append(messages, model.Message{
			SessionID: sessionID,
			Role:      normalizeLegacyRole(role),
			Content:   content,
			SortOrder: order,
		})
		order++
	}

	session.MessageCount = len(messages)
	if len(messages) == 0 {
		return nil, nil
	}

	return []model.ParseResult{{Session: session, Messages: messages}}, nil
}

// extractJSONContent tries to extract text content from a JSON content field.
func extractJSONContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	// Try as string
	var str string
	if json.Unmarshal(raw, &str) == nil {
		return str
	}

	// Try as array of content blocks
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			if b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}

	return ""
}
