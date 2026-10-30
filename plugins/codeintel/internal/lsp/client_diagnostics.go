package lsp

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"
)

func (c *client) diagnostics(ctx context.Context, abs string, settle time.Duration) ([]Diagnostic, error) {
	version, err := c.ensureOpen(ctx, abs)
	if err != nil {
		return nil, err
	}
	return c.diagnosticsVersion(ctx, abs, version, settle)
}

var errDiagnosticsUnavailable = errors.New("lsp: diagnostics for the requested document version are not ready")

func (c *client) diagnosticsVersion(ctx context.Context, abs string, version int, settle time.Duration) ([]Diagnostic, error) {
	uri := pathToURI(abs)
	// Nudge servers that only publish on save (typescript-language-server) —
	// harmless for those that publish on change (gopls).
	if err := c.conn.Notify(ctx, "textDocument/didSave", didSaveParams{TextDocument: textDocumentIdentifier{URI: uri}}); err != nil {
		return nil, fmt.Errorf("lsp: notify didSave for %s: %w", abs, err)
	}
	deadline := time.NewTimer(settle)
	defer deadline.Stop()
	for {
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
		select {
		case <-wait: // a push arrived — re-check
		case <-deadline.C:
			return nil, errDiagnosticsUnavailable
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}
