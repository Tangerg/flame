package builtin

import (
	"context"
	"strings"
	"testing"

	toolcontract "github.com/Tangerg/scope/core/tool"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset/codeintel"
)

// lspTool returns the combined `lsp` tool from a fresh Build.
func lspTool(t *testing.T, ci *codeintel.Analyzer) toolcontract.Tool {
	t.Helper()
	tools, err := BuildLSP(ci, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools {
		if tool.Definition().Name == "lsp" {
			return tool
		}
	}
	t.Fatal("lsp tool not built")
	return nil
}

func newTestAnalyzer(t *testing.T) *codeintel.Analyzer {
	t.Helper()
	analyzer, err := codeintel.New(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = analyzer.Close() })
	return analyzer
}

func TestLSPPositionPreservesPresence(t *testing.T) {
	one, zero := 1, 0

	if _, present, err := (lspInput{}).position(); err != nil || present {
		t.Fatalf("omitted position = (present=%v, err=%v), want absent", present, err)
	}
	if _, _, err := (lspInput{Line: &one}).position(); err == nil {
		t.Fatal("incomplete position was accepted")
	}
	if _, _, err := (lspInput{Line: &zero, Character: &one}).position(); err == nil {
		t.Fatal("present zero line was treated as omission")
	}
	position, present, err := (lspInput{Line: &one, Character: &one}).position()
	if err != nil || !present || position.line != 1 || position.character != 1 {
		t.Fatalf("valid position = (%+v, present=%v, err=%v)", position, present, err)
	}
}

// TestLSPToolUnsupportedFile checks the tool-layer contract: a query on a file
// type with no configured server returns a plain message (the model adapts),
// not an error that would halt the loop.
func TestLSPToolUnsupportedFile(t *testing.T) {
	ci := newTestAnalyzer(t)

	out, err := callTextTool(context.Background(), lspTool(t, ci), `{"operation":"hover","path":"notes.txt","line":1,"character":1}`)
	if err != nil {
		t.Fatalf("unsupported file should not error: %v", err)
	}
	if !strings.Contains(out, "No language server") {
		t.Errorf("output = %q, want a no-server message", out)
	}
}

// TestLSPToolValidation covers the combined tool's dispatch guards: an unknown
// operation and a missing required operand are model-facing errors, and the new
// operations (implementation, incoming/outgoing calls) are accepted + routed
// (returning the no-server message under the default servers, not an error).
func TestLSPToolValidation(t *testing.T) {
	ci := newTestAnalyzer(t)
	lsp := lspTool(t, ci)

	if _, err := callTextTool(context.Background(), lsp, `{"operation":"bogus"}`); err == nil {
		t.Error("unknown operation must error")
	}
	if _, err := callTextTool(context.Background(), lsp, `{"operation":"definition"}`); err == nil {
		t.Error("definition without path must error")
	}
	if _, err := callTextTool(context.Background(), lsp, `{"operation":"definition","path":"notes.txt"}`); err == nil {
		t.Error("position operation without line and character must error")
	}
	if _, err := callTextTool(context.Background(), lsp, `{"operation":"definition","path":"notes.txt","line":1}`); err == nil {
		t.Error("position operation with an incomplete coordinate must error")
	}
	if _, err := callTextTool(context.Background(), lsp, `{"operation":"workspace_symbols"}`); err == nil {
		t.Error("workspace_symbols without query must error")
	}
	for _, op := range []string{"implementation", "incoming_calls", "outgoing_calls"} {
		out, err := callTextTool(context.Background(), lsp, `{"operation":"`+op+`","path":"notes.txt","line":1,"character":1}`)
		if err != nil {
			t.Errorf("%s should not error on unsupported file: %v", op, err)
		}
		if !strings.Contains(out, "No language server") {
			t.Errorf("%s output = %q, want a no-server message", op, out)
		}
	}
	if out, err := callTextTool(context.Background(), lsp, `{"operation":"diagnostics","path":"notes.txt"}`); err != nil || !strings.Contains(out, "No language server") {
		t.Errorf("diagnostics = (%q, %v), want a no-server message", out, err)
	}
	if _, err := callTextTool(context.Background(), lsp, `{"operation":"diagnostics","file_path":"notes.txt"}`); err == nil {
		t.Error("obsolete file_path field must be rejected")
	}
	for _, arguments := range []string{
		`{"operation":"definition","path":"notes.txt","line":0,"character":1}`,
		`{"operation":"definition","path":"notes.txt","line":1,"character":1,"query":"unused"}`,
		`{"operation":"diagnostics","path":"notes.txt","line":0,"character":0}`,
		`{"operation":"diagnostics","path":"notes.txt","line":1}`,
		`{"operation":"document_symbols","path":"notes.txt","query":"unused"}`,
		`{"operation":"workspace_symbols","query":"Thing","path":"notes.txt"}`,
		`{"operation":"workspace_symbols","query":"Thing","line":0,"character":0}`,
	} {
		if _, err := callTextTool(context.Background(), lsp, arguments); err == nil {
			t.Errorf("lsp accepted fields ignored by the selected operation: %s", arguments)
		}
	}
}
