package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/kuk1song/slashstage/internal/model"
)

// CreateProject inserts a new project and returns it with the generated ID.
func (db *DB) CreateProject(name, path, gitRemote, gitBranch string) (*model.Project, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(
		`INSERT INTO projects (name, path, git_remote, git_branch, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		name, path, gitRemote, gitBranch, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert project: %w", err)
	}
	id, _ := res.LastInsertId()
	return &model.Project{
		ID:        id,
		Name:      name,
		Path:      path,
		GitRemote: gitRemote,
		GitBranch: gitBranch,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}, nil
}

// GetProject returns a project by ID.
func (db *DB) GetProject(id int64) (*model.Project, error) {
	row := db.QueryRow(`SELECT id, name, path, git_remote, git_branch, created_at, updated_at FROM projects WHERE id = ?`, id)
	return scanProject(row)
}

// GetProjectByPath returns a project by its absolute path.
func (db *DB) GetProjectByPath(path string) (*model.Project, error) {
	row := db.QueryRow(`SELECT id, name, path, git_remote, git_branch, created_at, updated_at FROM projects WHERE path = ?`, path)
	return scanProject(row)
}

// ListProjects returns all projects ordered by last update.
func (db *DB) ListProjects() ([]model.Project, error) {
	rows, err := db.Query(`SELECT id, name, path, git_remote, git_branch, created_at, updated_at FROM projects ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var projects []model.Project
	for rows.Next() {
		p, err := scanProjectRow(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, *p)
	}
	return projects, rows.Err()
}

// DeleteProject removes a project by ID.
func (db *DB) DeleteProject(id int64) error {
	_, err := db.Exec(`DELETE FROM projects WHERE id = ?`, id)
	return err
}

func scanProject(row *sql.Row) (*model.Project, error) {
	var p model.Project
	var createdAt, updatedAt string
	err := row.Scan(&p.ID, &p.Name, &p.Path, &p.GitRemote, &p.GitBranch, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &p, nil
}

func scanProjectRow(rows *sql.Rows) (*model.Project, error) {
	var p model.Project
	var createdAt, updatedAt string
	err := rows.Scan(&p.ID, &p.Name, &p.Path, &p.GitRemote, &p.GitBranch, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &p, nil
}
