// Package model defines the core data types for SlashStage.
package model

import "time"

// Project represents a user's coding project tracked by SlashStage.
type Project struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`       // Absolute path on disk
	GitRemote string    `json:"git_remote"` // e.g. "git@github.com:kuk1song/slashstage.git"
	GitBranch string    `json:"git_branch"` // e.g. "main"
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Session represents a coding session from any IDE/CLI.
type Session struct {
	ID            string    `json:"id"`             // Unique session ID (from IDE or generated)
	ProjectID     *int64    `json:"project_id"`     // nil = unassigned
	AgentType     string    `json:"agent_type"`     // "cursor-ide", "claude", "codex", etc.
	Title         string    `json:"title"`          // Session title or first message summary
	Workspace     string    `json:"workspace"`      // Workspace path extracted from session
	Model         string    `json:"model"`          // Model used (e.g. "claude-3.5-sonnet")
	MessageCount  int       `json:"message_count"`  // Total messages in session
	TokensIn      int64     `json:"tokens_in"`      // Input tokens (if available)
	TokensOut     int64     `json:"tokens_out"`     // Output tokens (if available)
	StartedAt     time.Time `json:"started_at"`     // When session started
	LastActiveAt  time.Time `json:"last_active_at"` // Last activity
	SourcePath    string    `json:"source_path"`    // Where the session file lives on disk
	SourceHash    string    `json:"source_hash"`    // SHA-256 of source file for change detection
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// MessageRole represents who sent a message.
type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
	RoleSystem    MessageRole = "system"
)

// Message represents a single message within a session.
type Message struct {
	ID        int64       `json:"id"`
	SessionID string      `json:"session_id"`
	Role      MessageRole `json:"role"`
	Content   string      `json:"content"`
	ToolName  string      `json:"tool_name,omitempty"`  // For tool calls
	ToolInput string      `json:"tool_input,omitempty"` // Tool call input (JSON)
	TokensIn  int64       `json:"tokens_in"`
	TokensOut int64       `json:"tokens_out"`
	CreatedAt time.Time   `json:"created_at"`
	SortOrder int         `json:"sort_order"` // Ordering within session
}

// AgentType identifies which IDE/CLI a session comes from.
type AgentType string

const (
	AgentCursorIDE  AgentType = "cursor-ide"
	AgentCursorCLI  AgentType = "cursor-cli"
	AgentClaude     AgentType = "claude"
	AgentCodex      AgentType = "codex"
	AgentGemini     AgentType = "gemini"
	AgentOpenCode   AgentType = "opencode"
	AgentAntigravity AgentType = "antigravity"
	AgentCopilot    AgentType = "copilot"
	AgentVSCodeCopilot AgentType = "vscode-copilot"
)

// AgentConfig declares how to discover sessions for a specific IDE/CLI.
type AgentConfig struct {
	Type        AgentType `json:"type"`
	DisplayName string    `json:"display_name"`
	DefaultDirs []string  `json:"default_dirs"` // Relative to $HOME
	EnvVar      string    `json:"env_var"`       // Env var to override path
	Format      string    `json:"format"`        // "sqlite", "jsonl", "json", "protobuf"
}

// ParseResult is what a Parser returns after parsing a session file.
type ParseResult struct {
	Session  Session   `json:"session"`
	Messages []Message `json:"messages"`
}
