// Package approvals owns the runtime tool-permission use cases: the approval
// stance (mode) and the persisted per-session/project/global approval rules.
package approvals

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

// SessionLookup resolves the validated application-owned Session read so rule
// listing can scope rules to its project directory.
type SessionLookup interface {
	Get(ctx context.Context, id string) (session.Session, error)
}

// Policy is the approval-management use case's view of runtime policy. Tool
// call evaluation consumes a separate, narrower policy view.
type Policy interface {
	DefaultMode(ctx context.Context) (approval.Mode, error)
	SetDefaultMode(ctx context.Context, mode approval.Mode) error
	Rules(ctx context.Context, sessionID, projectDir string) ([]RuleView, error)
	Forget(ctx context.Context, id string) error
	SetRule(context.Context, RuleChange, string) error
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
func (c *Coordinator) ListRules(ctx context.Context, sessionID string) ([]RuleView, error) {
	cwd := ""
	if sessionID != "" {
		switch sess, err := c.sessions.Get(ctx, sessionID); {
		case err == nil:
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

type RuleChange struct {
	Tool      tool.Ref
	Scope     approval.Scope
	SessionID string
	Subject   approval.Subject
	Decision  approval.Decision
}

func (c *Coordinator) SetRule(ctx context.Context, change RuleChange) error {
	cwd := ""
	if change.Scope != approval.ScopeGlobal {
		sess, err := c.sessions.Get(ctx, change.SessionID)
		if err != nil {
			return err
		}
		cwd = sess.Workspace().Path()
	}
	return c.policy.SetRule(ctx, change, cwd)
}
