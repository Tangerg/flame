package checkpoint

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
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
	root      string   // base dir holding one shadow GIT_DIR per session
	treeLocks sync.Map // canonical cwd → *sync.Mutex, serializing that working tree
	repoLocks sync.Map // session id → *sync.Mutex, serializing one shadow repository
}

// NewStore roots the shadow repos at dir (e.g. <FLAME_HOME>/runtime/checkpoints).
func NewStore(dir string) *Store { return &Store{root: dir} }

func (s *Store) treeLockFor(cwd string) *sync.Mutex {
	mu, _ := s.treeLocks.LoadOrStore(cwd, &sync.Mutex{})
	return storedMutex(mu, "checkpoint tree lock")
}

func (s *Store) repoLockFor(sessionID string) *sync.Mutex {
	mu, _ := s.repoLocks.LoadOrStore(sessionID, &sync.Mutex{})
	return storedMutex(mu, "checkpoint repository lock")
}

func storedMutex(value any, owner string) *sync.Mutex {
	mu, ok := value.(*sync.Mutex)
	if !ok {
		panic(owner + " map contains a non-mutex value")
	}
	return mu
}

// DropSession removes every workspace-specific shadow repository owned by one
// Session (on Session delete).
func (s *Store) DropSession(sessionID string) error {
	mu := s.repoLockFor(sessionID)
	mu.Lock()
	defer mu.Unlock()
	return os.RemoveAll(s.sessionDir(sessionID))
}

func (s *Store) sessionDir(sessionID string) string { return filepath.Join(s.root, sessionID) }

func (s *Store) gitDir(sessionID, cwd string) string {
	digest := sha256.Sum256([]byte(cwd))
	return filepath.Join(s.sessionDir(sessionID), "workspace-"+hex.EncodeToString(digest[:]))
}
