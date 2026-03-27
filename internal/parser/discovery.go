// Package parser — discovery.go implements the project auto-discovery engine.
// Scans all IDE data directories → discovers sessions → groups by project.
package parser

import (
	"fmt"
	"log/slog"

	"github.com/kuk1song/slashstage/internal/model"
)

// DiscoverAll scans all registered IDEs and returns discovered session files.
func DiscoverAll() ([]DiscoveredFile, error) {
	var allFiles []DiscoveredFile

	for _, config := range Registry {
		p := GetParser(config.Type)
		if p == nil {
			slog.Debug("no parser registered", "agent", config.Type)
			continue
		}

		files, err := p.Discover(config)
		if err != nil {
			slog.Warn("discover failed", "agent", config.Type, "error", err)
			continue
		}

		slog.Info("discovered files", "agent", config.DisplayName, "count", len(files))
		allFiles = append(allFiles, files...)
	}

	return allFiles, nil
}

// ParseAll discovers and parses all sessions from all IDEs.
func ParseAll() ([]model.ParseResult, error) {
	files, err := DiscoverAll()
	if err != nil {
		return nil, err
	}

	var allResults []model.ParseResult

	for _, file := range files {
		p := GetParser(file.AgentType)
		if p == nil {
			continue
		}

		results, err := p.Parse(file.Path)
		if err != nil {
			slog.Warn("parse failed", "path", file.Path, "agent", file.AgentType, "error", err)
			continue
		}

		allResults = append(allResults, results...)
	}

	return allResults, nil
}

// DiscoverProjects extracts unique project paths from parsed sessions.
// Groups sessions by workspace path and returns project candidates.
func DiscoverProjects(results []model.ParseResult) []ProjectCandidate {
	// Group by workspace path
	groups := make(map[string]*ProjectCandidate)

	for _, r := range results {
		ws := r.Session.Workspace
		if ws == "" {
			continue
		}

		if existing, ok := groups[ws]; ok {
			existing.SessionCount++
			existing.AgentTypes[model.AgentType(r.Session.AgentType)] = true
		} else {
			groups[ws] = &ProjectCandidate{
				Path:         ws,
				Name:         GetProjectName(ws),
				SessionCount: 1,
				AgentTypes:   map[model.AgentType]bool{model.AgentType(r.Session.AgentType): true},
			}
		}
	}

	// Convert to slice
	var candidates []ProjectCandidate
	for _, c := range groups {
		candidates = append(candidates, *c)
	}

	return candidates
}

// ProjectCandidate is a potential project discovered from session data.
type ProjectCandidate struct {
	Path         string
	Name         string
	SessionCount int
	AgentTypes   map[model.AgentType]bool
}

// AgentTypeList returns a sorted list of agent types for this candidate.
func (pc *ProjectCandidate) AgentTypeList() []string {
	var types []string
	for t := range pc.AgentTypes {
		types = append(types, string(t))
	}
	return types
}

// String returns a human-readable description.
func (pc *ProjectCandidate) String() string {
	return fmt.Sprintf("%s (%s) — %d sessions across %d IDEs",
		pc.Name, pc.Path, pc.SessionCount, len(pc.AgentTypes))
}
