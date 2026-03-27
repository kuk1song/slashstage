// Package parser provides the pluggable parser framework for SlashStage.
// Each IDE/CLI implements the Parser interface. New tools are added by
// registering an AgentConfig + implementing Discover() and Parse().
package parser

import (
	"os"
	"path/filepath"

	"github.com/kuk1song/slashstage/internal/model"
)

// DiscoveredFile represents a session source file found on disk.
type DiscoveredFile struct {
	Path      string          // Absolute path to the session file/database
	AgentType model.AgentType // Which IDE/CLI this belongs to
}

// Parser is the interface every IDE/CLI parser must implement.
type Parser interface {
	// Discover finds all session source files for this IDE/CLI on the local machine.
	Discover(config model.AgentConfig) ([]DiscoveredFile, error)

	// Parse reads a single session source file and returns parsed sessions + messages.
	// A single file may contain multiple sessions (e.g. state.vscdb).
	Parse(path string) ([]model.ParseResult, error)
}

// Registry holds all registered IDE/CLI configurations.
var Registry = []model.AgentConfig{
	{
		Type:        model.AgentCursorIDE,
		DisplayName: "Cursor 2 IDE",
		DefaultDirs: []string{"Library/Application Support/Cursor/User/globalStorage"},
		Format:      "sqlite",
	},
	{
		Type:        model.AgentCursorCLI,
		DisplayName: "Cursor CLI Agent",
		DefaultDirs: []string{".cursor/projects"},
		Format:      "jsonl",
	},
	{
		Type:        model.AgentClaude,
		DisplayName: "Claude Code",
		DefaultDirs: []string{".claude/projects"},
		Format:      "jsonl",
	},
	{
		Type:        model.AgentCodex,
		DisplayName: "Codex CLI",
		DefaultDirs: []string{".codex/sessions"},
		Format:      "jsonl",
	},
	{
		Type:        model.AgentGemini,
		DisplayName: "Gemini CLI",
		DefaultDirs: []string{".gemini"},
		Format:      "json",
	},
	{
		Type:        model.AgentOpenCode,
		DisplayName: "OpenCode",
		DefaultDirs: []string{".local/share/opencode"},
		Format:      "sqlite",
	},
	{
		Type:        model.AgentAntigravity,
		DisplayName: "Antigravity",
		DefaultDirs: []string{".gemini/antigravity"},
		Format:      "protobuf",
	},
	{
		Type:        model.AgentCopilot,
		DisplayName: "Copilot CLI",
		DefaultDirs: []string{".copilot"},
		Format:      "json",
	},
}

// parsers maps agent types to their parser implementations.
var parsers = map[model.AgentType]Parser{}

// Register adds a parser implementation for an agent type.
func Register(agentType model.AgentType, p Parser) {
	parsers[agentType] = p
}

// GetParser returns the parser for an agent type, or nil if not registered.
func GetParser(agentType model.AgentType) Parser {
	return parsers[agentType]
}

// GetConfig returns the AgentConfig for an agent type.
func GetConfig(agentType model.AgentType) *model.AgentConfig {
	for i := range Registry {
		if Registry[i].Type == agentType {
			return &Registry[i]
		}
	}
	return nil
}

// ResolveDir expands a default directory (relative to $HOME) to an absolute path.
func ResolveDir(relDir string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, relDir)
}

// FileExists checks if a file exists and is not a directory.
func FileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// DirExists checks if a directory exists.
func DirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
