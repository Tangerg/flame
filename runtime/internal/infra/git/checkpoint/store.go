package checkpoint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/runtime/internal/keylock"
)

// Store manages every Session/workspace shadow repository. It has two explicit
// synchronization domains: treeLocks serialize Git commands that inspect or
// mutate one shared working tree, while repoLocks serialize the lifecycle of one
// Session's checkpoint set (including DropSession). Snapshot and Restore always
// acquire tree then Session; DropSession touches only the Session, so the order
// is deadlock-free. Run lifecycle separately keeps same-Session admission held
// until a terminal Snapshot returns, preventing the next Run from crossing its
// own checkpoint boundary. A workspace digest is only a filesystem-safe index;
// each repository also persists and verifies the complete workspace identity.
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
