package sqlite

import "database/sql"

type AgentMemoryStore struct {
	db *sql.DB
}

func NewAgentMemoryStore(db *sql.DB) *AgentMemoryStore {
	return &AgentMemoryStore{db: db}
}
