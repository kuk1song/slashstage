// Package parser — cursor_ide.go implements the Cursor 2 IDE parser.
// Reads sessions from state.vscdb SQLite database.
//
// Cursor stores conversations in SQLite databases:
// - macOS: ~/Library/Application Support/Cursor/User/globalStorage/state.vscdb
// - Linux: ~/.config/Cursor/User/globalStorage/state.vscdb
//
// Two tables:
// - cursorDiskKV (v0.40+): composerData:<uuid> → session metadata
//                          bubbleId:<composerId>:<bubbleId> → individual messages
// - ItemTable (legacy v0.2x-v0.3x): older format
//
// Ported from casr src/providers/cursor.rs (MIT License).
package parser

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kuk1song/slashstage/internal/model"

	_ "modernc.org/sqlite"
)

func init() {
	Register(model.AgentCursorIDE, &CursorIDEParser{})
}

// CursorIDEParser reads sessions from Cursor's state.vscdb SQLite database.
type CursorIDEParser struct{}

// Discover finds all state.vscdb files under the Cursor config directory.
func (p *CursorIDEParser) Discover(config model.AgentConfig) ([]DiscoveredFile, error) {
	configDir := cursorConfigDir()
	if configDir == "" {
		return nil, nil
	}

	var files []DiscoveredFile

	// Global storage DB (most common)
	globalDB := filepath.Join(configDir, "User", "globalStorage", "state.vscdb")
	if FileExists(globalDB) {
		files = append(files, DiscoveredFile{Path: globalDB, AgentType: model.AgentCursorIDE})
	}

	// Workspace-specific DBs
	wsDir := filepath.Join(configDir, "User", "workspaceStorage")
	if DirExists(wsDir) {
		entries, err := os.ReadDir(wsDir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					candidate := filepath.Join(wsDir, entry.Name(), "state.vscdb")
					if FileExists(candidate) {
						files = append(files, DiscoveredFile{Path: candidate, AgentType: model.AgentCursorIDE})
					}
				}
			}
		}
	}

	return files, nil
}

// Parse reads a state.vscdb file and extracts all sessions.
func (p *CursorIDEParser) Parse(path string) ([]model.ParseResult, error) {
	// Compute hash for change detection
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))

	// Open SQLite read-only
	db, err := sql.Open("sqlite", path+"?mode=ro&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	defer db.Close()

	// Try modern format first (cursorDiskKV), then legacy (ItemTable)
	results, err := parseCursorModern(db, path, hash)
	if err != nil || len(results) == 0 {
		results, err = parseCursorLegacy(db, path, hash)
	}

	return results, err
}

