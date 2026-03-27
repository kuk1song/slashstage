// Package parser — copilot.go implements the Copilot CLI parser.
// Reads sessions from JSON files in ~/.copilot/session-state/
package parser

import (
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
	Register(model.AgentCopilot, &CopilotParser{})
}

// CopilotParser reads sessions from Copilot CLI JSON files.
type CopilotParser struct{}

// Discover finds all Copilot CLI session files.
func (p *CopilotParser) Discover(config model.AgentConfig) ([]DiscoveredFile, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
	}

	// Try multiple known locations
	dirs := []string{
		filepath.Join(home, ".copilot", "session-state"),
		filepath.Join(home, ".copilot"),
	}

	var files []DiscoveredFile
	for _, dir := range dirs {
		if !DirExists(dir) {
			continue
		}
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if !info.IsDir() && strings.HasSuffix(path, ".json") {
				files = append(files, DiscoveredFile{
					Path:      path,
					AgentType: model.AgentCopilot,
				})
			}
			return nil
		})
		if err != nil {
			continue
		}
		if len(files) > 0 {
			break
		}
	}

	return files, nil
}

// Parse reads a Copilot CLI session JSON file.
func (p *CopilotParser) Parse(path string) ([]model.ParseResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))

	var session copilotSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, nil
	}

	if len(session.Messages) == 0 && len(session.Turns) == 0 {
		return nil, nil
	}

	sessionID := strings.TrimSuffix(filepath.Base(path), ".json")

	result := model.Session{
		ID:         "copilot-" + sessionID,
		AgentType:  string(model.AgentCopilot),
		Workspace:  session.Workspace,
		SourcePath: path,
		SourceHash: hash,
		StartedAt:  time.Now(),
		LastActiveAt: time.Now(),
	}

	var messages []model.Message
	order := 0

	// Parse Messages array
	for _, msg := range session.Messages {
		content := msg.Content
		if content == "" {
			content = msg.Text
		}
		if content == "" {
			continue
		}

		role := normalizeLegacyRole(msg.Role)
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
			SortOrder: order,
		})
		order++
	}

	// Parse Turns array (alternative format)
	for _, turn := range session.Turns {
		for _, msg := range turn.Messages {
			content := msg.Content
			if content == "" {
				content = msg.Text
			}
			if content == "" {
				continue
			}

			role := normalizeLegacyRole(msg.Role)
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
				SortOrder: order,
			})
			order++
		}
	}

	result.MessageCount = len(messages)
	if len(messages) == 0 {
		return nil, nil
	}

	return []model.ParseResult{{Session: result, Messages: messages}}, nil
}

// --- Internal types ---

type copilotSession struct {
	Messages  []copilotMessage `json:"messages"`
	Turns     []copilotTurn    `json:"turns"`
	Workspace string           `json:"workspace"`
}

type copilotTurn struct {
	Messages []copilotMessage `json:"messages"`
}

type copilotMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Text    string `json:"text"`
}
