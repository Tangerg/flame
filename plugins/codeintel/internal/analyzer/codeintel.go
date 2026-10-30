package analyzer

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/plugins/codeintel/internal/lsp"
	"github.com/samber/lo"
)

// noServerMsg is returned (with a nil error) when no language server
// handles the target file type, so an unsupported file informs the model
// instead of halting the tool loop.
const noServerMsg = "No language server is available for that file type."

// Analyzer wraps language-server connections as the code-intelligence adapter
// surface. Servers launch lazily per (workspace root, language); Close shuts
// them all down.
type Analyzer struct {
	servers *lsp.Servers
}

// New builds an Analyzer over the given language-server table and the
// process-owned lifetime from which lazy language-server launches derive.
// A nil table selects defaults; an explicitly empty table disables language servers.
func New(lifetime context.Context, servers []lsp.ServerSpec) (*Analyzer, error) {
	if lo.IsNil(lifetime) {
		return nil, errors.New("codeintel: lifetime is required")
	}
	if servers == nil {
		servers = lsp.DefaultServers()
	}
	set, err := lsp.NewServers(lifetime, servers)
	if err != nil {
		return nil, err
	}
	return &Analyzer{servers: set}, nil
}

// Close shuts down every launched language server.
func (a *Analyzer) Close() error {
	return a.servers.Close()
}

// Supported reports whether a configured language server handles file's type.
func (a *Analyzer) Supported(file string) bool {
	return a.servers.Supported(file)
}

// Definition returns the declaration location(s) of the symbol at the
// 1-based (line, column) in file, as root-relative file:line:col lines.
func (a *Analyzer) Definition(ctx context.Context, root, file string, line, column int) (string, error) {
	locs, err := a.servers.Definition(ctx, root, file, toPosition(line, column))
	if msg, handled := foldNoServer(err); handled {
		return msg, nil
	}
	if err != nil {
		return "", err
	}
	return formatLocations(root, locs, "definition"), nil
}

// References returns every reference (including the declaration) to the
// symbol at the 1-based (line, column) in file, as root-relative locations.
func (a *Analyzer) References(ctx context.Context, root, file string, line, column int) (string, error) {
	locs, err := a.servers.References(ctx, root, file, toPosition(line, column))
	if msg, handled := foldNoServer(err); handled {
		return msg, nil
	}
	if err != nil {
		return "", err
	}
	return formatLocations(root, locs, "reference"), nil
}

// Implementation returns the concrete implementation site(s) of the interface
// or abstract method at the 1-based (line, column) in file — e.g. every type
// that implements the interface method under the cursor.
func (a *Analyzer) Implementation(ctx context.Context, root, file string, line, column int) (string, error) {
	locs, err := a.servers.Implementation(ctx, root, file, toPosition(line, column))
	if msg, handled := foldNoServer(err); handled {
		return msg, nil
	}
	if err != nil {
		return "", err
	}
	return formatLocations(root, locs, "implementation"), nil
}

// IncomingCalls lists the callers of the function/method at the 1-based
// (line, column) in file (who calls it). OutgoingCalls lists its callees.
func (a *Analyzer) IncomingCalls(ctx context.Context, root, file string, line, column int) (string, error) {
	syms, err := a.servers.IncomingCalls(ctx, root, file, toPosition(line, column))
	if msg, handled := foldNoServer(err); handled {
		return msg, nil
	}
	if err != nil {
		return "", err
	}
	return formatCalls(root, syms, "incoming calls"), nil
}

func (a *Analyzer) OutgoingCalls(ctx context.Context, root, file string, line, column int) (string, error) {
	syms, err := a.servers.OutgoingCalls(ctx, root, file, toPosition(line, column))
	if msg, handled := foldNoServer(err); handled {
		return msg, nil
	}
	if err != nil {
		return "", err
	}
	return formatCalls(root, syms, "outgoing calls"), nil
}

// Hover returns the hover text (type signature, documentation) for the
// symbol at the 1-based (line, column) in file.
func (a *Analyzer) Hover(ctx context.Context, root, file string, line, column int) (string, error) {
	text, err := a.servers.Hover(ctx, root, file, toPosition(line, column))
	if msg, handled := foldNoServer(err); handled {
		return msg, nil
	}
	if err != nil {
		return "", err
	}
	if text == "" {
		return "No hover information available at that position.", nil
	}
	return text, nil
}

// DocumentSymbols lists the symbols declared in file (root-relative).
func (a *Analyzer) DocumentSymbols(ctx context.Context, root, file string) (string, error) {
	syms, err := a.servers.DocumentSymbols(ctx, root, file)
	if msg, handled := foldNoServer(err); handled {
		return msg, nil
	}
	if err != nil {
		return "", err
	}
	return formatSymbols(root, syms), nil
}

// WorkspaceSymbols searches the whole workspace for symbols matching query.
func (a *Analyzer) WorkspaceSymbols(ctx context.Context, root, query string) (string, error) {
	syms, err := a.servers.WorkspaceSymbols(ctx, root, query)
	if msg, handled := foldNoServer(err); handled {
		return msg, nil
	}
	if err != nil {
		if len(syms) == 0 {
			return "", err
		}
		return formatSymbols(root, syms) + "\n\nWorkspace symbol search incomplete: " + err.Error(), nil
	}
	return formatSymbols(root, syms), nil
}

// Diagnostics returns the language server's current problems for file.
func (a *Analyzer) Diagnostics(ctx context.Context, root, file string) (string, error) {
	diags, err := a.servers.Diagnostics(ctx, root, file)
	if msg, handled := foldNoServer(err); handled {
		return msg, nil
	}
	if err != nil {
		return "", err
	}
	return formatDiagnostics(file, diags), nil
}

// foldNoServer maps the "no language server for this file type" sentinel to
// a plain model-facing message; handled is true only for that case.
func foldNoServer(err error) (msg string, handled bool) {
	if errors.Is(err, lsp.ErrNoServer) {
		return noServerMsg, true
	}
	return "", false
}
