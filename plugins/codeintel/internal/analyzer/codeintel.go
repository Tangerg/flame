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
	return answer(locs, err, func(locs []lsp.Location) string { return formatLocations(root, locs, "definition") })
}

// References returns every reference (including the declaration) to the
// symbol at the 1-based (line, column) in file, as root-relative locations.
func (a *Analyzer) References(ctx context.Context, root, file string, line, column int) (string, error) {
	locs, err := a.servers.References(ctx, root, file, toPosition(line, column))
	return answer(locs, err, func(locs []lsp.Location) string { return formatLocations(root, locs, "reference") })
}

// Implementation returns the concrete implementation site(s) of the interface
// or abstract method at the 1-based (line, column) in file — e.g. every type
// that implements the interface method under the cursor.
func (a *Analyzer) Implementation(ctx context.Context, root, file string, line, column int) (string, error) {
	locs, err := a.servers.Implementation(ctx, root, file, toPosition(line, column))
	return answer(locs, err, func(locs []lsp.Location) string { return formatLocations(root, locs, "implementation") })
}

// IncomingCalls lists the callers of the function/method at the 1-based
// (line, column) in file (who calls it). OutgoingCalls lists its callees.
func (a *Analyzer) IncomingCalls(ctx context.Context, root, file string, line, column int) (string, error) {
	syms, err := a.servers.IncomingCalls(ctx, root, file, toPosition(line, column))
	return answer(syms, err, func(syms []lsp.Symbol) string { return formatCalls(root, syms, "incoming calls") })
}

func (a *Analyzer) OutgoingCalls(ctx context.Context, root, file string, line, column int) (string, error) {
	syms, err := a.servers.OutgoingCalls(ctx, root, file, toPosition(line, column))
	return answer(syms, err, func(syms []lsp.Symbol) string { return formatCalls(root, syms, "outgoing calls") })
}

// Hover returns the hover text (type signature, documentation) for the
// symbol at the 1-based (line, column) in file.
func (a *Analyzer) Hover(ctx context.Context, root, file string, line, column int) (string, error) {
	text, err := a.servers.Hover(ctx, root, file, toPosition(line, column))
	return answer(text, err, func(text string) string {
		if text == "" {
			return "No hover information available at that position."
		}
		return text
	})
}

// DocumentSymbols lists the symbols declared in file (root-relative).
func (a *Analyzer) DocumentSymbols(ctx context.Context, root, file string) (string, error) {
	syms, err := a.servers.DocumentSymbols(ctx, root, file)
	return answer(syms, err, func(syms []lsp.Symbol) string { return formatSymbols(root, syms) })
}

// WorkspaceSymbols searches the whole workspace for symbols matching query.
func (a *Analyzer) WorkspaceSymbols(ctx context.Context, root, query string) (string, error) {
	syms, err := a.servers.WorkspaceSymbols(ctx, root, query)
	if err != nil && !errors.Is(err, lsp.ErrNoServer) && len(syms) > 0 {
		return formatSymbols(root, syms) + "\n\nWorkspace symbol search incomplete: " + err.Error(), nil
	}
	return answer(syms, err, func(syms []lsp.Symbol) string { return formatSymbols(root, syms) })
}

// Diagnostics returns the language server's current problems for file.
func (a *Analyzer) Diagnostics(ctx context.Context, root, file string) (string, error) {
	diags, err := a.servers.Diagnostics(ctx, root, file)
	return answer(diags, err, func(diags []lsp.Diagnostic) string { return formatDiagnostics(file, diags) })
}

// answer presents one language-server result. An unsupported file type
// informs the model instead of failing the tool call.
func answer[T any](result T, err error, present func(T) string) (string, error) {
	if errors.Is(err, lsp.ErrNoServer) {
		return noServerMsg, nil
	}
	if err != nil {
		return "", err
	}
	return present(result), nil
}
