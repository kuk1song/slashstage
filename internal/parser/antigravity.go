// Package parser — antigravity.go implements the Antigravity IDE/CLI parser.
// Antigravity stores conversations as encrypted protobuf in
// ~/.gemini/antigravity/conversations/<uuid>.pb
// with metadata in brain/<uuid>/*.metadata.json and
// timestamps in annotations/<uuid>.pbtxt.
//
// Since the .pb conversation files use a proprietary encrypted format,
// this parser extracts session-level metadata (titles, summaries,
// timestamps) from the sidecar files. Full message-level parsing
// requires reverse-engineering the protobuf schema (future work).
package parser

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kuk1song/slashstage/internal/model"
)

func init() {
	Register(model.AgentAntigravity, &AntigravityParser{})
}

// AntigravityParser reads session metadata from Antigravity's local data.
type AntigravityParser struct{}

// Discover finds all Antigravity conversation files.
func (p *AntigravityParser) Discover(config model.AgentConfig) ([]DiscoveredFile, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
	}

	convDir := filepath.Join(home, ".gemini", "antigravity", "conversations")
	if !DirExists(convDir) {
		return nil, nil
	}

	var files []DiscoveredFile
	entries, err := os.ReadDir(convDir)
	if err != nil {
		return nil, nil
	}

	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".pb") {
			files = append(files, DiscoveredFile{
				Path:      filepath.Join(convDir, e.Name()),
				AgentType: model.AgentAntigravity,
			})
		}
	}
	return files, nil
}

// Parse reads metadata for an Antigravity conversation.
// Since the .pb files are in a proprietary encrypted format, we extract
// session-level information from sidecar metadata files rather than
// parsing the conversation content directly.
func (p *AntigravityParser) Parse(path string) ([]model.ParseResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}

	// Session ID from filename (UUID)
	sessionUUID := strings.TrimSuffix(filepath.Base(path), ".pb")
	sessionID := "antigravity-" + sessionUUID

	// Use file modification time as fallback timestamp
	fileModTime := info.ModTime()

	// Compute a lightweight hash from file size + mod time (avoid reading large encrypted files)
	hash := fmt.Sprintf("ag-%s-%d-%d", sessionUUID, info.Size(), fileModTime.Unix())

	session := model.Session{
		ID:           sessionID,
		AgentType:    string(model.AgentAntigravity),
		SourcePath:   path,
		SourceHash:   hash,
		StartedAt:    fileModTime,
		LastActiveAt: fileModTime,
	}

	home, _ := os.UserHomeDir()
	agDir := filepath.Join(home, ".gemini", "antigravity")

	// 1. Extract timestamp from annotations/<uuid>.pbtxt
	pbtxtPath := filepath.Join(agDir, "annotations", sessionUUID+".pbtxt")
	if ts := parseAntigravityTimestamp(pbtxtPath); !ts.IsZero() {
		session.LastActiveAt = ts
	}

	// 2. Extract title from brain/<uuid>/ metadata files
	brainDir := filepath.Join(agDir, "brain", sessionUUID)
	title, artifactCount, summaryKeywords := extractBrainMetadata(brainDir)
	if title != "" {
		session.Title = title
	}

	// Use artifact count as a rough "message count" indicator
	if artifactCount > 0 {
		session.MessageCount = artifactCount
	}

	// 3. Resolve workspace from code_tracker + brain keyword matching
	if workspace := resolveAntigravityWorkspace(agDir, summaryKeywords); workspace != "" {
		session.Workspace = workspace
		slog.Debug("resolved antigravity workspace", "session", sessionUUID, "workspace", workspace)
	}

	// 4. If no title yet, generate one from the UUID and file size
	if session.Title == "" {
		sizeMB := float64(info.Size()) / (1024 * 1024)
		if sizeMB > 1 {
			session.Title = fmt.Sprintf("Antigravity Session (%.1f MB)", sizeMB)
		} else {
			sizeKB := float64(info.Size()) / 1024
			session.Title = fmt.Sprintf("Antigravity Session (%.0f KB)", sizeKB)
		}
	}

	// Create a summary message from the metadata we have
	var messages []model.Message
	summaries := collectBrainSummaries(brainDir)
	for i, summary := range summaries {
		messages = append(messages, model.Message{
			SessionID: sessionID,
			Role:      model.RoleAssistant,
			Content:   summary,
			SortOrder: i,
			CreatedAt: session.LastActiveAt,
		})
	}
	session.MessageCount = max(session.MessageCount, len(messages))

	return []model.ParseResult{{Session: session, Messages: messages}}, nil
}

