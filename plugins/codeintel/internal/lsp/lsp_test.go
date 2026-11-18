package lsp

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeModule lays down a tiny buildable Go module and returns its root. The
// LSP operations are exercised against real gopls, so the fixture must be a
// genuine module (gopls keys everything off go.mod).
func writeModule(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("go.mod", "module example.com/lsptest\n\ngo 1.21\n")
	// Line/char references below are 0-based, matching the wire.
	write("main.go", "package main\n"+ // 0
		"\n"+ // 1
		"func Greet(name string) string {\n"+ // 2: decl of Greet at char 5
		"\treturn \"hi \" + name\n"+ // 3
		"}\n"+ // 4
		"\n"+ // 5
		"func main() {\n"+ // 6
		"\t_ = Greet(\"world\")\n"+ // 7: use of Greet at char 5
		"}\n") // 8
	write("bad.go", "package main\n\nfunc bad() {\n\tundefinedSymbol()\n}\n")
	return root
}

// gopls is at minimum 3s of cold start; give the whole suite generous slack.
func newTestServers(t *testing.T) *Servers {
	t.Helper()
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed; skipping LSP integration test")
	}
	return mustNewServers(t, DefaultServers())
}

func mustNewServers(t *testing.T, specs []ServerSpec) *Servers {
	t.Helper()
	servers, err := NewServers(t.Context(), specs)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = servers.Close() })
	return servers
}

func TestServers_DefinitionAndHover(t *testing.T) {
	servers := newTestServers(t)
	root := writeModule(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Definition of Greet at its use site (main.go:7:5) → its declaration (line 2).
	locs, err := servers.Definition(ctx, root, "main.go", Position{Line: 7, Character: 5})
	if err != nil {
		t.Fatalf("Definition: %v", err)
	}
	if len(locs) == 0 {
		t.Fatal("Definition returned no locations")
	}
	if got := locs[0].Range.Start.Line; got != 2 {
		t.Errorf("Definition start line = %d, want 2", got)
	}
	if !strings.HasSuffix(uriToPath(locs[0].URI), "main.go") {
		t.Errorf("Definition uri = %q, want .../main.go", locs[0].URI)
	}

	// Hover at the same spot mentions the function.
	hov, err := servers.Hover(ctx, root, "main.go", Position{Line: 7, Character: 5})
	if err != nil {
		t.Fatalf("Hover: %v", err)
	}
	if !strings.Contains(hov, "Greet") {
		t.Errorf("Hover = %q, want it to mention Greet", hov)
	}
}

func TestServers_Diagnostics(t *testing.T) {
	servers := newTestServers(t)
	root := writeModule(t)

	// Cold gopls can take longer than one settle window to first analyze; poll.
	var diags []Diagnostic
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		d, err := servers.Diagnostics(ctx, root, "bad.go")
		cancel()
		if err != nil {
			if _, pending := errors.AsType[*DiagnosticsPendingError](err); pending {
				continue
			}
			t.Fatalf("Diagnostics: %v", err)
		}
		if len(d) > 0 {
			diags = d
			break
		}
	}
	if len(diags) == 0 {
		t.Fatal("Diagnostics returned none for a file with an undefined symbol")
	}
	if diags[0].SeverityName() != "error" {
		t.Errorf("diagnostic severity = %q, want error", diags[0].SeverityName())
	}
}