// parseCursorModern reads from cursorDiskKV table (v0.40+).
func parseCursorModern(db *sql.DB, sourcePath, sourceHash string) ([]model.ParseResult, error) {
	// Check if table exists
	var tableName string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='cursorDiskKV'`).Scan(&tableName)
	if err != nil {
		return nil, nil // Table doesn't exist, try legacy
	}

	// Find all composerData entries (each is a session)
	rows, err := db.Query(`SELECT key, value FROM cursorDiskKV WHERE key LIKE 'composerData:%'`)
	if err != nil {
		return nil, fmt.Errorf("query composerData: %w", err)
	}
	defer rows.Close()

	var results []model.ParseResult

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}

		// Extract session ID from key "composerData:<uuid>"
		sessionID := strings.TrimPrefix(key, "composerData:")
		if sessionID == "" {
			continue
		}

		// Parse session metadata JSON
		var meta cursorSessionMeta
		if err := json.Unmarshal([]byte(value), &meta); err != nil {
			continue
		}

		// Build session
		session := model.Session{
			ID:         sessionID,
			AgentType:  string(model.AgentCursorIDE),
			Title:      extractCursorTitle(&meta),
			Workspace:  extractCursorWorkspace(&meta),
			Model:      extractCursorModel(meta.ModelConfig),
			SourcePath: sourcePath,
			SourceHash: sourceHash,
			StartedAt:  parseCursorTimestamp(meta.CreatedAt),
			LastActiveAt: parseCursorTimestamp(meta.LastUpdated),
		}

		// Fetch individual messages (bubbleId entries)
		messages := fetchCursorBubbles(db, sessionID, meta.BubbleHeaders)
		session.MessageCount = len(messages)

		results = append(results, model.ParseResult{
			Session:  session,
			Messages: messages,
		})
	}

	return results, rows.Err()
}

// fetchCursorBubbles reads individual message bubbles for a session.
// It uses the fullConversationHeadersOnly ordering from the composerData.
func fetchCursorBubbles(db *sql.DB, composerID string, headers json.RawMessage) []model.Message {
	// Parse bubble ordering from headers
	var bubbleOrder []struct {
		BubbleID string `json:"bubbleId"`
		Type     int    `json:"type"`
	}
	json.Unmarshal(headers, &bubbleOrder)

	// If no ordering, fall back to key scan
	if len(bubbleOrder) == 0 {
		return fetchCursorBubblesUnordered(db, composerID)
	}

	var messages []model.Message
	order := 0

	for _, header := range bubbleOrder {
		key := fmt.Sprintf("bubbleId:%s:%s", composerID, header.BubbleID)
		var value string
		if err := db.QueryRow(`SELECT value FROM cursorDiskKV WHERE key = ?`, key).Scan(&value); err != nil {
			continue
		}

		var bubble cursorBubble
		if err := json.Unmarshal([]byte(value), &bubble); err != nil {
			continue
		}

		role := cursorBubbleRole(bubble.Type)
		content := extractBubbleContent(&bubble)

		// Skip completely empty bubbles (but keep tool calls with toolFormerData)
		if content == "" && len(bubble.ToolFormerData) == 0 {
			continue
		}

		msg := model.Message{
			SessionID: composerID,
			Role:      role,
			Content:   content,
			SortOrder: order,
		}

		// Parse timestamp
		if bubble.CreatedAt != "" {
			if t, err := time.Parse(time.RFC3339, bubble.CreatedAt); err == nil {
				msg.CreatedAt = t
			} else if t, err := time.Parse("2006-01-02T15:04:05.000Z", bubble.CreatedAt); err == nil {
				msg.CreatedAt = t
			}
		}

		// Extract tool call info
		if len(bubble.ToolFormerData) > 0 {
			var tool struct {
				Tool       int    `json:"tool"`
				ToolCallID string `json:"toolCallId"`
				Status     string `json:"status"`
			}
			if json.Unmarshal(bubble.ToolFormerData, &tool) == nil {
				msg.ToolName = fmt.Sprintf("tool_%d", tool.Tool)
				msg.Role = model.RoleTool
			}
		}

		messages = append(messages, msg)
		order++
	}

	return messages
}

// fetchCursorBubblesUnordered is a fallback when no bubble ordering is available.
func fetchCursorBubblesUnordered(db *sql.DB, composerID string) []model.Message {
	prefix := fmt.Sprintf("bubbleId:%s:", composerID)
	rows, err := db.Query(`SELECT value FROM cursorDiskKV WHERE key LIKE ? ORDER BY key`, prefix+"%")
	if err != nil {
		return nil
	}
	defer rows.Close()

	var messages []model.Message
	order := 0

	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			continue
		}

		var bubble cursorBubble
		if err := json.Unmarshal([]byte(value), &bubble); err != nil {
			continue
		}

		content := extractBubbleContent(&bubble)
		if content == "" {
			continue
		}

		messages = append(messages, model.Message{
			SessionID: composerID,
			Role:      cursorBubbleRole(bubble.Type),
			Content:   content,
			SortOrder: order,
		})
		order++
	}

	return messages
}

// parseCursorLegacy reads from ItemTable (v0.2x-v0.3x format).
func parseCursorLegacy(db *sql.DB, sourcePath, sourceHash string) ([]model.ParseResult, error) {
	var tableName string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='ItemTable'`).Scan(&tableName)
	if err != nil {
		return nil, nil // Table doesn't exist
	}

	rows, err := db.Query(`SELECT key, value FROM ItemTable WHERE key LIKE '%aiChat%' OR key LIKE '%composer%'`)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()

	var results []model.ParseResult

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}

		// Try to parse as a chat session
		var legacy cursorLegacyChat
		if err := json.Unmarshal([]byte(value), &legacy); err != nil {
			continue
		}

		if len(legacy.Messages) == 0 && len(legacy.Tabs) == 0 {
			continue
		}

		sessionID := key
		session := model.Session{
			ID:         sessionID,
			AgentType:  string(model.AgentCursorIDE),
			Title:      extractLegacyTitle(&legacy),
			SourcePath: sourcePath,
			SourceHash: sourceHash,
			StartedAt:  time.Now(),
			LastActiveAt: time.Now(),
		}

		var messages []model.Message
		order := 0

		// Parse from Messages array
		for _, msg := range legacy.Messages {
			role := normalizeLegacyRole(msg.Role)
			content := msg.Content
			if content == "" {
				content = msg.Text
			}
			if content == "" {
				continue
			}
			messages = append(messages, model.Message{
				SessionID: sessionID,
				Role:      role,
				Content:   content,
				SortOrder: order,
			})
			order++
		}

		// Parse from Tabs (older format)
		for _, tab := range legacy.Tabs {
			for _, bubble := range tab.Bubbles {
				role := normalizeLegacyRole(bubble.Role)
				content := bubble.Content
				if content == "" {
					content = bubble.Text
				}
				if content == "" {
					continue
				}
				messages = append(messages, model.Message{
					SessionID: sessionID,
					Role:      role,
					Content:   content,
					SortOrder: order,
				})
				order++
			}
		}

		session.MessageCount = len(messages)
		if len(messages) > 0 {
			results = append(results, model.ParseResult{
				Session:  session,
				Messages: messages,
			})
		}
	}

	return results, nil
}

