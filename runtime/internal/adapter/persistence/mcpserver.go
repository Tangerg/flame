package persistence

import "database/sql"

// MCPServerStore retains host-issued transport grants and per-source execution controls.
type MCPServerStore struct{ db *sql.DB }

func newMCPServerStore(db *sql.DB) *MCPServerStore { return &MCPServerStore{db: db} }
