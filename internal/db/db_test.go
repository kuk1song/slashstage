package db_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kuk1song/slashstage/internal/db"
	"github.com/kuk1song/slashstage/internal/model"
)

// testDB creates a temporary SQLite database for testing.
func testDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// --- Projects ---

func TestCreateProject(t *testing.T) {
	d := testDB(t)

	p, err := d.CreateProject("myproject", "/home/user/myproject", "git@github.com:user/myproject.git", "main")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if p.Name != "myproject" {
		t.Errorf("Name = %q, want %q", p.Name, "myproject")
	}
	if p.Path != "/home/user/myproject" {
		t.Errorf("Path = %q, want %q", p.Path, "/home/user/myproject")
	}
}

func TestCreateProjectDuplicatePath(t *testing.T) {
	d := testDB(t)

	_, err := d.CreateProject("a", "/same/path", "", "")
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err = d.CreateProject("b", "/same/path", "", "")
	if err == nil {
		t.Error("expected error on duplicate path, got nil")
	}
}

func TestGetProject(t *testing.T) {
	d := testDB(t)

	created, _ := d.CreateProject("test", "/tmp/test", "", "")
	got, err := d.GetProject(created.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got.Name != "test" {
		t.Errorf("Name = %q, want %q", got.Name, "test")
	}
}

func TestGetProjectNotFound(t *testing.T) {
	d := testDB(t)

	_, err := d.GetProject(999)
	if err == nil {
		t.Error("expected error for nonexistent project")
	}
}

func TestGetProjectByPath(t *testing.T) {
	d := testDB(t)

	_, _ = d.CreateProject("test", "/tmp/test", "", "")
	got, err := d.GetProjectByPath("/tmp/test")
	if err != nil {
		t.Fatalf("GetProjectByPath: %v", err)
	}
	if got.Name != "test" {
		t.Errorf("Name = %q, want %q", got.Name, "test")
	}
}

func TestListProjects(t *testing.T) {
	d := testDB(t)

	_, _ = d.CreateProject("alpha", "/alpha", "", "")
	_, _ = d.CreateProject("beta", "/beta", "", "")
	_, _ = d.CreateProject("gamma", "/gamma", "", "")

	list, err := d.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("got %d projects, want 3", len(list))
	}
}

func TestDeleteProject(t *testing.T) {
	d := testDB(t)

	p, _ := d.CreateProject("test", "/tmp/delete-me", "", "")
	if err := d.DeleteProject(p.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	_, err := d.GetProject(p.ID)
	if err == nil {
		t.Error("expected error after delete")
	}
}

// --- Sessions ---

func TestUpsertAndGetSession(t *testing.T) {
	d := testDB(t)

	now := time.Now().UTC()
	s := &model.Session{
		ID:           "test-session-001",
		AgentType:    "claude",
		Title:        "Test Session",
		Workspace:    "/tmp/test",
		Model:        "claude-3.5-sonnet",
		MessageCount: 10,
		StartedAt:    now,
		LastActiveAt: now,
		SourcePath:   "/home/.claude/test.jsonl",
		SourceHash:   "abc123",
	}

	if err := d.UpsertSession(s); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	got, err := d.GetSession("test-session-001")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Title != "Test Session" {
		t.Errorf("Title = %q, want %q", got.Title, "Test Session")
	}
	if got.AgentType != "claude" {
		t.Errorf("AgentType = %q, want %q", got.AgentType, "claude")
	}
	if got.MessageCount != 10 {
		t.Errorf("MessageCount = %d, want 10", got.MessageCount)
	}
}

func TestUpsertSessionUpdate(t *testing.T) {
	d := testDB(t)

	now := time.Now().UTC()
	s := &model.Session{
		ID:           "update-test",
		AgentType:    "codex",
		Title:        "Original",
		Workspace:    "/tmp/test",
		MessageCount: 5,
		StartedAt:    now,
		LastActiveAt: now,
		SourcePath:   "/path/to/source",
		SourceHash:   "hash1",
	}
	_ = d.UpsertSession(s)

	// Update with same ID
	s.Title = "Updated"
	s.MessageCount = 20
	s.SourceHash = "hash2"
	if err := d.UpsertSession(s); err != nil {
		t.Fatalf("UpsertSession (update): %v", err)
	}

	got, _ := d.GetSession("update-test")
	if got.Title != "Updated" {
		t.Errorf("Title = %q, want %q", got.Title, "Updated")
	}
	if got.MessageCount != 20 {
		t.Errorf("MessageCount = %d, want 20", got.MessageCount)
	}
}

func TestListSessionsByProject(t *testing.T) {
	d := testDB(t)

	p, _ := d.CreateProject("proj", "/proj", "", "")
	now := time.Now().UTC()

	for i, title := range []string{"S1", "S2", "S3"} {
		s := &model.Session{
			ID:           title,
			ProjectID:    &p.ID,
			AgentType:    "claude",
			Title:        title,
			MessageCount: i + 1,
			StartedAt:    now,
			LastActiveAt: now,
			SourcePath:   "/path/" + title,
			SourceHash:   "h" + title,
		}
		_ = d.UpsertSession(s)
	}

	sessions, err := d.ListSessionsByProject(p.ID)
	if err != nil {
		t.Fatalf("ListSessionsByProject: %v", err)
	}
	if len(sessions) != 3 {
		t.Errorf("got %d sessions, want 3", len(sessions))
	}
}

func TestListUnassignedSessions(t *testing.T) {
	d := testDB(t)

	now := time.Now().UTC()
	s := &model.Session{
		ID:           "unassigned-001",
		AgentType:    "gemini",
		Title:        "No Project",
		StartedAt:    now,
		LastActiveAt: now,
		SourcePath:   "/path",
		SourceHash:   "h",
	}
	_ = d.UpsertSession(s)

	sessions, err := d.ListUnassignedSessions()
	if err != nil {
		t.Fatalf("ListUnassignedSessions: %v", err)
	}
	if len(sessions) < 1 {
		t.Error("expected at least 1 unassigned session")
	}
}

// --- Messages ---

func TestInsertAndGetMessages(t *testing.T) {
	d := testDB(t)

	// Create session first
	now := time.Now().UTC()
	s := &model.Session{
		ID: "msg-session", AgentType: "claude",
		StartedAt: now, LastActiveAt: now,
		SourcePath: "/p", SourceHash: "h",
	}
	_ = d.UpsertSession(s)

	msgs := []model.Message{
		{SessionID: "msg-session", Role: model.RoleUser, Content: "Hello", SortOrder: 0},
		{SessionID: "msg-session", Role: model.RoleAssistant, Content: "Hi there!", SortOrder: 1},
		{SessionID: "msg-session", Role: model.RoleTool, Content: "file created", ToolName: "write_file", SortOrder: 2},
	}

	if err := d.InsertMessages("msg-session", msgs); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	got, err := d.GetMessages("msg-session")
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d messages, want 3", len(got))
	}
	if got[0].Role != model.RoleUser {
		t.Errorf("msg[0].Role = %q, want %q", got[0].Role, model.RoleUser)
	}
	if got[1].Content != "Hi there!" {
		t.Errorf("msg[1].Content = %q, want %q", got[1].Content, "Hi there!")
	}
	if got[2].ToolName != "write_file" {
		t.Errorf("msg[2].ToolName = %q, want %q", got[2].ToolName, "write_file")
	}
}

