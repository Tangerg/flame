package executionctx

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

type scopeKey struct{}
type invocationOwnerKey struct{}
type runCapabilitiesKey struct{}

func WithScope(ctx context.Context, scope runs.ExecutionScope) context.Context {
	return context.WithValue(ctx, scopeKey{}, scope)
}

func Scope(ctx context.Context) (runs.ExecutionScope, bool) {
	if ctx == nil {
		return runs.ExecutionScope{}, false
	}
	scope, ok := ctx.Value(scopeKey{}).(runs.ExecutionScope)
	return scope, ok
}

func WithRunCapabilities(ctx context.Context, capabilities run.Capabilities) context.Context {
	return context.WithValue(ctx, runCapabilitiesKey{}, capabilities.Clone())
}

func RunCapabilities(ctx context.Context) (run.Capabilities, bool) {
	if ctx == nil {
		return run.Capabilities{}, false
	}
	capabilities, ok := ctx.Value(runCapabilitiesKey{}).(run.Capabilities)
	return capabilities.Clone(), ok
}

func CWD(ctx context.Context) (string, bool) {
	if scope, ok := Scope(ctx); ok && scope.CWD != "" {
		return scope.CWD, true
	}
	return "", false
}

func WorkspaceCWD(ctx context.Context) (string, bool) {
	if scope, ok := Scope(ctx); ok && scope.WorkspaceCWD != "" {
		return scope.WorkspaceCWD, true
	}
	return "", false
}

func Project(ctx context.Context) identity.ProjectID {
	if scope, ok := Scope(ctx); ok {
		return scope.Project
	}
	return identity.ProjectID{}
}

func Isolated(ctx context.Context) bool {
	scope, ok := Scope(ctx)
	return ok && scope.Isolated
}

func GoalIncarnationID(ctx context.Context) (goalref.IncarnationID, bool) {
	if scope, ok := Scope(ctx); ok && !scope.GoalIncarnationID.IsZero() {
		return scope.GoalIncarnationID, true
	}
	return goalref.IncarnationID{}, false
}

func SessionID(ctx context.Context) identity.SessionID {
	if scope, ok := Scope(ctx); ok {
		return scope.SessionID
	}
	return identity.SessionID{}
}

func WithInvocationOwner(ctx context.Context, callID string) context.Context {
	return context.WithValue(ctx, invocationOwnerKey{}, callID)
}

func InvocationOwner(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	owner, _ := ctx.Value(invocationOwnerKey{}).(string)
	return owner
}
