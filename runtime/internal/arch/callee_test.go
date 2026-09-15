package arch

import (
	"go/ast"
	"go/types"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"
)

const runtimeModule = "github.com/Tangerg/flame/runtime"

// forbidCalls rejects calls to exact callees, resolved through the type checker
// rather than matched by spelling. A banned key is the callee's identity:
// "import/path.Function" for a package-level function, "import/path.Type.Method"
// for a method. Identity is what makes the rule about the object — a different
// type that happens to share a method name is not accused, and a caller cannot
// escape it by renaming its own helper or aliasing the owner.
//
// Every banned identity must resolve to something that exists. A rule naming a
// symbol the codebase no longer has protects nothing and hides that it protects
// nothing, so it fails here rather than passing forever.
func forbidCalls(t *testing.T, pattern string, banned map[string]string) {
	t.Helper()
	requireBannedCalleesExist(t, banned)
	root := moduleRoot(t)
	for _, pkg := range loadTypedPackages(t, pattern) {
		for index, file := range pkg.Syntax {
			path := pkg.CompiledGoFiles[index]
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				identity, ok := calleeIdentity(typeutil.Callee(pkg.TypesInfo, call))
				if !ok {
					return true
				}
				if reason, forbidden := banned[identity]; forbidden {
					relative, _ := filepath.Rel(root, path)
					t.Errorf("%s: calls %s; %s", relative, identity, reason)
				}
				return true
			})
		}
	}
}

func calleeIdentity(callee types.Object) (string, bool) {
	function, ok := callee.(*types.Func)
	if !ok || function.Pkg() == nil {
		return "", false
	}
	signature, ok := function.Type().(*types.Signature)
	if !ok {
		return "", false
	}
	receiver := signature.Recv()
	if receiver == nil {
		return function.Pkg().Path() + "." + function.Name(), true
	}
	named, ok := namedType(receiver.Type())
	if !ok {
		return "", false
	}
	return function.Pkg().Path() + "." + named.Obj().Name() + "." + function.Name(), true
}

func namedType(value types.Type) (*types.Named, bool) {
	if pointer, ok := value.(*types.Pointer); ok {
		value = pointer.Elem()
	}
	named, ok := value.(*types.Named)
	return named, ok
}

// requireBannedCalleesExist proves every rule still names a real callee.
func requireBannedCalleesExist(t *testing.T, banned map[string]string) {
	t.Helper()
	owners := make(map[string][]string)
	for identity := range banned {
		path, member, ok := splitCalleeIdentity(identity)
		if !ok {
			t.Fatalf("forbidden callee %q is not an import path plus a member", identity)
		}
		owners[path] = append(owners[path], member)
	}
	paths := make([]string, 0, len(owners))
	for path := range owners {
		paths = append(paths, path)
	}
	for _, pkg := range loadTypedPackages(t, paths...) {
		for _, member := range owners[pkg.PkgPath] {
			if !declaresCallee(pkg.Types.Scope(), member) {
				t.Errorf("%s no longer declares %s, so the rule naming it protects nothing", pkg.PkgPath, member)
			}
		}
		delete(owners, pkg.PkgPath)
	}
	for path := range owners {
		t.Errorf("forbidden callee package %q did not load", path)
	}
}

func declaresCallee(scope *types.Scope, member string) bool {
	typeName, method, isMethod := strings.Cut(member, ".")
	declared := scope.Lookup(typeName)
	if declared == nil {
		return false
	}
	if !isMethod {
		_, ok := declared.(*types.Func)
		return ok
	}
	owner, ok := declared.(*types.TypeName)
	if !ok {
		return false
	}
	named, ok := owner.Type().(*types.Named)
	if !ok {
		return false
	}
	found, _, _ := types.LookupFieldOrMethod(named, true, owner.Pkg(), method)
	_, isFunc := found.(*types.Func)
	return isFunc
}

// splitCalleeIdentity separates the import path from the member. A package name
// cannot contain a dot, so the first dot after the last path separator ends the
// import path.
func splitCalleeIdentity(identity string) (path, member string, ok bool) {
	boundary := strings.LastIndex(identity, "/") + 1
	dot := strings.Index(identity[boundary:], ".")
	if dot < 0 {
		return "", "", false
	}
	dot += boundary
	return identity[:dot], identity[dot+1:], true
}

func loadTypedPackages(t *testing.T, patterns ...string) []*packages.Package {
	t.Helper()
	loaded, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports,
		Dir: moduleRoot(t),
	}, patterns...)
	if err != nil {
		t.Fatalf("load %v: %v", patterns, err)
	}
	if len(loaded) == 0 {
		t.Fatalf("load %v matched no package", patterns)
	}
	for _, pkg := range loaded {
		for _, loadErr := range pkg.Errors {
			t.Fatalf("load %s: %v", pkg.PkgPath, loadErr)
		}
	}
	return loaded
}
