package export

import (
	"strings"
	"testing"
	"time"

	"github.com/kuk1song/slashstage/internal/model"
)

func TestToMarkdown_Empty(t *testing.T) {
	session := model.Session{
		ID:        "test-session-1",
		Title:     "Empty Session",
		AgentType: "test-agent",
		StartedAt: time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC),
	}
	markdown := ToMarkdown(session, []model.Message{})
	if !strings.Contains(markdown, "# Session: Empty Session") {
		t.Errorf("Expected title, got: %s", markdown)
	}
	if !strings.Contains(markdown, "**Message Count**: 0") {
		t.Errorf("Expected message count 0, got: %s", markdown)
	}
	// Should NOT have "Conversation History" section for empty session
	if strings.Contains(markdown, "Conversation History") {
		t.Errorf("Did not expect conversation history for empty session")
	}
}

func TestToMarkdown_WithMessages(t *testing.T) {
	session := model.Session{
		ID:           "test-session-2",
		Title:        "Test Session with Messages",
		AgentType:    "test-agent",
		Workspace:    "/path/to/workspace",
		Model:        "gpt-4",
		StartedAt:    time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC),
		LastActiveAt: time.Date(2023, 1, 1, 10, 30, 0, 0, time.UTC),
		TokensIn:     100,
		TokensOut:    50,
	}
	messages := []model.Message{
		{
			Role:      model.RoleUser,
			Content:   "Hello, world!",
			CreatedAt: time.Date(2023, 1, 1, 10, 1, 0, 0, time.UTC),
			SortOrder: 0,
		},
		{
			Role:      model.RoleAssistant,
			Content:   "Hi there! How can I help?",
			CreatedAt: time.Date(2023, 1, 1, 10, 2, 0, 0, time.UTC),
			SortOrder: 1,
		},
	}

	markdown := ToMarkdown(session, messages)

	checks := []string{
		"# Session: Test Session with Messages",
		"**Workspace**: `/path/to/workspace`",
		"**Model**: `gpt-4`",
		"**Message Count**: 2",
		"**Tokens In**: 100",
		"**Tokens Out**: 50",
		"### User (2023-01-01T10:01:00Z)",
		"Hello, world!",
		"### Assistant (2023-01-01T10:02:00Z)",
		"Hi there! How can I help?",
	}
	for _, check := range checks {
		if !strings.Contains(markdown, check) {
			t.Errorf("Expected %q in output", check)
		}
	}
}

func TestToMarkdown_ToolMessages(t *testing.T) {
	session := model.Session{
		ID:        "test-session-3",
		Title:     "Tool Session",
		AgentType: "test-agent",
		StartedAt: time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC),
	}
	messages := []model.Message{
		{
			Role:      model.RoleAssistant,
			Content:   "Calling tool...",
			ToolName:  "search_web",
			ToolInput: `{"query": "latest Go features"}`,
			CreatedAt: time.Date(2023, 1, 1, 10, 1, 0, 0, time.UTC),
			SortOrder: 0,
		},
		{
			Role:      model.RoleTool,
			Content:   "Go 1.22 released with new features.",
			CreatedAt: time.Date(2023, 1, 1, 10, 2, 0, 0, time.UTC),
			SortOrder: 1,
		},
	}

	markdown := ToMarkdown(session, messages)

	if !strings.Contains(markdown, "#### Tool Call: `search_web`") {
		t.Errorf("Expected tool call header")
	}
	if !strings.Contains(markdown, `"query": "latest Go features"`) {
		t.Errorf("Expected tool input")
	}
	if !strings.Contains(markdown, "Go 1.22 released with new features.") {
		t.Errorf("Expected tool result")
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello world", 100, "hello world"},
		{"hello world", 8, "hello..."},
		{"a", 1, "a"},
		{"", 10, ""},
		{"long string that needs truncation", 10, "long st..."},
	}

	for _, tt := range tests {
		result := Truncate(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("Truncate(%q, %d): got %q, want %q", tt.input, tt.maxLen, result, tt.expected)
		}
	}
}
