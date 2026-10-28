package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/flame/apps/desktop/internal/bridge"
	"github.com/Tangerg/flame/qimu/contractgen"
)

func fixtureGraph(t *testing.T) *schemaGraph {
	t.Helper()
	const source = `package bridge
//flame:enum
type Authorization string
const (
 Allowed Authorization = "allowed"
 Denied Authorization = "denied"
 Ordinary = "not-an-enum-member"
)
type Snapshot struct {
 Authorization Authorization ` + "`json:\"authorization\"`" + `
 Token string ` + "`json:\"token,omitempty\"`" + `
 Count float64 ` + "`json:\"count\"`" + `
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "bridge.go", source, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := new(types.Config).Check("example/bridge", fset, []*ast.File{file}, nil)
	if err != nil {
		t.Fatal(err)
	}
	graph, _, err := newGraph(pkg, []*ast.File{file})
	if err != nil {
		t.Fatal(err)
	}
	return graph
}

func fixtureMethods(t *testing.T, graph *schemaGraph, body string) ([]method, error) {
	t.Helper()
	directory := t.TempDir()
	source := `package shell
import (
 "example/bridge"
 "context"
 "github.com/wailsapp/wails/v3/pkg/application"
)
type DesktopHost struct{}
` + body
	if err := os.WriteFile(filepath.Join(directory, "host.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	return readMethods(directory, "DesktopHost", graph)
}

func TestActualMethodSignaturesOwnNamesParameterOrderAndResults(t *testing.T) {
	graph := fixtureGraph(t)
	methods, err := fixtureMethods(t, graph, `
func (*DesktopHost) Save(filename, content string) (bool, error) { panic("native body is not compiled") }
func (*DesktopHost) Bootstrap() (bridge.Snapshot, error) { panic("native") }
func (*DesktopHost) Reveal() error { panic("native") }
func (*DesktopHost) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error { panic("native") }
func (*DesktopHost) usePrivate(value any) {}
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(methods) != 3 || methods[0].fqn != "shell.DesktopHost.Bootstrap" || methods[1].result != nil {
		t.Fatalf("actual exposed method set or void result was lost: %+v", methods)
	}
	if methods[2].params[0].name != "filename" || methods[2].params[1].name != "content" || methods[2].result.Type != contractgen.SchemaTypeBoolean {
		t.Fatalf("parameter order or bool/error result changed: %+v", methods[2])
	}
	if !slices.Equal(graph.definitions["Authorization"].Enum, []string{"allowed", "denied"}) {
		t.Fatal("enum did not come from its exact typed constants")
	}
	snapshot := graph.definitions["Snapshot"]
	if slices.Contains(snapshot.Required, "token") || !slices.Contains(snapshot.Required, "count") {
		t.Fatal("JSON omission rules were not projected")
	}
	if err := graph.refine([]bridge.Rule{{Type: "Snapshot", Field: "Count", Kind: bridge.Nonnegative}}); err != nil {
		t.Fatal(err)
	}
	projected, err := zod(snapshot, graph.definitions)
	if err != nil || !strings.Contains(projected, "token: z.string().optional()") || !strings.Contains(projected, "count: z.number().min(0)") {
		t.Fatalf("optional/refinement projection: %s: %v", projected, err)
	}
}

func TestRenamedOrChangedActualMethodChangesTheContract(t *testing.T) {
	graph := fixtureGraph(t)
	before, err := fixtureMethods(t, graph, `func (*DesktopHost) Read() string { return "" }`)
	if err != nil {
		t.Fatal(err)
	}
	after, err := fixtureMethods(t, graph, `func (*DesktopHost) Inspect() bool { return false }`)
	if err != nil {
		t.Fatal(err)
	}
	if before[0].name == after[0].name || before[0].result.Type == after[0].result.Type || after[0].fqn != "shell.DesktopHost.Inspect" {
		t.Fatal("a stale method inventory hid the actual declaration change")
	}
}

func TestUnsupportedNativeSignaturesFailGeneration(t *testing.T) {
	for _, body := range []string{
		`func (*DesktopHost) Read() *bridge.Snapshot { return nil }`,
		`func (*DesktopHost) Read(value any) {}`,
		`func (*DesktopHost) Read(value func()) {}`,
		`func (*DesktopHost) Read(value chan string) {}`,
		`func (*DesktopHost) Read() (string, bool, error) { panic("native") }`,
		`func (*DesktopHost) Read() bridge.Absent { panic("native") }`,
		`func (*DesktopHost) ServiceStartup(value string) error { return nil }`,
	} {
		t.Run(body, func(t *testing.T) {
			if _, err := fixtureMethods(t, fixtureGraph(t), body); err == nil {
				t.Fatal("unsupported signature silently became an opaque wire contract")
			}
		})
	}
}

func TestWireRulesAndZodKeywordsMustResolve(t *testing.T) {
	graph := fixtureGraph(t)
	if _, err := graph.named("Snapshot"); err != nil {
		t.Fatal(err)
	}
	if err := graph.refine([]bridge.Rule{{Type: "Snapshot", Field: "Missing", Kind: bridge.Nonempty}}); err == nil {
		t.Fatal("a stale refinement field was ignored")
	}
	if _, err := zod(&contractgen.Schema{Type: contractgen.SchemaTypeString, Pattern: "^a"}, graph.definitions); err == nil {
		t.Fatal("an unsupported Schema keyword was ignored")
	}
}
