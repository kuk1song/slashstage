// Package server provides the HTTP API for SlashStage.
package server

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/kuk1song/slashstage/internal/db"
	"github.com/kuk1song/slashstage/internal/mcp"
	"github.com/kuk1song/slashstage/internal/model"
	"github.com/kuk1song/slashstage/internal/parser"
)

//go:embed frontend.html
var frontendHTML string

// Server is the SlashStage HTTP API server.
type Server struct {
	db     *db.DB
	mux    *http.ServeMux
	addr   string
}

// New creates a new HTTP server.
func New(database *db.DB, addr string) *Server {
	s := &Server{
		db:   database,
		mux:  http.NewServeMux(),
		addr: addr,
	}
	s.routes()
	return s
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	slog.Info("starting server", "addr", s.addr)
	return http.ListenAndServe(s.addr, s.mux)
}

func (s *Server) routes() {
	// API routes
	s.mux.HandleFunc("GET /api/projects", s.handleListProjects)
	s.mux.HandleFunc("POST /api/projects", s.handleCreateProject)
	s.mux.HandleFunc("GET /api/projects/{id}", s.handleGetProject)
	s.mux.HandleFunc("DELETE /api/projects/{id}", s.handleDeleteProject)
	s.mux.HandleFunc("GET /api/projects/{id}/sessions", s.handleProjectSessions)
	s.mux.HandleFunc("GET /api/projects/{id}/config", s.handleProjectConfig)
	s.mux.HandleFunc("GET /api/sessions/{id}", s.handleGetSession)
	s.mux.HandleFunc("GET /api/sessions/{id}/messages", s.handleSessionMessages)
	s.mux.HandleFunc("GET /api/sessions/unassigned", s.handleUnassignedSessions)
	s.mux.HandleFunc("POST /api/scan", s.handleScan)
	s.mux.HandleFunc("GET /api/search", s.handleSearch)
	s.mux.HandleFunc("GET /api/mcp", s.handleMCPConfigs)
	s.mux.HandleFunc("GET /api/stats", s.handleStats)

	// Health check
	s.mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": "0.1.0"})
	})

	// Frontend static files (will be embedded later)
	s.mux.HandleFunc("GET /", s.handleFrontend)
}

// --- Handlers ---

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.db.ListProjects()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Enrich with session counts per agent type
	type ProjectWithStats struct {
		model.Project
		SessionsByAgent map[string]int `json:"sessions_by_agent"`
		TotalSessions   int            `json:"total_sessions"`
	}

	var enriched []ProjectWithStats
	for _, p := range projects {
		sessions, err := s.db.ListSessionsByProject(p.ID)
		if err != nil {
			continue
		}
		byAgent := make(map[string]int)
		for _, sess := range sessions {
			byAgent[sess.AgentType]++
		}
		enriched = append(enriched, ProjectWithStats{
			Project:         p,
			SessionsByAgent: byAgent,
			TotalSessions:   len(sessions),
		})
	}

	writeJSON(w, http.StatusOK, enriched)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" || req.Path == "" {
		writeError(w, http.StatusBadRequest, "name and path are required")
		return
	}

	// Auto-detect git info
	gitRemote := ""
	gitBranch := ""
	if root := parser.FindGitRoot(req.Path); root != "" {
		// Could extract remote and branch here
	}

	project, err := s.db.CreateProject(req.Name, req.Path, gitRemote, gitBranch)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "project with this path already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, project)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project ID")
		return
	}
	project, err := s.db.GetProject(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project ID")
		return
	}
	if err := s.db.DeleteProject(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleProjectSessions(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project ID")
		return
	}
	sessions, err := s.db.ListSessionsByProject(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (s *Server) handleProjectConfig(w http.ResponseWriter, r *http.Request) {
	configs, err := mcp.ReadAllMCPConfigs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, configs)
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	session, err := s.db.GetSession(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) handleSessionMessages(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	messages, err := s.db.GetMessages(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, messages)
}

func (s *Server) handleUnassignedSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.db.ListUnassignedSessions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	slog.Info("starting full scan")

	// Parse all sessions
	results, err := parser.ParseAll()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("scan failed: %v", err))
		return
	}

	// Get existing projects for binding
	projects, err := s.db.ListProjects()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Auto-discover new projects if none exist
	if len(projects) == 0 {
		candidates := parser.DiscoverProjects(results)
		for _, c := range candidates {
			p, err := s.db.CreateProject(c.Name, c.Path, "", "")
			if err == nil {
				projects = append(projects, *p)
			}
		}
	}

	// Store sessions and bind to projects
	sessionCount := 0
	for _, result := range results {
		// Bind to project
		if proj := parser.BindSessionToProject(result.Session.Workspace, projects); proj != nil {
			result.Session.ProjectID = &proj.ID
		}

		// Upsert session
		if err := s.db.UpsertSession(&result.Session); err != nil {
			slog.Warn("upsert session failed", "id", result.Session.ID, "error", err)
			continue
		}

		// Insert messages
		if len(result.Messages) > 0 {
			if err := s.db.InsertMessages(result.Session.ID, result.Messages); err != nil {
				slog.Warn("insert messages failed", "session", result.Session.ID, "error", err)
			}
		}

		sessionCount++
	}

	slog.Info("scan complete", "sessions", sessionCount, "projects", len(projects))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sessions_found":    sessionCount,
		"projects_found":    len(projects),
		"new_projects":      len(projects),
	})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	messages, err := s.db.SearchMessages(q, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, messages)
}

func (s *Server) handleMCPConfigs(w http.ResponseWriter, r *http.Request) {
	configs, err := mcp.ReadAllMCPConfigs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, configs)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	projects, _ := s.db.ListProjects()
	unassigned, _ := s.db.ListUnassignedSessions()

	totalSessions := 0
	for _, p := range projects {
		sessions, _ := s.db.ListSessionsByProject(p.ID)
		totalSessions += len(sessions)
	}
	totalSessions += len(unassigned)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_projects":      len(projects),
		"total_sessions":      totalSessions,
		"unassigned_sessions": len(unassigned),
	})
}

func (s *Server) handleFrontend(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, frontendHTML)
}
