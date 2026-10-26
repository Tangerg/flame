package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestWireConstraintsStayPure(t *testing.T) {
	root := moduleRoot(t)
	for _, name := range []string{"wire_constraints.go", "wire_constraints.generated.go"} {
		assertConstraintsArePure(t, filepath.Join(root, "protocol", name))
	}
}

func assertConstraintsArePure(t *testing.T, path string) {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, spec := range file.Imports {
		imported := strings.Trim(spec.Path.Value, `"`)
		if strings.Contains(imported, "/internal/") {
			t.Errorf("wire constraints import %q; a shape constraint may only read the value it validates", imported)
		}
	}
	// ValidateWire must also be reachable without a context: a check that needs one is
	// a check that does I/O.
	full, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, decl := range full.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "ValidateWire" || fn.Recv == nil {
			continue
		}
		if fn.Type.Params != nil && len(fn.Type.Params.List) != 0 {
			t.Errorf("%s.ValidateWire takes parameters; a shape constraint reads only its own value", exprString(fn.Recv.List[0].Type))
		}
	}
}

func exprString(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return exprString(t.X) + "." + t.Sel.Name
	case *ast.StarExpr:
		return "*" + exprString(t.X)
	case *ast.IndexExpr:
		return exprString(t.X) + "[" + exprString(t.Index) + "]"
	case *ast.ArrayType:
		return "[]" + exprString(t.Elt)
	default:
		return ""
	}
}
