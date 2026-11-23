package analyzer

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/plugins/codeintel/internal/lsp"
	"github.com/samber/lo"
)

const noServerMsg = "No language server is available for that file type."

type Analyzer struct {
	servers *lsp.Servers
}

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

func (a *Analyzer) Close() error {
	return a.servers.Close()
}

func (a *Analyzer) Supported(file string) bool {
	return a.servers.Supported(file)
}

func (a *Analyzer) Definition(ctx context.Context, root, file string, line, column int) (string, error) {
	locs, err := a.servers.Definition(ctx, root, file, toPosition(line, column))
	return answer(locs, err, func(locs []lsp.Location) string { return formatLocations(root, locs, "definition") })
}

func (a *Analyzer) References(ctx context.Context, root, file string, line, column int) (string, error) {
	locs, err := a.servers.References(ctx, root, file, toPosition(line, column))
	return answer(locs, err, func(locs []lsp.Location) string { return formatLocations(root, locs, "reference") })
}

func (a *Analyzer) Implementation(ctx context.Context, root, file string, line, column int) (string, error) {
	locs, err := a.servers.Implementation(ctx, root, file, toPosition(line, column))
	return answer(locs, err, func(locs []lsp.Location) string { return formatLocations(root, locs, "implementation") })
}

func (a *Analyzer) IncomingCalls(ctx context.Context, root, file string, line, column int) (string, error) {
	syms, err := a.servers.IncomingCalls(ctx, root, file, toPosition(line, column))
	return answer(syms, err, func(syms []lsp.Symbol) string { return formatCalls(root, syms, "incoming calls") })
}

func (a *Analyzer) OutgoingCalls(ctx context.Context, root, file string, line, column int) (string, error) {
	syms, err := a.servers.OutgoingCalls(ctx, root, file, toPosition(line, column))
	return answer(syms, err, func(syms []lsp.Symbol) string { return formatCalls(root, syms, "outgoing calls") })
}

func (a *Analyzer) Hover(ctx context.Context, root, file string, line, column int) (string, error) {
	text, err := a.servers.Hover(ctx, root, file, toPosition(line, column))
	return answer(text, err, func(text string) string {
		if text == "" {
			return "No hover information available at that position."
		}
		return text
	})
}

func (a *Analyzer) DocumentSymbols(ctx context.Context, root, file string) (string, error) {
	syms, err := a.servers.DocumentSymbols(ctx, root, file)
	return answer(syms, err, func(syms []lsp.Symbol) string { return formatSymbols(root, syms) })
}

func (a *Analyzer) WorkspaceSymbols(ctx context.Context, root, query string) (string, error) {
	syms, err := a.servers.WorkspaceSymbols(ctx, root, query)
	if err != nil && !errors.Is(err, lsp.ErrNoServer) && len(syms) > 0 {
		return formatSymbols(root, syms) + "\n\nWorkspace symbol search incomplete: " + err.Error(), nil
	}
	return answer(syms, err, func(syms []lsp.Symbol) string { return formatSymbols(root, syms) })
}

func (a *Analyzer) Diagnostics(ctx context.Context, root, file string) (string, error) {
	diags, err := a.servers.Diagnostics(ctx, root, file)
	return answer(diags, err, func(diags []lsp.Diagnostic) string { return formatDiagnostics(file, diags) })
}

func answer[T any](result T, err error, present func(T) string) (string, error) {
	if errors.Is(err, lsp.ErrNoServer) {
		return noServerMsg, nil
	}
	if err != nil {
		return "", err
	}
	return present(result), nil
}