// --- Internal types ---

type cursorSessionMeta struct {
	ComposerID   string          `json:"composerId"`
	Name         string          `json:"name"`
	Subtitle     string          `json:"subtitle"`
	CreatedAt    json.RawMessage `json:"createdAt"`
	LastUpdated  json.RawMessage `json:"lastUpdatedAt"`
	Status       string          `json:"status"`
	ModelConfig  json.RawMessage `json:"modelConfig"`
	FileURIs     []string        `json:"allAttachedFileCodeChunksUris"`
	CodeBlockData json.RawMessage `json:"codeBlockData"`
	OriginalFiles json.RawMessage `json:"originalFileStates"`
	BubbleHeaders json.RawMessage `json:"fullConversationHeadersOnly"`
}

type cursorBubble struct {
	Type           json.RawMessage `json:"type"`      // 1=user, 2=assistant
	BubbleID       string          `json:"bubbleId"`
	Text           string          `json:"text"`
	RawText        string          `json:"rawText"`
	Content        string          `json:"content"`
	Message        string          `json:"message"`
	Role           string          `json:"role"`
	CreatedAt      string          `json:"createdAt"`
	Thinking       *struct {
		Text string `json:"text"`
	} `json:"thinking"`
	ToolFormerData json.RawMessage `json:"toolFormerData"`
	ModelInfo      *struct {
		ModelName string `json:"modelName"`
	} `json:"modelInfo"`
}

type cursorLegacyChat struct {
	Messages []cursorLegacyMessage `json:"messages"`
	Tabs     []cursorLegacyTab     `json:"tabs"`
}

type cursorLegacyMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Text    string `json:"text"`
}

type cursorLegacyTab struct {
	Bubbles []cursorLegacyMessage `json:"bubbles"`
}

// --- Helper functions ---

func cursorConfigDir() string {
	// Respect CURSOR_HOME env var
	if home := os.Getenv("CURSOR_HOME"); home != "" {
		return home
	}
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "Cursor")
	case "linux":
		if cfg, err := os.UserConfigDir(); err == nil {
			return filepath.Join(cfg, "Cursor")
		}
	}
	// Windows and others
	if cfg, err := os.UserConfigDir(); err == nil {
		return filepath.Join(cfg, "Cursor")
	}
	return ""
}

func extractCursorTitle(meta *cursorSessionMeta) string {
	if meta.Name != "" {
		return meta.Name
	}
	if meta.Subtitle != "" {
		return meta.Subtitle
	}
	return ""
}

