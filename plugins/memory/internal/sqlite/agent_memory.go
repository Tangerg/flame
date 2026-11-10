package sqlite

import framesqlite "github.com/Tangerg/frame/sqlite"

type AgentMemoryStore struct {
	db *framesqlite.Database
}

func NewAgentMemoryStore(db *framesqlite.Database) *AgentMemoryStore {
	return &AgentMemoryStore{db: db}
}
