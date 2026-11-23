package goals

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/keylock"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
	"go.opentelemetry.io/otel/trace"
)

type SessionMutations struct {
	// The critical section performs no I/O and never waits, so a reader's wait is
	// bounded by the flag write.
	admission sync.RWMutex

	commands *keylock.Set

	mu     sync.Mutex
	drives map[identity.SessionID]*goalDrive
}

func NewSessionMutations() *SessionMutations {
	return &SessionMutations{commands: keylock.NewSet(), drives: map[identity.SessionID]*goalDrive{}}
}

// Session locks are taken before admission so a command waiting behind drive
// reconciliation cannot prevent shutdown from closing task admission.
func (s *SessionMutations) acquire(ctx context.Context, sessionIDs ...identity.SessionID) (func(), error) {
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

func (s *SessionMutations) acquireSessions(ctx context.Context, sessionIDs ...identity.SessionID) (func(), error) {
	keys := make([]string, len(sessionIDs))
	for index, sessionID := range sessionIDs {
		keys[index] = sessionID.String()
	}
	return s.commands.AcquireAll(ctx, keys...)
}

func normalizeSessionIDs(sessionIDs []identity.SessionID) []identity.SessionID {
	ids := slices.Clone(sessionIDs)
	slices.SortFunc(ids, identity.SessionID.Compare)
	return slices.Compact(ids)
}

func (s *SessionMutations) acquireAll() func() {
	s.admission.Lock()
	return s.admission.Unlock
}

func (s *SessionMutations) WithSessionMutation(
	ctx context.Context,
	sessionIDs []identity.SessionID,
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
		sessionID identity.SessionID
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
		errs = append(errs, s.joinDrive(ctx, owned.sessionID, owned.drive))
	}
	errs = append(errs, afterCommit(ctx))
	if settlement := errors.Join(errs...); settlement != nil {
		trace.SpanFromContext(ctx).RecordError(settlement)
		slog.ErrorContext(ctx, "goals: committed session mutation did not settle",
			slog.Any("sessions", sessionIDs), telemetry.Error(settlement))
	}
	return nil
}

func (s *SessionMutations) installDrive(sessionID identity.SessionID, drive *goalDrive) *goalDrive {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.drives[sessionID]
	s.drives[sessionID] = drive
	return previous
}

func (s *SessionMutations) forget(sessionID identity.SessionID, drive *goalDrive) {
	s.mu.Lock()
	if s.drives[sessionID] == drive {
		delete(s.drives, sessionID)
	}
	s.mu.Unlock()
}

func (s *SessionMutations) quiesce(sessionID identity.SessionID) *goalDrive {
	drive := s.activeDrive(sessionID)
	if drive != nil {
		drive.quiesce()
	}
	return drive
}

func (s *SessionMutations) joinDrive(ctx context.Context, sessionID identity.SessionID, drive *goalDrive) error {
	if drive == nil {
		return nil
	}
	err := drive.await(ctx)
	if drive.completed() {
		s.forget(sessionID, drive)
	}
	return err
}

func (s *SessionMutations) activeDrive(sessionID identity.SessionID) *goalDrive {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.drives[sessionID]
}
