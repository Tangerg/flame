package lsp

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/flame/qimu/cancellation"
	"github.com/sourcegraph/jsonrpc2"
)

// DiagnosticsPendingError means the current document version has no completed
// analysis. An observed empty set succeeds only for that exact version.
type DiagnosticsPendingError struct {
	Version         int
	ObservedVersion *int
}

func (e *DiagnosticsPendingError) Error() string {
	if e.ObservedVersion == nil {
		return fmt.Sprintf("lsp: diagnostics pending for document version %d; no analysis observed", e.Version)
	}
	return fmt.Sprintf("lsp: diagnostics pending for document version %d; last observed version %d", e.Version, *e.ObservedVersion)
}

// diagnostics returns only a completed analysis of the version it synchronized.
func (c *client) diagnostics(ctx context.Context, abs string, settle time.Duration) ([]Diagnostic, error) {
	version, err := c.ensureOpen(ctx, abs)
	if err != nil {
		return nil, errors.Join(err, cancellation.Err(ctx))
	}
	return c.diagnosticsVersion(ctx, abs, version, settle)
}

func (c *client) diagnosticsVersion(ctx context.Context, abs string, version int, settle time.Duration) ([]Diagnostic, error) {
	uri := pathToURI(abs)
	// Some servers publish on save rather than change.
	if err := c.conn.Notify(ctx, "textDocument/didSave", didSaveParams{TextDocument: textDocumentIdentifier{URI: uri}}); err != nil {
		return nil, fmt.Errorf("lsp: notify didSave for %s: %w", abs, errors.Join(err, cancellation.Err(ctx)))
	}
	deadline := time.NewTimer(settle)
	defer deadline.Stop()
	expired := false
	for {
		if err := cancellation.Err(ctx); err != nil {
			return nil, err
		}
		select {
		case <-c.conn.DisconnectNotify():
			return nil, fmt.Errorf("lsp: diagnostics connection ended: %w", errors.Join(jsonrpc2.ErrClosed, c.close()))
		default:
		}
		c.mu.Lock()
		ds, ok := c.diags[uri]
		protocolErr := c.diagnosticsErr
		current := c.open[uri].version
		wait := c.updated
		c.mu.Unlock()
		if protocolErr != nil {
			return nil, fmt.Errorf("lsp: diagnostics notification: %w", protocolErr)
		}
		if current != version {
			return nil, errors.New("lsp: document changed during analysis")
		}
		if ok && ds.version == version {
			return slices.Clone(ds.diagnostics), nil
		}
		if expired {
			pending := &DiagnosticsPendingError{Version: version}
			if ok {
				pending.ObservedVersion = &ds.version
			}
			return nil, pending
		}
		select {
		case <-wait:
		case <-deadline.C:
			expired = true
		case <-c.conn.DisconnectNotify():
		case <-ctx.Done():
		}
	}
}
