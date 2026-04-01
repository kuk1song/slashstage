// Package parser — codex.go implements the Codex CLI parser.
// Reads sessions from JSONL files in ~/.codex/sessions/ (including nested date dirs).
//
// Codex JSONL format (2026):
//
//	{"timestamp":"...","type":"session_meta","payload":{"id":"...","cwd":"...","model_provider":"openai",...}}
//	{"timestamp":"...","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"..."}]}}
//	{"timestamp":"...","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"..."}]}}
//	{"timestamp":"...","type":"response_item","payload":{"type":"function_call","name":"shell","arguments":"{...}","call_id":"..."}}
//	{"timestamp":"...","type":"response_item","payload":{"type":"function_call_output","call_id":"...","output":"..."}}
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
// Supports both flat layout (~/.codex/sessions/*.jsonl) and nested date
// directories (~/.codex/sessions/2026/03/28/*.jsonl).
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
	err = filepath.Walk(sessionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".jsonl") {
			files = append(files, DiscoveredFile{
				Path:      path,
				AgentType: model.AgentCodex,
			})
		}
		return nil
	})
	if err != nil {
		return nil, nil
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

	// Default session ID from filename
	sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")

	session := model.Session{
		ID:           "codex-" + sessionID,
		AgentType:    string(model.AgentCodex),
		SourcePath:   path,
		SourceHash:   hash,
		StartedAt:    GetFileModTime(path),
		LastActiveAt: GetFileModTime(path),
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

		var entry codexEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		// Parse top-level timestamp
		ts := parseCodexTimestamp(entry.Timestamp)

		switch entry.Type {
		case "session_meta":
			// Extract session metadata from payload
			var meta codexSessionMeta
			if json.Unmarshal(entry.Payload, &meta) == nil {
				if meta.ID != "" {
					session.ID = "codex-" + meta.ID
				}
				if meta.CWD != "" {
					session.Workspace = meta.CWD
				}
				if meta.CLIVersion != "" {
					session.Model = "codex/" + meta.CLIVersion
				}
				if meta.ModelProvider != "" {
					session.Model = meta.ModelProvider
				}
				session.StartedAt = ts
			}

		case "response_item":
			// Extract message from payload
			var item codexResponseItem
			if json.Unmarshal(entry.Payload, &item) != nil {
				continue
			}

			session.LastActiveAt = ts

			switch item.Type {
			case "message":
				content := extractCodexContent(item.Content)
				if content == "" {
					continue
				}

				role := normalizeCodexRole(item.Role)

				// Extract title from first user message (skip environment context)
				if session.Title == "" && role == model.RoleUser && !strings.HasPrefix(content, "<environment_context>") {
					title := content
					if len(title) > 100 {
						title = title[:100] + "..."
					}
					session.Title = title
				}

				messages = append(messages, model.Message{
					SessionID: session.ID,
					Role:      role,
					Content:   content,
					SortOrder: order,
					CreatedAt: ts,
				})
				order++

			case "function_call":
				// Tool call
				messages = append(messages, model.Message{
					SessionID: session.ID,
					Role:      model.RoleAssistant,
					Content:   fmt.Sprintf("[tool_call: %s]", item.Name),
					ToolName:  item.Name,
					ToolInput: item.Arguments,
					SortOrder: order,
					CreatedAt: ts,
				})
				order++

			case "function_call_output":
				// Tool result
				content := item.Output
				if len(content) > 2000 {
					content = content[:2000] + "...(truncated)"
				}
				messages = append(messages, model.Message{
					SessionID: session.ID,
					Role:      model.RoleTool,
					Content:   content,
					SortOrder: order,
					CreatedAt: ts,
				})
				order++
			}

		case "event_msg":
			// Event messages (info, rate_limits, etc.) — skip for now
			// Could extract model info from rate_limits events in the future
		}
	}

	session.MessageCount = len(messages)
	if len(messages) == 0 {
		return nil, nil
	}

	return []model.ParseResult{{Session: session, Messages: messages}}, nil
}

// --- Internal types ---

// codexEntry is the top-level JSONL entry format.
type codexEntry struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"` // "session_meta", "response_item", "event_msg", "turn_context"
	Payload   json.RawMessage `json:"payload"`
}

// codexSessionMeta is the payload for type="session_meta".
type codexSessionMeta struct {
	ID            string `json:"id"`
	CWD           string `json:"cwd"`
	Originator    string `json:"originator"`
	CLIVersion    string `json:"cli_version"`
	Source        string `json:"source"`
	ModelProvider string `json:"model_provider"`
}

// codexResponseItem is the payload for type="response_item".
type codexResponseItem struct {
	Type      string          `json:"type"` // "message", "function_call", "function_call_output"
	Role      string          `json:"role"` // "user", "assistant", "developer"
	Content   json.RawMessage `json:"content"`
	Name      string          `json:"name"`      // function name (for function_call)
	Arguments string          `json:"arguments"` // function args (for function_call)
	CallID    string          `json:"call_id"`
	Output    string          `json:"output"` // function output (for function_call_output)
}

// --- Helpers ---

func parseCodexTimestamp(s string) time.Time {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	if t, err := time.Parse("2006-01-02T15:04:05.000Z", s); err == nil {
		return t
	}
	return time.Time{}
}

func normalizeCodexRole(role string) model.MessageRole {
	switch role {
	case "user", "human":
		return model.RoleUser
	case "assistant":
		return model.RoleAssistant
	case "developer", "system":
		return model.RoleSystem
	default:
		return model.RoleAssistant
	}
}

// extractCodexContent extracts text from Codex content blocks.
// Content is an array: [{"type":"input_text","text":"..."}, ...]
// or [{"type":"output_text","text":"..."}, ...]
func extractCodexContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	// Try as string first
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
