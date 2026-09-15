package goals

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/flame/runtime/internal/keylock"
)

// SessionMutations serializes session lifecycle write-sets with Goal commands
// and owns the in-process registry of active Goal drives. It is created before
// either coordinator so both use cases share one stable lifecycle boundary
// without late-bound references.
type SessionMutations struct {
	// admission excludes every command while shutdown flips its closed flag and
	// cancels drives. That critical section performs no I/O and never waits, so a
	// reader's wait is bounded by the flag write rather than by another command.
	admission sync.RWMutex

	// commands serializes lifecycle commands per Session. The wait belongs to the
	// caller's context: the holder owns a whole durable write-set, so a command
	// that queued behind one must be able to leave when its request ends.
	commands *keylock.Set

	mu     sync.Mutex
	drives map[string]*goalDrive
}

// NewSessionMutations returns the shared lifecycle coordinator for one runtime.
func NewSessionMutations() *SessionMutations {
	return &SessionMutations{commands: keylock.NewSet(), drives: map[string]*goalDrive{}}
}

// acquire serializes lifecycle commands only for the sessions they mutate.
// Session locks are taken before admission so a command waiting behind internal
// Goal drive reconciliation does not prevent shutdown from closing task admission.
// Once the read side is held, shutdown cannot cross the command's launch
// boundary.
func (s *SessionMutations) acquire(ctx context.Context, sessionIDs ...string) (func(), error) {
	releaseSessions, err := s.acquireSessions(ctx, sessionIDs...)
	if err != nil {
		return nil, err
	}
	s.admission.RLock()
	return func() {
		s.admission.RUnlock()
		releaseSessions()
	}, nil
}

// acquireSessions is the internal half of acquire. Background reconciliation
// participates in per-session ordering but not external command admission; its
// task-group ownership is the shutdown join boundary.
func (s *SessionMutations) acquireSessions(ctx context.Context, sessionIDs ...string) (func(), error) {
	return s.commands.AcquireAll(ctx, sessionIDs...)
}

func normalizeSessionIDs(sessionIDs []string) []string {
	ids := slices.Clone(sessionIDs)
	slices.Sort(ids)
	return slices.Compact(ids)
}

func (s *SessionMutations) acquireAll() func() {
	s.admission.Lock()
	return s.admission.Unlock
}

// WithSessionMutation owns both phases of a session mutation. commit decides
// the command: a failure leaves the authoritative Goal drive intact and is
// returned to the caller. Once commit succeeds the mutation has happened, so
// draining affected drives and afterCommit are settlement — their failures are
// recorded against the operation instead of returned, because a caller that
// projected one would report a committed mutation as one that never happened.
func (s *SessionMutations) WithSessionMutation(
	ctx context.Context,
	sessionIDs []string,
	commit func(context.Context) error,
	afterCommit func(context.Context) error,
) error {
	sessionIDs = normalizeSessionIDs(sessionIDs)
	release, err := s.acquire(ctx, sessionIDs...)
	if err != nil {
		return err
	}
	defer release()
	if err := commit(ctx); err != nil {
		return err
	}
	type ownedDrive struct {
		sessionID string
		drive     *goalDrive
	}
	drives := make([]ownedDrive, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		if drive := s.quiesce(sessionID); drive != nil {
			drives = append(drives, ownedDrive{sessionID: sessionID, drive: drive})
		}
	}
	var errs []error
	for _, owned := range drives {
		errs = append(errs, owned.drive.await(ctx))
		if owned.drive.completed() {
			s.forget(owned.sessionID, owned.drive)
		}
	}
	errs = append(errs, afterCommit(ctx))
	if settlement := errors.Join(errs...); settlement != nil {
		trace.SpanFromContext(ctx).RecordError(settlement)
		slog.ErrorContext(ctx, "goals: committed session mutation did not settle",
			"sessions", sessionIDs, "error", settlement)
	}
	return nil
}

func (s *SessionMutations) launch(sessionID string, drive *goalDrive) {
	s.mu.Lock()
	if s.drives == nil {
		s.drives = map[string]*goalDrive{}
	}
	if s.drives[sessionID] != nil {
		s.mu.Unlock()
		panic("goals: launch attempted before the prior Goal drive was joined")
	}
	s.drives[sessionID] = drive
	s.mu.Unlock()
}

func (s *SessionMutations) forget(sessionID string, drive *goalDrive) {
	s.mu.Lock()
	if s.drives[sessionID] == drive {
		delete(s.drives, sessionID)
	}
	s.mu.Unlock()
}

func (s *SessionMutations) quiesce(sessionID string) *goalDrive {
	s.mu.Lock()
	drive := s.drives[sessionID]
	s.mu.Unlock()
	if drive != nil {
		drive.quiesce()
	}
	return drive
}

func (s *SessionMutations) activeDrive(sessionID string) *goalDrive {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.drives[sessionID]
}
