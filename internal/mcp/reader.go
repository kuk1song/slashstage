// Package mcp reads MCP server configurations from various IDE config files.
package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// MCPServer represents a configured MCP server.
type MCPServer struct {
	Name    string            `json:"name"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	URL     string            `json:"url,omitempty"`
	Type    string            `json:"type,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Source  string            `json:"source"` // "cursor", "claude", etc.
}

// ReadAllMCPConfigs reads MCP configurations from all known IDE config locations.
func ReadAllMCPConfigs() ([]MCPServer, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	var all []MCPServer

	// Cursor MCP config
	cursorMCP := filepath.Join(home, ".cursor", "mcp.json")
	if servers, err := readCursorMCP(cursorMCP); err == nil {
		all = append(all, servers...)
	}

	// Claude Code MCP config
	claudeMCP := filepath.Join(home, ".claude", "mcp.json")
	if servers, err := readCursorMCP(claudeMCP); err == nil { // Same format
		for i := range servers {
			servers[i].Source = "claude"
		}
		all = append(all, servers...)
	}

	// Codex MCP config
	codexMCP := filepath.Join(home, ".codex", "mcp.json")
	if servers, err := readCursorMCP(codexMCP); err == nil {
		for i := range servers {
			servers[i].Source = "codex"
		}
		all = append(all, servers...)
	}

	return all, nil
}

// ReadMCPConfig reads a single MCP config file.
func ReadMCPConfig(path, source string) ([]MCPServer, error) {
	servers, err := readCursorMCP(path)
	if err != nil {
		return nil, err
	}
	for i := range servers {
		servers[i].Source = source
	}
	return servers, nil
}

// readCursorMCP reads Cursor-style MCP config JSON.
// Format: { "mcpServers": { "name": { "command": "...", "args": [...] } } }
func readCursorMCP(path string) ([]MCPServer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	var servers []MCPServer
	for name, raw := range config.MCPServers {
		var server MCPServer
		if err := json.Unmarshal(raw, &server); err != nil {
			continue
		}
		server.Name = name
		server.Source = "cursor"
		servers = append(servers, server)
	}

	return servers, nil
}