// --- Helpers ---

// timestampRegex matches protobuf text format: seconds:1234567890
var timestampRegex = regexp.MustCompile(`seconds:(\d+)`)

// parseAntigravityTimestamp reads a .pbtxt file and extracts the timestamp.
// Format: last_user_view_time:{seconds:1771513671 nanos:88000000}
func parseAntigravityTimestamp(pbtxtPath string) time.Time {
	data, err := os.ReadFile(pbtxtPath)
	if err != nil {
		return time.Time{}
	}

	matches := timestampRegex.FindSubmatch(data)
	if len(matches) < 2 {
		return time.Time{}
	}

	seconds, err := strconv.ParseInt(string(matches[1]), 10, 64)
	if err != nil {
		return time.Time{}
	}

	return time.Unix(seconds, 0)
}

// brainMetadata represents a metadata JSON file from the brain directory.
type brainMetadata struct {
	ArtifactType string `json:"artifactType"`
	Summary      string `json:"summary"`
	UpdatedAt    string `json:"updatedAt"`
	Version      string `json:"version"`
}

// extractBrainMetadata reads all metadata files in the brain directory
// and extracts the title, artifact count, and keywords from summaries
// (used to match against known projects).
func extractBrainMetadata(brainDir string) (title string, artifactCount int, summaryKeywords []string) {
	if !DirExists(brainDir) {
		return "", 0, nil
	}

	entries, err := os.ReadDir(brainDir)
	if err != nil {
		return "", 0, nil
	}

	var metadataFiles []brainMetadata

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".metadata.json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(brainDir, e.Name()))
		if err != nil {
			continue
		}

		var meta brainMetadata
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}

		metadataFiles = append(metadataFiles, meta)
		artifactCount++
	}

	if len(metadataFiles) == 0 {
		return "", 0, nil
	}

	// Sort by update time descending
	sort.Slice(metadataFiles, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339Nano, metadataFiles[i].UpdatedAt)
		tj, _ := time.Parse(time.RFC3339Nano, metadataFiles[j].UpdatedAt)
		return ti.After(tj)
	})

	// Use the most recent summary as the title
	for _, m := range metadataFiles {
		if m.Summary != "" {
			title = m.Summary
			if len(title) > 120 {
				title = title[:120] + "..."
			}
			break
		}
	}

	// Collect all summary text as keywords for project matching
	var allText []string
	for _, m := range metadataFiles {
		if m.Summary != "" {
			allText = append(allText, m.Summary)
		}
	}
	// Also collect artifact file names (without .metadata.json suffix)
	for _, e := range entries {
		name := e.Name()
		name = strings.TrimSuffix(name, ".metadata.json")
		name = strings.TrimSuffix(name, ".resolved")
		if !strings.HasSuffix(name, ".metadata") {
			allText = append(allText, name)
		}
	}

	return title, artifactCount, allText
}

// antigravityProject maps a project name to its workspace path on disk.
type antigravityProject struct {
	name string
	path string
}

// resolveAntigravityWorkspace finds the workspace path for an Antigravity session
// by cross-referencing code_tracker/active/ project names with brain artifact keywords.
//
// Strategy:
// 1. Read code_tracker/active/ to find project names (e.g., "PrivateGpt_<hash>")
// 2. For each project name, find the actual workspace path on disk
// 3. If there's only one active project, use it (most common case)
// 4. If there are multiple, check if summaryKeywords mention a project name
func resolveAntigravityWorkspace(agDir string, summaryKeywords []string) string {
	activeDir := filepath.Join(agDir, "code_tracker", "active")
	if !DirExists(activeDir) {
		return ""
	}

	entries, err := os.ReadDir(activeDir)
	if err != nil {
		return ""
	}

	var projects []antigravityProject
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "no_repo" {
			continue
		}

		projectName := extractProjectNameFromTracker(e.Name())
		if projectName == "" {
			continue
		}

		// Try to find the actual workspace path on disk
		workspacePath := findWorkspaceOnDisk(projectName)
		if workspacePath == "" {
			slog.Debug("could not find workspace for antigravity project", "name", projectName)
			continue
		}

		projects = append(projects, antigravityProject{
			name: projectName,
			path: workspacePath,
		})
	}

	if len(projects) == 0 {
		return ""
	}

	// If only one active project, use it
	if len(projects) == 1 {
		return projects[0].path
	}

	// Multiple projects: match against summary keywords
	keywordText := strings.ToLower(strings.Join(summaryKeywords, " "))
	for _, proj := range projects {
		if strings.Contains(keywordText, strings.ToLower(proj.name)) {
			return proj.path
		}
	}

	// Fallback: return the first project (better than nothing)
	return projects[0].path
}

