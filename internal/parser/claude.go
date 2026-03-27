// Package parser — claude.go implements the Claude Code parser.
// Reads sessions from JSONL files in ~/.claude/projects/<encoded-path>/sessions/*.jsonl
// Referenced from agentsview internal/parser/claude.go (MIT License).
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
	Register(model.AgentClaude, &ClaudeParser{})
}

// ClaudeParser reads sessions from Claude Code JSONL files.
type ClaudeParser struct{}

// Discover finds all Claude Code session JSONL files.
func (p *ClaudeParser) Discover(config model.AgentConfig) ([]DiscoveredFile, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
	}
	projectsDir := filepath.Join(home, ".claude", "projects")
	if !DirExists(projectsDir) {
		return nil, nil
	}

	var files []DiscoveredFile

	// Walk ~/.claude/projects/<project-dir>/sessions/*.jsonl
	projectDirs, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil, nil
	}

	for _, projDir := range projectDirs {
		if !projDir.IsDir() {
			continue
		}
		sessionsDir := filepath.Join(projectsDir, projDir.Name(), "sessions")
		if !DirExists(sessionsDir) {
			continue
		}

		sessionFiles, err := os.ReadDir(sessionsDir)
		if err != nil {
			continue
		}
		for _, sf := range sessionFiles {
			if !sf.IsDir() && strings.HasSuffix(sf.Name(), ".jsonl") {
				files = append(files, DiscoveredFile{
					Path:      filepath.Join(sessionsDir, sf.Name()),
					AgentType: model.AgentClaude,
				})
			}
		}
	}

	return files, nil
}

// Parse reads a Claude Code JSONL session file.
func (p *ClaudeParser) Parse(path string) ([]model.ParseResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	// Compute hash
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))

	// Extract workspace from path: ~/.claude/projects/<encoded-path>/sessions/xxx.jsonl
	workspace := extractClaudeWorkspace(path)

	// Session ID from filename
	sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")

	session := model.Session{
		ID:         sessionID,
		AgentType:  string(model.AgentClaude),
		Workspace:  workspace,
		SourcePath: path,
		SourceHash: hash,
		StartedAt:  time.Now(),
		LastActiveAt: time.Now(),
	}

	var messages []model.Message
	order := 0

	scanner := bufio.NewScanner(f)
	// Increase buffer for long lines
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry claudeEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		// Update session metadata
		if entry.Model != "" && session.Model == "" {
			session.Model = entry.Model
		}

		// Extract timestamp
		ts := parseClaudeTimestamp(entry.Timestamp)
		if order == 0 {
			session.StartedAt = ts
		}
		session.LastActiveAt = ts

		// Extract title from first user message
		if session.Title == "" && entry.Role == "user" {
			title := extractClaudeContent(&entry)
			if len(title) > 100 {
				title = title[:100] + "..."
			}
			session.Title = title
		}

		// Build message
		content := extractClaudeContent(&entry)
		if content == "" && entry.Type != "tool_use" && entry.Type != "tool_result" {
			continue
		}

		role := normalizeClaudeRole(entry.Role, entry.Type)
		msg := model.Message{
			SessionID: sessionID,
			Role:      role,
			Content:   content,
			SortOrder: order,
			CreatedAt: ts,
		}

		// Tool calls
		if entry.Type == "tool_use" {
			msg.ToolName = entry.Name
			if entry.Input != nil {
				inputBytes, _ := json.Marshal(entry.Input)
				msg.ToolInput = string(inputBytes)
			}
		}

		messages = append(messages, msg)
		order++
	}

	session.MessageCount = len(messages)

	if len(messages) == 0 {
		return nil, nil
	}

	return []model.ParseResult{{Session: session, Messages: messages}}, nil
}

// --- Internal types ---

type claudeEntry struct {
	Role      string          `json:"role"`
	Type      string          `json:"type"`
	Content   json.RawMessage `json:"content"`
	Text      string          `json:"text"`
	Model     string          `json:"model"`
	Name      string          `json:"name"`      // tool name
	Input     json.RawMessage `json:"input"`     // tool input
	Timestamp json.RawMessage `json:"timestamp"` // Various formats
}

// --- Helpers ---

func extractClaudeWorkspace(path string) string {
	// Path: ~/.claude/projects/<encoded-path>/sessions/xxx.jsonl
	// We want to decode <encoded-path>
	dir := filepath.Dir(filepath.Dir(path)) // Go up from sessions/ to project dir
	encodedName := filepath.Base(dir)
	return DecodeEncodedPath(encodedName)
}

func extractClaudeContent(entry *claudeEntry) string {
	if entry.Text != "" {
		return entry.Text
	}

	if len(entry.Content) == 0 {
		return ""
	}

	// Try as string
	var str string
	if json.Unmarshal(entry.Content, &str) == nil {
		return str
	}

	// Try as array of content blocks
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(entry.Content, &blocks) == nil {
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

func normalizeClaudeRole(role, msgType string) model.MessageRole {
	switch role {
	case "user", "human":
		return model.RoleUser
	case "assistant":
		return model.RoleAssistant
	}
	switch msgType {
	case "tool_use":
		return model.RoleAssistant
	case "tool_result":
		return model.RoleTool
	}
	return model.RoleAssistant
}

func parseClaudeTimestamp(raw json.RawMessage) time.Time {
	if len(raw) == 0 {
		return time.Now()
	}

	// Try as string (ISO 8601)
	var str string
	if json.Unmarshal(raw, &str) == nil && str != "" {
		if t, err := time.Parse(time.RFC3339, str); err == nil {
			return t
		}
		if t, err := time.Parse(time.RFC3339Nano, str); err == nil {
			return t
		}
	}

	// Try as Unix milliseconds
	var ms float64
	if json.Unmarshal(raw, &ms) == nil && ms > 0 {
		return time.UnixMilli(int64(ms))
	}

	return time.Now()
}
