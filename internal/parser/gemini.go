// Package parser — gemini.go implements the Gemini CLI parser.
// Reads sessions from JSON files in ~/.gemini/tmp/*/chats/session-*.json
package parser

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kuk1song/slashstage/internal/model"
)

func init() {
	Register(model.AgentGemini, &GeminiParser{})
}

// GeminiParser reads sessions from Gemini CLI JSON files.
type GeminiParser struct{}

// Discover finds all Gemini CLI session files.
func (p *GeminiParser) Discover(config model.AgentConfig) ([]DiscoveredFile, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
	}
	geminiDir := filepath.Join(home, ".gemini")
	if !DirExists(geminiDir) {
		return nil, nil
	}

	var files []DiscoveredFile

	// Walk ~/.gemini/ looking for session JSON files
	// Pattern: ~/.gemini/tmp/*/chats/session-*.json
	//    or:   ~/.gemini/sessions/*.json
	err = filepath.Walk(geminiDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// Skip antigravity directory (handled by separate parser)
		if info.IsDir() && info.Name() == "antigravity" {
			return filepath.SkipDir
		}
		if !info.IsDir() && strings.HasSuffix(path, ".json") && strings.Contains(path, "chat") {
			files = append(files, DiscoveredFile{
				Path:      path,
				AgentType: model.AgentGemini,
			})
		}
		return nil
	})

	return files, err
}

// Parse reads a Gemini CLI session JSON file.
func (p *GeminiParser) Parse(path string) ([]model.ParseResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))

	// Try to parse as a session object
	var session geminiSession
	if err := json.Unmarshal(data, &session); err != nil {
		// Try as array of messages
		var msgs []geminiMessage
		if err := json.Unmarshal(data, &msgs); err != nil {
			return nil, nil
		}
		session.Messages = msgs
	}

	if len(session.Messages) == 0 {
		return nil, nil
	}

	sessionID := strings.TrimSuffix(filepath.Base(path), ".json")

	result := model.Session{
		ID:         "gemini-" + sessionID,
		AgentType:  string(model.AgentGemini),
		Workspace:  session.Cwd,
		Model:      session.Model,
		SourcePath: path,
		SourceHash: hash,
		StartedAt:    GetFileModTime(path),
		LastActiveAt: GetFileModTime(path),
	}

	var messages []model.Message
	for i, msg := range session.Messages {
		content := extractGeminiContent(msg.Parts)
		if content == "" {
			continue
		}

		role := normalizeGeminiRole(msg.Role)

		if result.Title == "" && role == model.RoleUser {
			title := content
			if len(title) > 100 {
				title = title[:100] + "..."
			}
			result.Title = title
		}

		messages = append(messages, model.Message{
			SessionID: result.ID,
			Role:      role,
			Content:   content,
			SortOrder: i,
		})
	}

	result.MessageCount = len(messages)
	if len(messages) == 0 {
		return nil, nil
	}

	return []model.ParseResult{{Session: result, Messages: messages}}, nil
}

// --- Internal types ---

type geminiSession struct {
	Messages []geminiMessage `json:"messages"`
	Cwd      string          `json:"cwd"`
	Model    string          `json:"model"`
}

type geminiMessage struct {
	Role  string            `json:"role"`
	Parts []json.RawMessage `json:"parts"`
}

// --- Helpers ---

func extractGeminiContent(parts []json.RawMessage) string {
	var texts []string
	for _, part := range parts {
		// Try as string
		var str string
		if json.Unmarshal(part, &str) == nil {
			texts = append(texts, str)
			continue
		}

		// Try as object with text field
		var obj struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(part, &obj) == nil && obj.Text != "" {
			texts = append(texts, obj.Text)
		}
	}
	return strings.Join(texts, "\n")
}

func normalizeGeminiRole(role string) model.MessageRole {
	switch strings.ToLower(role) {
	case "user":
		return model.RoleUser
	case "model", "assistant":
		return model.RoleAssistant
	case "tool":
		return model.RoleTool
	default:
		return model.RoleAssistant
	}
}
