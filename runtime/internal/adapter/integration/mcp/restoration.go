package mcp

import (
	"context"
	"errors"
	"sync"

	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/frame"
	sdk "github.com/Tangerg/go-sdk/mcp"

	"github.com/Tangerg/frame/process/runner"
)

type restorationEvidenceKey struct{}

// Evidence is local to this acquisition. Only Frame's admitted read of the
// original business owner may supply it; it grants no permission or lifecycle.
type restorationEvidence struct {
	mu       sync.Mutex
	recordID string
	binding  frame.BindingSnapshot
}

func (e *restorationEvidence) observe(recordID string, binding frame.BindingSnapshot) {
	if e.recordID != recordID {
		return
	}
	e.mu.Lock()
	e.binding = binding.Clone()
	e.mu.Unlock()
}

func (e *restorationEvidence) snapshot() frame.BindingSnapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.binding.Clone()
}

func (s *Installed) restoreRetained(ctx context.Context, original plugin.Activation, grant frame.ScopeGrant) error {
	evidence := &restorationEvidence{recordID: original.Identity.String()}
	attempt := context.WithValue(ctx, restorationEvidenceKey{}, evidence)
	use, err := s.host.AcquireRecord(attempt, evidence.recordID, frame.UseConfig{Owner: s.owner, Scope: grant})
	if err == errBindingNotRetained {
		// The admitted owner settled after the catalog listed its work; that
		// earlier observation cannot require restoration or authorize execution.
		return nil
	}
	if err != nil {
		return failedRestoration(ctx, original, evidence.snapshot(), errors.Join(err, s.host.Admission().RecoveryError()))
	}
	_, err = use.Close(context.WithoutCancel(ctx))
	return err
}

// restorationFailure belongs to the existing connection attempt. Its binding
// identity prevents a failed replacement from settling an original waiting Run.
type restorationFailure struct {
	binding   framevalue.Digest
	completed bool
	cause     error
}

func (e *restorationFailure) Error() string {
	return "mcp: restore original binding: " + e.cause.Error()
}
func (e *restorationFailure) Unwrap() error { return e.cause }

func failedRestoration(ctx context.Context, original plugin.Activation, attempted frame.BindingSnapshot, cause error) error {
	identity, err := attempted.Identity()
	return &restorationFailure{
		binding: original.Identity,
		// Frame read this exact original graph from its accepted business owner.
		// Cancellation still ends only the wait, without settling preparation.
		completed: ctx.Err() == nil && err == nil && identity == original.Identity,
		cause:     plugins.FrameworkError(cause),
	}
}

// RestorationFailure reads the current attempt's conclusion for this exact graph.
// Catalog absence and a superseded or unauthorized attempt supply no such evidence.
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
		errors.Is(cause, frame.ErrRecoveryRequired) || errors.Is(cause, runner.ErrUnknown) || errors.Is(cause, frame.ErrUnconfirmedRelease) {
		return false, cause
	}
	// Only the original MCP connection or an authoritative source refusal proves
	// source loss. A completed preparation can also have failed a storage read.
	return errors.Is(cause, sdk.ErrConnectionClosed) || errors.Is(cause, plugin.ErrUnavailable) || errors.Is(cause, plugin.ErrStale), cause
}
