package parser

import (
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
// Claude Code / Cursor CLI encode paths: "-Users-kuki-openclaw" → "/Users/kuki/openclaw"
func DecodeEncodedPath(encoded string) string {
	if encoded == "" {
		return ""
	}
	// Replace leading dash with /
	decoded := strings.ReplaceAll(encoded, "-", "/")
	// Handle double dashes (escaped separators)
	// Some IDEs use -- for a literal dash
	decoded = strings.ReplaceAll(decoded, "//", "-")
	return filepath.Clean(decoded)
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