// extractProjectNameFromTracker extracts the project name from a code_tracker
// directory name. Format: "<ProjectName>_<40-char-hex-hash>"
func extractProjectNameFromTracker(dirName string) string {
	// The hash suffix is typically a 40-char hex string (git SHA)
	lastUnderscore := strings.LastIndex(dirName, "_")
	if lastUnderscore < 0 {
		return dirName
	}

	suffix := dirName[lastUnderscore+1:]
	// Verify it looks like a hex hash (at least 20 chars, all hex)
	if len(suffix) >= 20 && isHexString(suffix) {
		return dirName[:lastUnderscore]
	}

	return dirName
}

// isHexString checks if a string contains only hexadecimal characters.
func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return len(s) > 0
}

// findWorkspaceOnDisk searches common locations for a project directory.
func findWorkspaceOnDisk(projectName string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Common project parent directories to search
	searchDirs := []string{
		home,
		filepath.Join(home, "Documents"),
		filepath.Join(home, "Projects"),
		filepath.Join(home, "projects"),
		filepath.Join(home, "Developer"),
		filepath.Join(home, "dev"),
		filepath.Join(home, "src"),
		filepath.Join(home, "workspace"),
		filepath.Join(home, "work"),
		filepath.Join(home, "code"),
		filepath.Join(home, "Code"),
		filepath.Join(home, "repos"),
		filepath.Join(home, "Desktop"),
	}

	// Also search any immediate subdirectories of home (e.g., ~/ot/, ~/my-projects/)
	homeEntries, _ := os.ReadDir(home)
	for _, e := range homeEntries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			searchDirs = append(searchDirs, filepath.Join(home, e.Name()))
		}
	}

	// Search for exact match (case-insensitive on macOS, but we do explicit check)
	for _, dir := range searchDirs {
		candidate := filepath.Join(dir, projectName)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}

	// Search with case-insensitive matching
	lowerName := strings.ToLower(projectName)
	for _, dir := range searchDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() && strings.ToLower(e.Name()) == lowerName {
				return filepath.Join(dir, e.Name())
			}
		}
	}

	return ""
}

// collectBrainSummaries gathers all unique summaries from the brain metadata.
func collectBrainSummaries(brainDir string) []string {
	if !DirExists(brainDir) {
		return nil
	}

	entries, err := os.ReadDir(brainDir)
	if err != nil {
		return nil
	}

	type metaEntry struct {
		summary   string
		updatedAt time.Time
	}
	var metas []metaEntry

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".metadata.json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(brainDir, e.Name()))
		if err != nil {
			continue
		}
		var meta brainMetadata
		if err := json.Unmarshal(data, &meta); err != nil || meta.Summary == "" {
			continue
		}
		t, _ := time.Parse(time.RFC3339Nano, meta.UpdatedAt)
		metas = append(metas, metaEntry{summary: meta.Summary, updatedAt: t})
	}

	// Sort by time descending (newest first)
	sort.Slice(metas, func(i, j int) bool {
		return metas[i].updatedAt.After(metas[j].updatedAt)
	})

	// Return up to 20 summaries to avoid overwhelming the message list
	var summaries []string
	seen := make(map[string]bool)
	for _, m := range metas {
		if seen[m.summary] {
			continue
		}
		seen[m.summary] = true
		summaries = append(summaries, fmt.Sprintf("[%s] %s", m.updatedAt.Format("2006-01-02 15:04"), m.summary))
		if len(summaries) >= 20 {
			break
		}
	}
	return summaries
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
