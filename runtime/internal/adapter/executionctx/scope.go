package executionctx

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

type scopeKey struct{}

type executionScope struct {
	target       runs.ExecutionScope
	capabilities run.Capabilities
	model        modelCallScope
}

// A new scope carries caller provenance through the parent context, but its
// calls cannot publish into the execution that requested it.
func WithSessionScope(ctx context.Context, sessionID identity.SessionID, workspace string) context.Context {
	return context.WithValue(ctx, scopeKey{}, executionScope{
		target: runs.ExecutionScope{SessionID: sessionID, CWD: workspace, WorkspaceCWD: workspace},
	})
}

func WithRunScope(ctx context.Context, target runs.ExecutionScope, capabilities run.Capabilities) context.Context {
	return context.WithValue(ctx, scopeKey{}, executionScope{target: target, capabilities: capabilities.Normalized()})
}

func currentScope(ctx context.Context) executionScope {
	scope, _ := ctx.Value(scopeKey{}).(executionScope)
	return scope
}

func RunCapabilities(ctx context.Context) run.Capabilities {
	return currentScope(ctx).capabilities.Clone()
}

func CWD(ctx context.Context) (string, bool) {
	cwd := currentScope(ctx).target.CWD
	return cwd, cwd != ""
}

func WorkspaceCWD(ctx context.Context) (string, bool) {
	cwd := currentScope(ctx).target.WorkspaceCWD
	return cwd, cwd != ""
}

func Project(ctx context.Context) identity.ProjectID {
	return currentScope(ctx).target.Project
}

func Isolated(ctx context.Context) bool {
	return currentScope(ctx).target.Isolated
}

func GoalIncarnationID(ctx context.Context) (goalref.IncarnationID, bool) {
	id := currentScope(ctx).target.GoalIncarnationID
	return id, !id.IsZero()
}

func SessionID(ctx context.Context) identity.SessionID {
	return currentScope(ctx).target.SessionID
}
