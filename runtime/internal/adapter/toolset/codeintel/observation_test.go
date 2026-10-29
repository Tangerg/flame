package codeintel

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sourcegraph/jsonrpc2"
)

func TestMutationReportsPendingWithoutClaimingCleanDiagnostics(t *testing.T) {
	t.Setenv("FLAME_CODEINTEL_FIXTURE", "empty")
	analyzer, root := observationAnalyzer(t, false)
	file := filepath.Join(root, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	text, err := analyzer.Diagnostics(t.Context(), root, file)
	if err == nil || !strings.Contains(err.Error(), "diagnostics pending") || strings.Contains(text, "No diagnostics") {
		t.Fatalf("unobserved diagnostics = %q, %v", text, err)
	}
	section, err := analyzer.DiagnoseMutation(t.Context(), root, file, func() error {
		return os.WriteFile(file, []byte("package changed\n"), 0o600)
	})
	if err != nil || !strings.Contains(section, "diagnostics pending for document version 2") || strings.Contains(section, "No diagnostics") {
		t.Fatalf("post-mutation diagnostics = %q, %v", section, err)
	}
}

func TestWorkspaceSymbolToolMarksPartialSearches(t *testing.T) {
	for _, fixture := range []string{"empty", "symbols"} {
		t.Run(fixture, func(t *testing.T) {
			t.Setenv("FLAME_CODEINTEL_FIXTURE", fixture)
			analyzer, root := observationAnalyzer(t, true)
			text, err := analyzer.WorkspaceSymbols(t.Context(), root, "Found")
			if fixture == "empty" {
				if err == nil || !strings.Contains(err.Error(), "typescript") || strings.Contains(text, "No symbols found") {
					t.Fatalf("partial empty = %q, %v", text, err)
				}
			} else if err != nil || !strings.Contains(text, "Found") || !strings.Contains(text, "incomplete") || !strings.Contains(text, "typescript") {
				t.Fatalf("partial symbols = %q, %v", text, err)
			}
		})
	}
}

func observationAnalyzer(t *testing.T, includeUnavailable bool) (*Analyzer, string) {
	t.Helper()
	root := t.TempDir()
	specs := []ServerSpec{{Name: "go", Command: os.Args[0], Args: []string{"-test.run=^TestCodeIntelServerFixture$"}, LanguageID: "go", Extensions: []string{".go"}, RootMarkers: []string{"main.go"}}}
	if includeUnavailable {
		specs = append(specs, ServerSpec{Name: "typescript", Command: filepath.Join(root, "absent-server"), LanguageID: "typescript", Extensions: []string{".ts"}, RootMarkers: []string{"main.go"}})
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	analyzer, err := New(t.Context(), specs)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = analyzer.Close() })
	return analyzer, root
}

type fixtureStdio struct {
	io.Reader
	io.Writer
}

func (fixtureStdio) Close() error { return nil }

type fixtureHandler struct{ symbols bool }

type fixtureSymbol struct {
	Name     string `json:"name"`
	Kind     int    `json:"kind"`
	Location struct {
		URI string `json:"uri"`
	} `json:"location"`
}

func (h fixtureHandler) Handle(ctx context.Context, conn *jsonrpc2.Conn, request *jsonrpc2.Request) {
	if request.Notif {
		return
	}
	var result any
	if request.Method == "workspace/symbol" {
		result = []fixtureSymbol{}
		if h.symbols {
			symbol := fixtureSymbol{Name: "Found", Kind: 12}
			symbol.Location.URI = "file:///main.go"
			result = []fixtureSymbol{symbol}
		}
	}
	_ = conn.Reply(ctx, request.ID, result)
}

func TestCodeIntelServerFixture(t *testing.T) {
	mode := os.Getenv("FLAME_CODEINTEL_FIXTURE")
	if mode == "" {
		return
	}
	conn := jsonrpc2.NewConn(t.Context(), jsonrpc2.NewBufferedStream(fixtureStdio{os.Stdin, os.Stdout}, jsonrpc2.VSCodeObjectCodec{}), fixtureHandler{symbols: mode == "symbols"})
	<-conn.DisconnectNotify()
}