// extractCursorWorkspace extracts the workspace path from file URIs in the session.
// Cursor stores file URIs in allAttachedFileCodeChunksUris, codeBlockData, and originalFileStates.
//
// Strategy: collect ALL file paths from all sources, find git roots, and pick
// the most common valid workspace. This avoids the old bug of using the FIRST
// random file URI (which might point to a Cursor config dir).
func extractCursorWorkspace(meta *cursorSessionMeta) string {
	// Step 1: Collect all file paths from all sources
	var allPaths []string

	for _, uri := range meta.FileURIs {
		if p := fileURIToPath(uri); p != "" {
			allPaths = append(allPaths, filepath.Dir(p))
		}
	}

	if len(meta.CodeBlockData) > 0 {
		var codeBlocks map[string]json.RawMessage
		if json.Unmarshal(meta.CodeBlockData, &codeBlocks) == nil {
			for uri := range codeBlocks {
				if p := fileURIToPath(uri); p != "" {
					allPaths = append(allPaths, filepath.Dir(p))
				}
			}
		}
	}

	if len(meta.OriginalFiles) > 0 {
		var origFiles map[string]json.RawMessage
		if json.Unmarshal(meta.OriginalFiles, &origFiles) == nil {
			for uri := range origFiles {
				if p := fileURIToPath(uri); p != "" {
					allPaths = append(allPaths, filepath.Dir(p))
				}
			}
		}
	}

	if len(allPaths) == 0 {
		return ""
	}

	// Step 2: Find git roots for all paths, count occurrences
	gitRootCount := make(map[string]int)
	dirCount := make(map[string]int)

	for _, p := range allPaths {
		if root := FindGitRoot(p); root != "" && IsValidWorkspace(root) {
			gitRootCount[root]++
		} else if IsValidWorkspace(p) {
			dirCount[p]++
		}
	}

	// Step 3: Pick the most frequent git root (most files come from this project)
	if len(gitRootCount) > 0 {
		var bestRoot string
		var bestCount int
		for root, count := range gitRootCount {
			if count > bestCount {
				bestRoot = root
				bestCount = count
			}
		}
		return bestRoot
	}

	// Step 4: Fallback to most frequent valid directory
	if len(dirCount) > 0 {
		var bestDir string
		var bestCount int
		for dir, count := range dirCount {
			if count > bestCount {
				bestDir = dir
				bestCount = count
			}
		}
		return bestDir
	}

	return ""
}

// fileURIToPath converts a file:// URI to an absolute path.
func fileURIToPath(uri string) string {
	if strings.HasPrefix(uri, "file://") {
		return strings.TrimPrefix(uri, "file://")
	}
	return ""
}

// extractCursorModel extracts the model name from modelConfig JSON.
func extractCursorModel(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var config struct {
		ModelName string `json:"modelName"`
	}
	if json.Unmarshal(raw, &config) == nil {
		return config.ModelName
	}
	return ""
}

func extractLegacyTitle(chat *cursorLegacyChat) string {
	// Use first user message as title
	for _, msg := range chat.Messages {
		if msg.Role == "user" || msg.Role == "human" {
			title := msg.Content
			if title == "" {
				title = msg.Text
			}
			if len(title) > 100 {
				title = title[:100] + "..."
			}
			return title
		}
	}
	return ""
}

// cursorBubbleRole determines the message role from bubble type.
// Modern format: 1 = user, 2 = assistant
// String format: "user"/"human" = user, "assistant"/"ai"/"bot" = assistant
func cursorBubbleRole(typeRaw json.RawMessage) model.MessageRole {
	if len(typeRaw) == 0 {
		return model.RoleAssistant
	}

	// Try as integer
	var typeInt int
	if json.Unmarshal(typeRaw, &typeInt) == nil {
		switch typeInt {
		case 1:
			return model.RoleUser
		case 2:
			return model.RoleAssistant
		}
	}

	// Try as string
	var typeStr string
	if json.Unmarshal(typeRaw, &typeStr) == nil {
		return normalizeLegacyRole(typeStr)
	}

	return model.RoleAssistant
}

func normalizeLegacyRole(role string) model.MessageRole {
	switch strings.ToLower(role) {
	case "user", "human":
		return model.RoleUser
	case "assistant", "ai", "bot":
		return model.RoleAssistant
	case "tool", "function":
		return model.RoleTool
	case "system":
		return model.RoleSystem
	default:
		return model.RoleAssistant
	}
}

func extractBubbleContent(b *cursorBubble) string {
	// Priority: text > rawText > content > message > thinking
	if b.Text != "" {
		return b.Text
	}
	if b.RawText != "" {
		return b.RawText
	}
	if b.Content != "" {
		return b.Content
	}
	if b.Message != "" {
		return b.Message
	}
	// For assistant thinking bubbles, use thinking text
	if b.Thinking != nil && b.Thinking.Text != "" {
		return "[thinking] " + b.Thinking.Text
	}
	return ""
}

func parseCursorTimestamp(raw json.RawMessage) time.Time {
	if len(raw) == 0 {
		return time.Now()
	}

	// Try as Unix milliseconds (number)
	var ms int64
	if json.Unmarshal(raw, &ms) == nil && ms > 0 {
		return time.UnixMilli(ms)
	}

	// Try as string (ISO 8601)
	var str string
	if json.Unmarshal(raw, &str) == nil && str != "" {
		if t, err := time.Parse(time.RFC3339, str); err == nil {
			return t
		}
		if t, err := time.Parse("2006-01-02T15:04:05.000Z", str); err == nil {
			return t
		}
	}

	return time.Now()
}
