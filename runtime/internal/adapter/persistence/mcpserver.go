package persistence

// MCPServerStore retains host-issued transport grants and per-source execution controls.
type MCPServerStore struct{ db *database }

func newMCPServerStore(db *database) *MCPServerStore { return &MCPServerStore{db: db} }
