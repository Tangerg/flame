// Package approvals owns the runtime tool-permission use cases: the approval
// stance (mode) and the persisted per-session/project/global approval rules.
package approvals

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

// SessionLookup resolves a session so rule listing can scope rules to the
// session's project directory. A successful read must return the valid Session
// whose identity was requested. The session store satisfies it.
type SessionLookup interface {
	Get(ctx context.Context, id string) (session.Session, error)
}

// Policy is the approval-management use case's view of runtime policy. Tool
// call evaluation consumes a separate, narrower policy view.
type Policy interface {
	DefaultMode(ctx context.Context) (approval.Mode, error)
	SetDefaultMode(ctx context.Context, mode approval.Mode) error
	Rules(ctx context.Context, sessionID, projectDir string) ([]approval.Rule, error)
	Forget(ctx context.Context, id string) error
}

// Coordinator drives the tool-permission stance + approval-rule use cases.
type Coordinator struct {
	policy   Policy
	sessions SessionLookup
}

// New returns a Coordinator over the approval policy + the session lookup its
// rule scoping reads.
func New(policy Policy, sessions SessionLookup) *Coordinator {
	return &Coordinator{policy: policy, sessions: sessions}
}

// DefaultMode returns the runtime fallback for sessions without an explicit
// permission mode.
func (c *Coordinator) DefaultMode(ctx context.Context) (approval.Mode, error) {
	return c.policy.DefaultMode(ctx)
}

// SetDefaultMode changes the runtime fallback. Plan mode remains session-only.
func (c *Coordinator) SetDefaultMode(ctx context.Context, mode approval.Mode) error {
	return c.policy.SetDefaultMode(ctx, mode)
}

// ListRules returns the rules visible from a session. Unknown sessions degrade to
// session/global lookup; storage failures are real errors.
func (c *Coordinator) ListRules(ctx context.Context, sessionID string) ([]approval.Rule, error) {
	cwd := ""
	if sessionID != "" {
		switch sess, err := c.sessions.Get(ctx, sessionID); {
		case err == nil:
			if err := sess.Validate(); err != nil {
				return nil, fmt.Errorf("approvals: invalid session %q: %w", sessionID, err)
			}
			if sess.ID() != sessionID {
				return nil, fmt.Errorf("approvals: requested session %q, got %q", sessionID, sess.ID())
			}
			cwd = sess.Workspace().Path()
		case !errors.Is(err, session.ErrNotFound):
			return nil, err
		}
	}
	return c.policy.Rules(ctx, sessionID, cwd)
}

// ForgetRule removes one persisted approval rule by id.
func (c *Coordinator) ForgetRule(ctx context.Context, id string) error {
	return c.policy.Forget(ctx, id)
}
