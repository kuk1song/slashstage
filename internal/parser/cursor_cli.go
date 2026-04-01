// Package parser — cursor_cli.go implements the Cursor CLI Agent parser.
// Reads sessions from agent-transcripts JSONL/TXT files in
// ~/.cursor/projects/<project-slug>/agent-transcripts/<uuid>/<uuid>.jsonl
package parser

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kuk1song/slashstage/internal/model"
)

func init() {
	Register(model.AgentCursorCLI, &CursorCLIParser{})
}

// CursorCLIParser reads sessions from Cursor CLI agent-transcripts.
type CursorCLIParser struct{}

// Discover finds all Cursor CLI agent-transcript files.
func (p *CursorCLIParser) Discover(config model.AgentConfig) ([]DiscoveredFile, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
	}
	projectsDir := filepath.Join(home, ".cursor", "projects")
	if !DirExists(projectsDir) {
		return nil, nil
	}

	var files []DiscoveredFile

	projectDirs, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil, nil
	}

	for _, projDir := range projectDirs {
		if !projDir.IsDir() {
			continue
		}
		transcriptsDir := filepath.Join(projectsDir, projDir.Name(), "agent-transcripts")
		if !DirExists(transcriptsDir) {
			continue
		}

		// Walk agent-transcripts/<uuid>/<uuid>.jsonl or .txt
		err := filepath.Walk(transcriptsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if !info.IsDir() && (strings.HasSuffix(path, ".jsonl") || strings.HasSuffix(path, ".txt")) {
				files = append(files, DiscoveredFile{
					Path:      path,
					AgentType: model.AgentCursorCLI,
				})
			}
			return nil
		})
		if err != nil {
			continue
		}
	}

	return files, nil
}

// Parse reads a Cursor CLI agent-transcript file.
func (p *CursorCLIParser) Parse(path string) ([]model.ParseResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))

	// Extract workspace from path: ~/.cursor/projects/<encoded-path>/agent-transcripts/...
	workspace := extractCursorCLIWorkspace(path)

	ext := filepath.Ext(path)
	sessionID := strings.TrimSuffix(filepath.Base(path), ext)

	session := model.Session{
		ID:         "cursor-cli-" + sessionID,
		AgentType:  string(model.AgentCursorCLI),
		Workspace:  workspace,
		SourcePath: path,
		SourceHash: hash,
		StartedAt:    GetFileModTime(path),
		LastActiveAt: GetFileModTime(path),
	}

	var messages []model.Message

	if ext == ".jsonl" {
		messages = parseCursorCLIJsonl(path, session.ID)
	} else {
		messages = parseCursorCLIText(path, session.ID)
	}

	session.MessageCount = len(messages)
	if len(messages) == 0 {
		return nil, nil
	}

	// Title from first user message
	for _, m := range messages {
		if m.Role == model.RoleUser {
			title := m.Content
			if len(title) > 100 {
				title = title[:100] + "..."
			}
			session.Title = title
			break
		}
	}

	return []model.ParseResult{{Session: session, Messages: messages}}, nil
}

func parseCursorCLIJsonl(path, sessionID string) []model.Message {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

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

		var role string
		if raw, ok := entry["role"]; ok {
			json.Unmarshal(raw, &role)
		}
		if role == "" {
			// Try type field
			if raw, ok := entry["type"]; ok {
				json.Unmarshal(raw, &role)
			}
		}

		content := extractJSONContent(entry["content"])
		if content == "" {
			if raw, ok := entry["text"]; ok {
				var text string
				json.Unmarshal(raw, &text)
				content = text
			}
		}
		if content == "" {
			continue
		}

		messages = append(messages, model.Message{
			SessionID: sessionID,
			Role:      normalizeLegacyRole(role),
			Content:   content,
			SortOrder: order,
		})
		order++
	}
	return messages
}

func parseCursorCLIText(path, sessionID string) []model.Message {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var messages []model.Message
	order := 0
	var currentRole model.MessageRole
	var currentContent strings.Builder

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		// Detect role markers
		if strings.HasPrefix(line, "Human:") || strings.HasPrefix(line, "User:") {
			// Flush previous
			if currentContent.Len() > 0 {
				messages = append(messages, model.Message{
					SessionID: sessionID,
					Role:      currentRole,
					Content:   strings.TrimSpace(currentContent.String()),
					SortOrder: order,
				})
				order++
			}
			currentRole = model.RoleUser
			currentContent.Reset()
			currentContent.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "Human:"), "User:"))
		} else if strings.HasPrefix(line, "Assistant:") || strings.HasPrefix(line, "AI:") {
			if currentContent.Len() > 0 {
				messages = append(messages, model.Message{
					SessionID: sessionID,
					Role:      currentRole,
					Content:   strings.TrimSpace(currentContent.String()),
					SortOrder: order,
				})
				order++
			}
			currentRole = model.RoleAssistant
			currentContent.Reset()
			currentContent.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "Assistant:"), "AI:"))
		} else {
			if currentContent.Len() > 0 {
				currentContent.WriteString("\n")
			}
			currentContent.WriteString(line)
		}
	}

	// Flush last message
	if currentContent.Len() > 0 {
		messages = append(messages, model.Message{
			SessionID: sessionID,
			Role:      currentRole,
			Content:   strings.TrimSpace(currentContent.String()),
			SortOrder: order,
		})
	}

	return messages
}

func extractCursorCLIWorkspace(path string) string {
	// Path: ~/.cursor/projects/<encoded-path>/agent-transcripts/...
	// Walk up to find the projects/<encoded-path> component
	parts := strings.Split(path, string(filepath.Separator))
	for i, part := range parts {
		if part == "projects" && i+1 < len(parts) {
			return DecodeEncodedPath(parts[i+1])
		}
	}
	return ""
}
