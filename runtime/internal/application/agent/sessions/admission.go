package sessions

import (
	"context"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
)

type Admissions interface {
	AcquireSession(ctx context.Context, sessionID identity.SessionID) (release func(), ok bool, err error)
	AcquireWorkspaceRollback(ctx context.Context, sessionID identity.SessionID) (release func(), ok bool, err error)
	AcquireWorkingTreeMutation(cwd string) (release func(), ok bool, err error)
}

type WorkingTreeAdmission struct {
	release *releaseOnce
}

func (w WorkingTreeAdmission) Release() {
	if w.release != nil {
		w.release.run()
	}
}

type Admission struct {
	SessionID identity.SessionID
	release   *releaseOnce
}

func (a Admission) Release() {
	if a.release != nil {
		a.release.run()
	}
}

type releaseOnce struct {
	once sync.Once
	fn   func()
}

func newReleaseOnce(fn func()) *releaseOnce { return &releaseOnce{fn: fn} }

func (r *releaseOnce) run() {
	r.once.Do(r.fn)
}

func heldSessionAdmission(sessionID identity.SessionID, release func()) Admission {
	return Admission{
		SessionID: sessionID,
		release:   newReleaseOnce(release),
	}
}

func heldWorkingTreeAdmission(release func()) WorkingTreeAdmission {
	return WorkingTreeAdmission{release: newReleaseOnce(release)}
}

func (c *Coordinator) ClaimIdleSession(ctx context.Context, sessionID identity.SessionID) (Admission, error) {
	release, ok, leaseErr := c.admissions.AcquireSession(ctx, sessionID)
	if leaseErr != nil {
		return Admission{}, leaseErr
	}
	if !ok {
		return Admission{}, rundomain.ErrSessionBusy
	}
	admission := heldSessionAdmission(sessionID, release)
	open, err := c.interrupts.List(ctx, sessionID)
	if err != nil {
		admission.Release()
		return Admission{}, err
	}
	if len(open) > 0 {
		admission.Release()
		return Admission{}, rundomain.ErrSessionBusy
	}
	return admission, nil
}

func (c *Coordinator) ClaimSessionMutation(ctx context.Context, sessionID identity.SessionID) (Admission, error) {
	release, ok, leaseErr := c.admissions.AcquireSession(ctx, sessionID)
	if leaseErr != nil {
		return Admission{}, leaseErr
	}
	if !ok {
		return Admission{}, rundomain.ErrSessionBusy
	}
	return heldSessionAdmission(sessionID, release), nil
}
