package checkpoint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/runtime/internal/keylock"
)

// Snapshot and Restore acquire the tree lock then the Session lock; DropSession
// takes only the Session lock, so the order is deadlock-free.
type Store struct {
	root      string
	treeLocks *keylock.Set
	repoLocks *keylock.Set
}

func NewStore(dir string) *Store {
	return &Store{root: dir, treeLocks: keylock.NewSet(), repoLocks: keylock.NewSet()}
}

func (s *Store) acquireWorkspace(ctx context.Context, sessionID, cwd string) (func(), error) {
	tree, err := s.treeLocks.Acquire(ctx, cwd)
	if err != nil {
		return nil, err
	}
	repo, err := s.repoLocks.Acquire(ctx, sessionID)
	if err != nil {
		tree()
		return nil, err
	}
	return func() { repo(); tree() }, nil
}

func (s *Store) DropSession(ctx context.Context, sessionID string) error {
	release, err := s.repoLocks.Acquire(ctx, sessionID)
	if err != nil {
		return err
	}
	defer release()
	return os.RemoveAll(s.sessionDir(sessionID))
}

func (s *Store) sessionDir(sessionID string) string { return filepath.Join(s.root, sessionID) }

func (s *Store) gitDir(sessionID, cwd string) string {
	digest := sha256.Sum256([]byte(cwd))
	return filepath.Join(s.sessionDir(sessionID), "workspace-"+hex.EncodeToString(digest[:]))
}
