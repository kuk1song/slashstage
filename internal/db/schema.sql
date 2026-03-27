-- SlashStage SQLite Schema
-- Every project is a stage 🎸

PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA synchronous = NORMAL;
PRAGMA foreign_keys = ON;
PRAGMA mmap_size = 268435456; -- 256MB mmap for read performance

-- Projects: the core entity
CREATE TABLE IF NOT EXISTS projects (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL,
    path        TEXT    NOT NULL UNIQUE,
    git_remote  TEXT    DEFAULT '',
    git_branch  TEXT    DEFAULT '',
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_projects_path ON projects(path);

-- Project links: external URLs associated with a project
CREATE TABLE IF NOT EXISTS project_links (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    kind       TEXT    NOT NULL, -- 'git', 'doc', 'url', 'api'
    label      TEXT    NOT NULL,
    url        TEXT    NOT NULL,
    created_at TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- Sessions: coding sessions from all IDE/CLI tools
CREATE TABLE IF NOT EXISTS sessions (
    id             TEXT    PRIMARY KEY, -- UUID or IDE-specific ID
    project_id     INTEGER REFERENCES projects(id) ON DELETE SET NULL,
    agent_type     TEXT    NOT NULL,    -- 'cursor-ide', 'claude', 'codex', etc.
    title          TEXT    DEFAULT '',
    workspace      TEXT    DEFAULT '',  -- Workspace path from session
    model          TEXT    DEFAULT '',
    message_count  INTEGER DEFAULT 0,
    tokens_in      INTEGER DEFAULT 0,
    tokens_out     INTEGER DEFAULT 0,
    started_at     TEXT    NOT NULL,
    last_active_at TEXT    NOT NULL,
    source_path    TEXT    NOT NULL,    -- File path on disk
    source_hash    TEXT    NOT NULL,    -- SHA-256 for change detection
    created_at     TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at     TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_sessions_project   ON sessions(project_id);
CREATE INDEX IF NOT EXISTS idx_sessions_agent     ON sessions(agent_type);
CREATE INDEX IF NOT EXISTS idx_sessions_workspace ON sessions(workspace);
CREATE INDEX IF NOT EXISTS idx_sessions_source    ON sessions(source_path);

-- Messages: individual messages within sessions
CREATE TABLE IF NOT EXISTS messages (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT    NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    role       TEXT    NOT NULL, -- 'user', 'assistant', 'tool', 'system'
    content    TEXT    NOT NULL DEFAULT '',
    tool_name  TEXT    DEFAULT '',
    tool_input TEXT    DEFAULT '',
    tokens_in  INTEGER DEFAULT 0,
    tokens_out INTEGER DEFAULT 0,
    created_at TEXT    NOT NULL DEFAULT (datetime('now')),
    sort_order INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, sort_order);

-- Full-text search on messages (FTS5)
CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
    content,
    tool_name,
    content='messages',
    content_rowid='id',
    tokenize='unicode61'
);

-- Triggers to keep FTS in sync
CREATE TRIGGER IF NOT EXISTS messages_ai AFTER INSERT ON messages BEGIN
    INSERT INTO messages_fts(rowid, content, tool_name)
    VALUES (new.id, new.content, new.tool_name);
END;

CREATE TRIGGER IF NOT EXISTS messages_ad AFTER DELETE ON messages BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, content, tool_name)
    VALUES ('delete', old.id, old.content, old.tool_name);
END;

CREATE TRIGGER IF NOT EXISTS messages_au AFTER UPDATE ON messages BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, content, tool_name)
    VALUES ('delete', old.id, old.content, old.tool_name);
    INSERT INTO messages_fts(rowid, content, tool_name)
    VALUES (new.id, new.content, new.tool_name);
END;

-- Artifacts: IDE-specific files (images, docs) outside the project directory
CREATE TABLE IF NOT EXISTS artifacts (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER REFERENCES projects(id) ON DELETE SET NULL,
    session_id TEXT    REFERENCES sessions(id) ON DELETE SET NULL,
    agent_type TEXT    NOT NULL,
    kind       TEXT    NOT NULL, -- 'image', 'document', 'code_snapshot'
    name       TEXT    NOT NULL,
    path       TEXT    NOT NULL, -- Absolute path on disk
    size_bytes INTEGER DEFAULT 0,
    created_at TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- Daily stats: aggregated per-project per-agent daily metrics
CREATE TABLE IF NOT EXISTS daily_stats (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id    INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    agent_type    TEXT    NOT NULL,
    date          TEXT    NOT NULL, -- 'YYYY-MM-DD'
    session_count INTEGER DEFAULT 0,
    message_count INTEGER DEFAULT 0,
    tokens_in     INTEGER DEFAULT 0,
    tokens_out    INTEGER DEFAULT 0,
    UNIQUE(project_id, agent_type, date)
);

-- Sync state: track file hashes to enable incremental sync
CREATE TABLE IF NOT EXISTS sync_state (
    source_path TEXT    PRIMARY KEY,
    hash        TEXT    NOT NULL,
    agent_type  TEXT    NOT NULL,
    last_synced TEXT    NOT NULL DEFAULT (datetime('now')),
    skip        INTEGER DEFAULT 0  -- 1 = skip this file (parse failure)
);
