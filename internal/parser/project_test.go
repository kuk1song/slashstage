package parser

import (
	"testing"

	"github.com/kuk1song/slashstage/internal/model"
)

func TestExtractRepoName(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "SSH format",
			url:  "git@github.com:kuk1song/slashstage.git",
			want: "slashstage",
		},
		{
			name: "SSH format without .git",
			url:  "git@github.com:kuk1song/slashstage",
			want: "slashstage",
		},
		{
			name: "HTTPS format",
			url:  "https://github.com/kuk1song/slashstage.git",
			want: "slashstage",
		},
		{
			name: "HTTPS format without .git",
			url:  "https://github.com/kuk1song/slashstage",
			want: "slashstage",
		},
		{
			name: "SSH with custom host",
			url:  "git@github-personal:kuk1song/slashstage.git",
			want: "slashstage",
		},
		{
			name: "empty",
			url:  "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractRepoName(tt.url)
			if got != tt.want {
				t.Errorf("extractRepoName(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestDecodeEncodedPath(t *testing.T) {
	tests := []struct {
		name    string
		encoded string
		want    string
	}{
		{
			name:    "Claude Code style path",
			encoded: "-Users-kuki-openclaw",
			want:    "/Users/kuki/openclaw",
		},
		{
			name:    "empty",
			encoded: "",
			want:    "",
		},
		{
			name:    "single segment",
			encoded: "-tmp",
			want:    "/tmp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecodeEncodedPath(tt.encoded)
			if got != tt.want {
				t.Errorf("DecodeEncodedPath(%q) = %q, want %q", tt.encoded, got, tt.want)
			}
		})
	}
}

func TestExtractProjectFromCwd(t *testing.T) {
	tests := []struct {
		name string
		cwd  string
		want string
	}{
		{"normal path", "/Users/kuki/project", "/Users/kuki/project"},
		{"trailing slash", "/Users/kuki/project/", "/Users/kuki/project"},
		{"double slash", "/Users/kuki//project", "/Users/kuki/project"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractProjectFromCwd(tt.cwd)
			if got != tt.want {
				t.Errorf("ExtractProjectFromCwd(%q) = %q, want %q", tt.cwd, got, tt.want)
			}
		})
	}
}

func TestGetProjectNameFallback(t *testing.T) {
	// When there's no git, should fall back to directory name
	got := GetProjectName("/tmp/some-project-dir")
	if got != "some-project-dir" {
		t.Errorf("GetProjectName(/tmp/some-project-dir) = %q, want %q", got, "some-project-dir")
	}
}

func TestGetProjectNameEmpty(t *testing.T) {
	got := GetProjectName("")
	if got != "" {
		t.Errorf("GetProjectName('') = %q, want empty", got)
	}
}

func TestBindSessionToProject_ExactMatch(t *testing.T) {
	projects := []model.Project{
		{ID: 1, Name: "alpha", Path: "/Users/kuki/alpha"},
		{ID: 2, Name: "beta", Path: "/Users/kuki/beta"},
	}

	got := BindSessionToProject("/Users/kuki/alpha", projects)
	if got == nil {
		t.Fatal("expected match, got nil")
	}
	if got.ID != 1 {
		t.Errorf("matched project ID = %d, want 1", got.ID)
	}
}

func TestBindSessionToProject_PrefixMatch(t *testing.T) {
	projects := []model.Project{
		{ID: 1, Name: "alpha", Path: "/Users/kuki/alpha"},
		{ID: 2, Name: "beta", Path: "/Users/kuki/beta"},
	}

	got := BindSessionToProject("/Users/kuki/alpha/backend", projects)
	if got == nil {
		t.Fatal("expected prefix match, got nil")
	}
	if got.ID != 1 {
		t.Errorf("matched project ID = %d, want 1", got.ID)
	}
}

func TestBindSessionToProject_LongestPrefixWins(t *testing.T) {
	projects := []model.Project{
		{ID: 1, Name: "parent", Path: "/Users/kuki"},
		{ID: 2, Name: "child", Path: "/Users/kuki/alpha"},
	}

	got := BindSessionToProject("/Users/kuki/alpha/src", projects)
	if got == nil {
		t.Fatal("expected prefix match, got nil")
	}
	if got.ID != 2 {
		t.Errorf("should match longest prefix: got ID=%d, want 2", got.ID)
	}
}

func TestBindSessionToProject_NoMatch(t *testing.T) {
	projects := []model.Project{
		{ID: 1, Name: "alpha", Path: "/Users/kuki/alpha"},
	}

	got := BindSessionToProject("/Users/kuki/completely-different", projects)
	if got != nil {
		t.Errorf("expected nil for no match, got project %d", got.ID)
	}
}

func TestBindSessionToProject_EmptyWorkspace(t *testing.T) {
	projects := []model.Project{
		{ID: 1, Name: "alpha", Path: "/Users/kuki/alpha"},
	}

	got := BindSessionToProject("", projects)
	if got != nil {
		t.Error("expected nil for empty workspace")
	}
}

func TestRegistryHasExpectedParsers(t *testing.T) {
	expected := map[model.AgentType]bool{
		model.AgentCursorIDE:   true,
		model.AgentCursorCLI:   true,
		model.AgentClaude:      true,
		model.AgentCodex:       true,
		model.AgentGemini:      true,
		model.AgentOpenCode:    true,
		model.AgentAntigravity: true,
		model.AgentCopilot:     true,
	}

	for _, cfg := range Registry {
		if !expected[cfg.Type] {
			t.Errorf("unexpected agent in Registry: %s", cfg.Type)
		}
		delete(expected, cfg.Type)
	}

	for agentType := range expected {
		t.Errorf("missing agent in Registry: %s", agentType)
	}
}

func TestGetConfig(t *testing.T) {
	cfg := GetConfig(model.AgentClaude)
	if cfg == nil {
		t.Fatal("GetConfig(AgentClaude) = nil")
	}
	if cfg.DisplayName != "Claude Code" {
		t.Errorf("DisplayName = %q, want %q", cfg.DisplayName, "Claude Code")
	}
	if cfg.Format != "jsonl" {
		t.Errorf("Format = %q, want %q", cfg.Format, "jsonl")
	}
}

func TestGetConfigNotFound(t *testing.T) {
	cfg := GetConfig("nonexistent")
	if cfg != nil {
		t.Error("expected nil for nonexistent agent type")
	}
}
