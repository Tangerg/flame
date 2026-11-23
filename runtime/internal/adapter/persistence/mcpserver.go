package persistence

type MCPServerStore struct{ db *database }

func newMCPServerStore(db *database) *MCPServerStore { return &MCPServerStore{db: db} }