func TestInsertMessagesReplacesOld(t *testing.T) {
	d := testDB(t)

	now := time.Now().UTC()
	s := &model.Session{
		ID: "replace-session", AgentType: "claude",
		StartedAt: now, LastActiveAt: now,
		SourcePath: "/p", SourceHash: "h",
	}
	_ = d.UpsertSession(s)

	// Insert initial messages
	_ = d.InsertMessages("replace-session", []model.Message{
		{Role: model.RoleUser, Content: "First", SortOrder: 0},
	})

	// Replace with new messages
	_ = d.InsertMessages("replace-session", []model.Message{
		{Role: model.RoleUser, Content: "New first", SortOrder: 0},
		{Role: model.RoleAssistant, Content: "New second", SortOrder: 1},
	})

	got, _ := d.GetMessages("replace-session")
	if len(got) != 2 {
		t.Fatalf("got %d messages after replace, want 2", len(got))
	}
	if got[0].Content != "New first" {
		t.Errorf("msg[0].Content = %q, want %q", got[0].Content, "New first")
	}
}

func TestSearchMessages(t *testing.T) {
	d := testDB(t)

	now := time.Now().UTC()
	s := &model.Session{
		ID: "search-session", AgentType: "claude",
		StartedAt: now, LastActiveAt: now,
		SourcePath: "/p", SourceHash: "h",
	}
	_ = d.UpsertSession(s)

	_ = d.InsertMessages("search-session", []model.Message{
		{Role: model.RoleUser, Content: "How to implement authentication in Go?", SortOrder: 0},
		{Role: model.RoleAssistant, Content: "Here is how to implement JWT authentication...", SortOrder: 1},
		{Role: model.RoleUser, Content: "What about database migrations?", SortOrder: 2},
	})

	results, err := d.SearchMessages("authentication", 10)
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(results) < 1 {
		t.Error("expected at least 1 search result for 'authentication'")
	}

	// Search for something not present
	results, err = d.SearchMessages("kubernetes", 10)
	if err != nil {
		t.Fatalf("SearchMessages (no results): %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for 'kubernetes', got %d", len(results))
	}
}

// --- Sync State ---

func TestSyncState(t *testing.T) {
	d := testDB(t)

	if err := d.SetSyncState("/path/to/file", "hash123", "claude"); err != nil {
		t.Fatalf("SetSyncState: %v", err)
	}

	state, err := d.GetSyncState("/path/to/file")
	if err != nil {
		t.Fatalf("GetSyncState: %v", err)
	}
	if state.Hash != "hash123" {
		t.Errorf("Hash = %q, want %q", state.Hash, "hash123")
	}
	if state.AgentType != "claude" {
		t.Errorf("AgentType = %q, want %q", state.AgentType, "claude")
	}
}

func TestMarkSkip(t *testing.T) {
	d := testDB(t)

	_ = d.SetSyncState("/skip-me", "hash", "claude")
	if err := d.MarkSkip("/skip-me"); err != nil {
		t.Fatalf("MarkSkip: %v", err)
	}

	state, _ := d.GetSyncState("/skip-me")
	if !state.Skip {
		t.Error("expected Skip=true after MarkSkip")
	}
}

// --- Database ---

func TestDefaultDBPath(t *testing.T) {
	path, err := db.DefaultDBPath()
	if err != nil {
		t.Fatalf("DefaultDBPath: %v", err)
	}
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".slashstage", "data.db")
	if path != expected {
		t.Errorf("DefaultDBPath = %q, want %q", path, expected)
	}
}
