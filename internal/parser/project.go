package parser

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kuk1song/slashstage/internal/model"
)

// GetProjectName extracts a human-readable project name from a workspace path.
// Priority: git remote name > git root dir name > directory name.
// Ported from agentsview internal/parser/project.go.
func GetProjectName(workspacePath string) string {
	if workspacePath == "" {
		return ""
	}

	// Try git remote name first
	if name := getGitRemoteName(workspacePath); name != "" {
		return name
	}

	// Try git root directory name
	if root := FindGitRoot(workspacePath); root != "" {
		return filepath.Base(root)
	}

	// Fallback: directory name
	return filepath.Base(workspacePath)
}

// IsValidWorkspace checks if a path looks like a real user project workspace.
// Returns false for IDE config dirs, temp dirs, home directory itself, etc.
func IsValidWorkspace(path string) bool {
	if path == "" {
		return false
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return path != ""
	}

	cleanPath := filepath.Clean(path)

	// Reject home directory itself
	if cleanPath == home {
		return false
	}

	// Reject paths inside IDE/tool config directories
	invalidPrefixes := []string{
		filepath.Join(home, ".cursor"),
		filepath.Join(home, ".config"),
		filepath.Join(home, ".local", "share", "Trash"),
		filepath.Join(home, "Library", "Application Support", "Cursor"),
		filepath.Join(home, "Library", "Application Support", "Code"),
		"/tmp",
		"/var/tmp",
		"/private/tmp",
	}

	for _, prefix := range invalidPrefixes {
		if strings.HasPrefix(cleanPath, prefix) {
			slog.Debug("rejected workspace path", "path", cleanPath, "reason", "inside "+prefix)
			return false
		}
	}

	// Reject if path doesn't exist on disk
	if _, err := os.Stat(cleanPath); os.IsNotExist(err) {
		slog.Debug("rejected workspace path", "path", cleanPath, "reason", "does not exist")
		return false
	}

	return true
}

// FindGitRoot finds the git repository root for a given path.
func FindGitRoot(path string) string {
	// Walk up to find .git directory
	current := path
	for {
		gitDir := filepath.Join(current, ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break // Reached filesystem root
		}
		current = parent
	}
	return ""
}

// getGitRemoteName extracts the repo name from git remote origin URL.
func getGitRemoteName(path string) string {
	cmd := exec.Command("git", "-C", path, "config", "--get", "remote.origin.url")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	url := strings.TrimSpace(string(out))
	return extractRepoName(url)
}

// extractRepoName parses a repo name from a git remote URL.
// Handles: git@github.com:user/repo.git, https://github.com/user/repo.git
func extractRepoName(url string) string {
	if url == "" {
		return ""
	}
	// Remove .git suffix
	url = strings.TrimSuffix(url, ".git")

	// SSH format: git@github.com:user/repo
	if idx := strings.LastIndex(url, ":"); idx >= 0 && strings.Contains(url, "@") {
		parts := strings.Split(url[idx+1:], "/")
		if len(parts) >= 1 {
			return parts[len(parts)-1]
		}
	}

	// HTTPS format: https://github.com/user/repo
	if idx := strings.LastIndex(url, "/"); idx >= 0 {
		return url[idx+1:]
	}

	return ""
}

// ExtractProjectFromCwd extracts a project path from a CWD string in session data.
func ExtractProjectFromCwd(cwd string) string {
	if cwd == "" {
		return ""
	}
	return filepath.Clean(cwd)
}

// DecodeEncodedPath decodes an IDE-encoded path.
// Cursor CLI encodes paths: "Users-kuki-openclaw" → "/Users/kuki/openclaw"
// Claude Code encodes paths: "Users-kuki-openclaw" → "/Users/kuki/openclaw"
//
// Strategy: try replacing dashes with slashes at different levels and verify
// which decoded path actually exists on disk. This handles folder names with dashes.
func DecodeEncodedPath(encoded string) string {
	if encoded == "" {
		return ""
	}

	// The encoded path strips the leading "/" and replaces all "/" with "-".
	// Simple decode: add leading "/" and replace "-" with "/".
	simple := "/" + strings.ReplaceAll(encoded, "-", "/")
	simple = filepath.Clean(simple)

	// If simple decode exists on disk, use it
	if _, err := os.Stat(simple); err == nil {
		return simple
	}

	// Smart decode: try to reconstruct the path by testing which splits exist.
	// Walk the parts and greedily match the longest existing prefix.
	parts := strings.Split(encoded, "-")
	if len(parts) == 0 {
		return simple
	}

	best := smartDecodePath(parts)
	if best != "" {
		return best
	}

	// Fallback to simple decode
	return simple
}

// smartDecodePath tries to reconstruct a filesystem path from dash-separated parts.
// It greedily builds path components, checking what exists on disk.
// E.g. ["Users","kuki","ot","PrivateGpt"] → tries "/Users" (exists) → "/Users/kuki" (exists) → etc.
func smartDecodePath(parts []string) string {
	if len(parts) == 0 {
		return ""
	}

	type candidate struct {
		path      string
		remaining []string
	}

	// BFS: try all possible splits
	queue := []candidate{{path: "", remaining: parts}}
	var bestPath string

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		if len(cur.remaining) == 0 {
			if cur.path != "" {
				bestPath = cur.path
			}
			continue
		}

		// Try joining 1, 2, 3... remaining parts as a single path component
		for joinLen := 1; joinLen <= len(cur.remaining); joinLen++ {
			component := strings.Join(cur.remaining[:joinLen], "-")
			var testPath string
			if cur.path == "" {
				testPath = "/" + component
			} else {
				testPath = cur.path + "/" + component
			}

			if _, err := os.Stat(testPath); err == nil {
				if joinLen == len(cur.remaining) {
					// All parts consumed, this is a valid complete path
					return filepath.Clean(testPath)
				}
				queue = append(queue, candidate{
					path:      testPath,
					remaining: cur.remaining[joinLen:],
				})
			}
		}
	}

	if bestPath != "" {
		return filepath.Clean(bestPath)
	}
	return ""
}

// BindSessionToProject matches a session workspace path to a registered project.
// Returns nil if no match found (session goes to "Unassigned").
func BindSessionToProject(sessionWorkspace string, projects []model.Project) *model.Project {
	if sessionWorkspace == "" {
		return nil
	}

	sessionPath := filepath.Clean(sessionWorkspace)

	// Step 1: Exact match
	for i := range projects {
		if filepath.Clean(projects[i].Path) == sessionPath {
			return &projects[i]
		}
	}

	// Step 2: Prefix match (session is in a subdirectory of project)
	var bestMatch *model.Project
	var bestLen int
	for i := range projects {
		projPath := filepath.Clean(projects[i].Path)
		if strings.HasPrefix(sessionPath, projPath+string(filepath.Separator)) && len(projPath) > bestLen {
			bestMatch = &projects[i]
			bestLen = len(projPath)
		}
	}
	if bestMatch != nil {
		return bestMatch
	}

	// Step 3: Git root match
	sessionGitRoot := FindGitRoot(sessionPath)
	if sessionGitRoot != "" {
		for i := range projects {
			if FindGitRoot(projects[i].Path) == sessionGitRoot {
				return &projects[i]
			}
		}
	}

	// No match → unassigned
	return nil
}
