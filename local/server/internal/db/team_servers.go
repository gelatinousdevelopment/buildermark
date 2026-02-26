package db

import (
	"context"
	"database/sql"
	"fmt"
)

// TeamServer represents a row in the team_servers table.
type TeamServer struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	URL    string `json:"url"`
	APIKey string `json:"apiKey"`
}

// ListTeamServers returns all team servers ordered by label.
func ListTeamServers(ctx context.Context, db *sql.DB) ([]TeamServer, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, label, url, api_key FROM team_servers ORDER BY label")
	if err != nil {
		return nil, fmt.Errorf("query team_servers: %w", err)
	}
	defer rows.Close()

	servers := []TeamServer{}
	for rows.Next() {
		var s TeamServer
		if err := rows.Scan(&s.ID, &s.Label, &s.URL, &s.APIKey); err != nil {
			return nil, fmt.Errorf("scan team_server: %w", err)
		}
		servers = append(servers, s)
	}
	return servers, rows.Err()
}

// GetTeamServer returns a single team server by ID.
func GetTeamServer(ctx context.Context, db *sql.DB, id string) (*TeamServer, error) {
	var s TeamServer
	err := db.QueryRowContext(ctx, "SELECT id, label, url, api_key FROM team_servers WHERE id = ?", id).Scan(&s.ID, &s.Label, &s.URL, &s.APIKey)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("team_server %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("query team_server: %w", err)
	}
	return &s, nil
}

// CreateTeamServer inserts a new team server and returns the created record.
func CreateTeamServer(ctx context.Context, db *sql.DB, label, url, apiKey string) (*TeamServer, error) {
	id := newID()
	_, err := db.ExecContext(ctx, "INSERT INTO team_servers (id, label, url, api_key) VALUES (?, ?, ?, ?)", id, label, url, apiKey)
	if err != nil {
		return nil, fmt.Errorf("insert team_server: %w", err)
	}
	return &TeamServer{ID: id, Label: label, URL: url, APIKey: apiKey}, nil
}

// UpdateTeamServer updates an existing team server.
func UpdateTeamServer(ctx context.Context, db *sql.DB, id, label, url, apiKey string) error {
	res, err := db.ExecContext(ctx, "UPDATE team_servers SET label = ?, url = ?, api_key = ? WHERE id = ?", label, url, apiKey, id)
	if err != nil {
		return fmt.Errorf("update team_server: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("team_server %s: %w", id, ErrNotFound)
	}
	return nil
}

// DeleteTeamServer removes a team server and clears any project references to it.
func DeleteTeamServer(ctx context.Context, database *sql.DB, id string) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Clear references from projects.
	if _, err := tx.ExecContext(ctx, "UPDATE projects SET team_server_id = '' WHERE team_server_id = ?", id); err != nil {
		return fmt.Errorf("clear project references: %w", err)
	}

	res, err := tx.ExecContext(ctx, "DELETE FROM team_servers WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete team_server: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("team_server %s: %w", id, ErrNotFound)
	}

	return tx.Commit()
}

// SetProjectTeamServer sets the team_server_id on a project.
func SetProjectTeamServer(ctx context.Context, db *sql.DB, projectID, teamServerID string) error {
	res, err := db.ExecContext(ctx, "UPDATE projects SET team_server_id = ? WHERE id = ?", teamServerID, projectID)
	if err != nil {
		return fmt.Errorf("update project team_server_id: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("project %s: %w", projectID, ErrNotFound)
	}
	return nil
}
