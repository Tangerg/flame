package mcp

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/runner"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

// restorationFailure belongs to the existing connection attempt. Its binding
// identity prevents a failed replacement from settling an original waiting Run.
type restorationFailure struct {
	binding   identity.Digest
	completed bool
	cause     error
}

func (e *restorationFailure) Error() string {
	return "mcp: restore original binding: " + e.cause.Error()
}
func (e *restorationFailure) Unwrap() error { return e.cause }

func failedRestoration(ctx context.Context, original plugin.Activation, attempted qimu.BindingSnapshot, cause error) error {
	return &restorationFailure{
		binding: original.Identity,
		// A returned graph names an admitted preparation. Cancellation only ends
		// the caller's wait and does not settle that original preparation.
		completed: ctx.Err() == nil && attempted.Validate() == nil,
		cause:     plugins.FrameworkError(cause),
	}
}

// RestorationFailure reads the current attempt's conclusion for this exact graph.
// Catalog absence and an unstarted or superseded attempt supply no such evidence.
// A non-nil error with definite=false retains an inconclusive original attempt.
func (p *Pool) RestorationFailure(id mcpserver.ID, activation plugin.Activation) (definite bool, err error) {
	p.inner.mu.Lock()
	defer p.inner.mu.Unlock()
	current := p.inner.find(id)
	if current == nil || current.state != mcpserver.ConnectionFailed {
		return false, nil
	}
	failure, found := errors.AsType[*restorationFailure](current.cause)
	if !found || failure.binding != activation.Identity {
		return false, nil
	}
	cause := current.cause
	if !failure.completed || errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) ||
		errors.Is(cause, qimu.ErrRecoveryRequired) || errors.Is(cause, runner.ErrUnknown) || errors.Is(cause, qimu.ErrUnconfirmedRelease) {
		return false, cause
	}
	// Only the original MCP connection or an authoritative source refusal proves
	// source loss. A completed preparation can also have failed a storage read.
	return errors.Is(cause, sdk.ErrConnectionClosed) || errors.Is(cause, plugin.ErrUnavailable) || errors.Is(cause, plugin.ErrStale), cause
}
