package persistence

type SessionStore struct {
	db *database
}

func newSessionStore(db *database) *SessionStore {
	return &SessionStore{db: db}
}
