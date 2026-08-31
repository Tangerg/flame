// LSP exposes language-server capabilities as one model tool.
package builtin

import (
	"context"
	"errors"
	"fmt"
	"strings"

	toolcontract "github.com/Tangerg/scope/core/tool"

	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset/codeintel"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

// BuildLSP exposes the code-intelligence analyzer as one `lsp` tool whose
// operation selects the query. Keeping diagnostics in the same operation
// vocabulary avoids two model-visible names for one language-server capability.
//
// The analyzer is working-directory independent — it keys servers by workspace
// root internally — so these tools are built ONCE and read the Run's cwd off
// application context at call time (the per-session-cwd seam shared with fs /
// shell). Positions are 1-based at the tool boundary (what a human/LLM reads
// off a file); the analyzer converts to the LSP 0-based wire form and folds an
// unsupported file type into a plain reply.
func BuildLSP(ci *codeintel.Analyzer, defaultCWD string) ([]toolcontract.Tool, error) {
	if ci == nil {
		return nil, errors.New("lsp: analyzer is nil")
	}
	lsp, err := newQuery(ci, defaultCWD)
	if err != nil {
		return nil, err
	}
	return []toolcontract.Tool{lsp}, nil
}

// lspInput is the model-facing argument shape; [toolcontract.NewFunc] derives the
// JSON schema from it and decodes calls back into it, so the advertised schema
// and parsed value cannot drift. Only `operation` is structurally required —
// which operand each operation needs is validated per-operation in the handler.
type lspInput struct {
	Operation lspOperation `json:"operation" jsonschema:"enum=definition,enum=references,enum=implementation,enum=hover,enum=incoming_calls,enum=outgoing_calls,enum=document_symbols,enum=workspace_symbols,enum=diagnostics" jsonschema_description:"Language-server query to run."`
	Path      string       `json:"path,omitempty" jsonschema_description:"File path, absolute or relative to the workspace root. Required except for workspace_symbols."`
	Line      *int         `json:"line,omitempty" jsonschema:"minimum=1" jsonschema_description:"1-based line of the symbol. Required for position operations and omitted otherwise."`
	Character *int         `json:"character,omitempty" jsonschema:"minimum=1" jsonschema_description:"1-based character (column) of the symbol. Required for position operations and omitted otherwise."`
	Query     string       `json:"query,omitempty" jsonschema_description:"Symbol name or substring to search for. Required for workspace_symbols."`
}

// lspPosition is the validated, complete coordinate used inside the adapter.
// Pointer presence belongs only to the model-facing DTO above; incomplete or
// non-positive coordinates never reach the analyzer.
type lspPosition struct {
	line      int
	character int
}

type lspQuery struct {
	operation   lspOperation
	path        string
	position    lspPosition
	symbolQuery string
}

type lspOperation string

const (
	lspDefinition       lspOperation = "definition"
	lspReferences       lspOperation = "references"
	lspImplementation   lspOperation = "implementation"
	lspHover            lspOperation = "hover"
	lspIncomingCalls    lspOperation = "incoming_calls"
	lspOutgoingCalls    lspOperation = "outgoing_calls"
	lspDocumentSymbols  lspOperation = "document_symbols"
	lspWorkspaceSymbols lspOperation = "workspace_symbols"
	lspDiagnostics      lspOperation = "diagnostics"
)

func (l lspInput) normalize() (lspQuery, error) {
	position, hasPosition, err := l.position()
	if err != nil {
		return lspQuery{}, err
	}
	query := lspQuery{
		operation:   l.Operation,
		path:        l.Path,
		position:    position,
		symbolQuery: l.Query,
	}
	switch l.Operation {
	case lspDefinition, lspReferences, lspImplementation, lspHover,
		lspIncomingCalls, lspOutgoingCalls:
		if strings.TrimSpace(l.Path) == "" {
			return lspQuery{}, fmt.Errorf("lsp %s: path is required", l.Operation)
		}
		if !hasPosition {
			return lspQuery{}, fmt.Errorf("lsp %s: line and character are required", l.Operation)
		}
		if strings.TrimSpace(l.Query) != "" {
			return lspQuery{}, fmt.Errorf("lsp %s: query is not used for position operations", l.Operation)
		}
	case lspDocumentSymbols, lspDiagnostics:
		if strings.TrimSpace(l.Path) == "" {
			return lspQuery{}, fmt.Errorf("lsp %s: path is required", l.Operation)
		}
		if hasPosition || strings.TrimSpace(l.Query) != "" {
			return lspQuery{}, fmt.Errorf("lsp %s: only path is accepted", l.Operation)
		}
	case lspWorkspaceSymbols:
		if strings.TrimSpace(l.Query) == "" {
			return lspQuery{}, errors.New("lsp workspace_symbols: query is required")
		}
		if strings.TrimSpace(l.Path) != "" || hasPosition {
			return lspQuery{}, errors.New("lsp workspace_symbols: only query is accepted")
		}
	default:
		return lspQuery{}, fmt.Errorf("lsp: unknown operation %q", l.Operation)
	}
	return query, nil
}

func (l lspInput) position() (lspPosition, bool, error) {
	switch {
	case l.Line == nil && l.Character == nil:
		return lspPosition{}, false, nil
	case l.Line == nil || l.Character == nil:
		return lspPosition{}, false, errors.New("lsp: line and character must be provided together")
	case *l.Line < 1 || *l.Character < 1:
		return lspPosition{}, false, errors.New("lsp: line and character must both be at least 1")
	default:
		return lspPosition{line: *l.Line, character: *l.Character}, true, nil
	}
}

const lspDesc = "Query the language server (LSP) about code at a position or across the workspace. " +
	"Use definition, references, implementation, hover, incoming_calls, or outgoing_calls with path + 1-based line + character; " +
	"document_symbols or diagnostics with path; workspace_symbols with query. " +
	"diagnostics returns the current compile errors and warnings for one file."

type lspRunner struct {
	analyzer   *codeintel.Analyzer
	defaultCWD string
}

func newQuery(ci *codeintel.Analyzer, defaultCWD string) (toolcontract.Tool, error) {
	t := &lspRunner{analyzer: ci, defaultCWD: defaultCWD}
	return toolcontract.NewFunc[lspInput, string](
		toolcontract.FuncConfig{Name: tool.LSP, Description: lspDesc},
		t.query,
	)
}

func (l *lspRunner) query(ctx context.Context, in lspInput) (string, error) {
	query, err := in.normalize()
	if err != nil {
		return "", err
	}
	root := executionctx.CWD(ctx, l.defaultCWD)
	switch query.operation {
	case lspDefinition:
		return l.analyzer.Definition(ctx, root, query.path, query.position.line, query.position.character)
	case lspReferences:
		return l.analyzer.References(ctx, root, query.path, query.position.line, query.position.character)
	case lspImplementation:
		return l.analyzer.Implementation(ctx, root, query.path, query.position.line, query.position.character)
	case lspHover:
		return l.analyzer.Hover(ctx, root, query.path, query.position.line, query.position.character)
	case lspIncomingCalls:
		return l.analyzer.IncomingCalls(ctx, root, query.path, query.position.line, query.position.character)
	case lspOutgoingCalls:
		return l.analyzer.OutgoingCalls(ctx, root, query.path, query.position.line, query.position.character)
	case lspDocumentSymbols:
		return l.analyzer.DocumentSymbols(ctx, root, query.path)
	case lspDiagnostics:
		return l.analyzer.Diagnostics(ctx, root, query.path)
	case lspWorkspaceSymbols:
		return l.analyzer.WorkspaceSymbols(ctx, root, query.symbolQuery)
	default:
		return "", fmt.Errorf("lsp: unknown operation %q", query.operation)
	}
}
