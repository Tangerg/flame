package arch

import (
	"go/ast"
	"go/types"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// loadModule type-checks the whole module, tests included, for the checks that
// have to see every implementation a question could be answered by.
func loadModule(t *testing.T) []*packages.Package {
	t.Helper()
	loaded, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps |
			packages.NeedImports | packages.NeedFiles,
		Dir:   moduleRoot(t),
		Tests: true,
	}, "./...")
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func questionPackagePath(path string) string {
	if cut := strings.Index(path, " ["); cut >= 0 {
		path = path[:cut]
	}
	return strings.TrimSuffix(strings.TrimSuffix(path, ".test"), "_test")
}

// methodSetOf renders a type's method set as package-qualified signatures.
// Loading with Tests:true type-checks a package once per variant, so *types.Named
// identity does not survive across the corpus and types.Implements silently
// answers false; comparing rendered signatures restores the question. Parameter
// names are dropped because they are not part of the contract.
func methodSetOf(typ types.Type) map[string]bool {
	qualifier := func(p *types.Package) string { return questionPackagePath(p.Path()) }
	render := func(tuple *types.Tuple) string {
		parts := make([]string, 0, tuple.Len())
		for i := range tuple.Len() {
			parts = append(parts, types.TypeString(anonymous(tuple.At(i).Type()), qualifier))
		}
		return strings.Join(parts, ",")
	}
	set := types.NewMethodSet(typ)
	out := make(map[string]bool, set.Len())
	for i := range set.Len() {
		fn := set.At(i).Obj()
		signature, ok := fn.Type().(*types.Signature)
		if !ok {
			continue
		}
		name := fn.Name()
		if !fn.Exported() && fn.Pkg() != nil {
			name = questionPackagePath(fn.Pkg().Path()) + "." + name
		}
		out[name+"("+render(signature.Params())+")("+render(signature.Results())+")"] = true
	}
	return out
}

// anonymous rebuilds a type with every signature's parameter and result names
// removed, including the ones nested inside a func-typed parameter. Names are
// not part of a contract, but types.TypeString prints them, so leaving them in
// would make two spellings of the same method set look different.
func anonymous(typ types.Type) types.Type {
	switch shape := typ.(type) {
	case *types.Signature:
		strip := func(tuple *types.Tuple) *types.Tuple {
			vars := make([]*types.Var, 0, tuple.Len())
			for i := range tuple.Len() {
				vars = append(vars, types.NewVar(0, nil, "", anonymous(tuple.At(i).Type())))
			}
			return types.NewTuple(vars...)
		}
		return types.NewSignatureType(
			nil, nil, nil, strip(shape.Params()), strip(shape.Results()), shape.Variadic(),
		)
	case *types.Pointer:
		return types.NewPointer(anonymous(shape.Elem()))
	case *types.Slice:
		return types.NewSlice(anonymous(shape.Elem()))
	case *types.Array:
		return types.NewArray(anonymous(shape.Elem()), shape.Len())
	case *types.Map:
		return types.NewMap(anonymous(shape.Key()), anonymous(shape.Elem()))
	case *types.Chan:
		return types.NewChan(shape.Dir(), anonymous(shape.Elem()))
	}
	return typ
}

// TestRuntimeQuestionsHaveAnAnswer proves that every interface this module asks
// for at runtime — through a type assertion, a type switch, or a generic
// capability lookup — is implemented by something in the build.
//
// An optional capability is matched by dynamic type assertion, so when the
// asserted method set drifts from its implementor's the build stays green and
// the question is answered "no" forever. That is how the apply_patch tool's
// declared mutation paths stopped reaching the approval gate: the capability was
// re-signed on one side only, and every call was silently classified as
// mutating no files.
func TestRuntimeQuestionsHaveAnAnswer(t *testing.T) {
	root := moduleRoot(t)
	loaded := loadModule(t)

	type candidate struct {
		label   string
		methods map[string]bool
	}
	var candidates []candidate
	for _, pkg := range loaded {
		if pkg.Types == nil {
			continue
		}
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			declared, ok := scope.Lookup(name).(*types.TypeName)
			if !ok || declared.Type() == nil || declared.IsAlias() {
				continue
			}
			if _, isInterface := declared.Type().Underlying().(*types.Interface); isInterface {
				continue
			}
			label := questionPackagePath(pkg.PkgPath) + "." + name
			candidates = append(candidates,
				candidate{label: label, methods: methodSetOf(declared.Type())},
				candidate{
					label:   "*" + label,
					methods: methodSetOf(types.NewPointer(declared.Type())),
				})
		}
	}

	answered := func(wanted map[string]bool) bool {
		for _, implementor := range candidates {
			complete := true
			for method := range wanted {
				if !implementor.methods[method] {
					complete = false
					break
				}
			}
			if complete {
				return true
			}
		}
		return false
	}

	asked := map[string]bool{}
	var unanswerable []string
	ask := func(position string, requested types.Type) {
		if requested == nil {
			return
		}
		iface, isInterface := requested.Underlying().(*types.Interface)
		if !isInterface || iface.NumMethods() == 0 {
			return
		}
		// An anonymous interface literal is written here, so it is ours by
		// construction; a named one is ours only when this module declares it.
		// Either way only our own vocabulary can drift under our feet.
		label := position + " | anonymous interface"
		if named, isNamed := requested.(*types.Named); isNamed {
			if named.Obj().Pkg() == nil {
				return
			}
			owner := questionPackagePath(named.Obj().Pkg().Path())
			if !strings.HasPrefix(owner, runtimeModule) {
				return
			}
			label = position + " | " + strings.TrimPrefix(owner, runtimeModule+"/") +
				"." + named.Obj().Name()
		}
		if asked[label] {
			return
		}
		asked[label] = true
		if !answered(methodSetOf(requested)) {
			unanswerable = append(unanswerable, label)
		}
	}

	for _, pkg := range loaded {
		if pkg.TypesInfo == nil || pkg.Fset == nil {
			continue
		}
		for _, file := range pkg.Syntax {
			if !strings.HasPrefix(pkg.Fset.Position(file.Pos()).Filename, root+"/") {
				continue
			}
			ast.Inspect(file, func(n ast.Node) bool {
				at := func(node ast.Node) string {
					return strings.TrimPrefix(pkg.Fset.Position(node.Pos()).String(), root+"/")
				}
				switch node := n.(type) {
				case *ast.TypeAssertExpr:
					if node.Type != nil {
						ask(at(node), pkg.TypesInfo.TypeOf(node.Type))
					}
				case *ast.CaseClause:
					for _, expr := range node.List {
						ask(at(expr), pkg.TypesInfo.TypeOf(expr))
					}
				case *ast.IndexExpr:
					// A generic capability lookup asks the same question with the
					// interface as its type argument.
					ask(at(node), pkg.TypesInfo.TypeOf(node.Index))
				}
				return true
			})
		}
	}

	if len(unanswerable) > 0 {
		sort.Strings(unanswerable)
		t.Fatalf("no type in the build can answer these runtime questions:\n%s",
			strings.Join(unanswerable, "\n"))
	}
}

// TestOwnersDoNotHandOutTheirContainers proves no method returns the receiver's
// own slice or map. A caller that receives one can mutate state the owner still
// believes it controls, and neither the compiler nor any test of the owner sees
// it happen. Every projection in this module copies.
func TestOwnersDoNotHandOutTheirContainers(t *testing.T) {
	root := moduleRoot(t)
	var aliased []string

	for _, pkg := range loadModule(t) {
		if pkg.TypesInfo == nil || pkg.Fset == nil || !strings.Contains(pkg.PkgPath, "/internal/") {
			continue
		}
		for _, file := range pkg.Syntax {
			filename := pkg.Fset.Position(file.Pos()).Filename
			if !strings.HasPrefix(filename, root+"/") || strings.HasSuffix(filename, "_test.go") {
				continue
			}
			for _, decl := range file.Decls {
				function, ok := decl.(*ast.FuncDecl)
				if !ok || function.Body == nil || function.Recv == nil ||
					len(function.Recv.List) == 0 {
					continue
				}
				receiver := ""
				for _, name := range function.Recv.List[0].Names {
					receiver = name.Name
				}
				if receiver == "" || receiver == "_" {
					continue
				}
				ast.Inspect(function.Body, func(n ast.Node) bool {
					returned, isReturn := n.(*ast.ReturnStmt)
					if !isReturn {
						return true
					}
					for _, result := range returned.Results {
						// Only a bare `return r.field` aliases; a clone or an append
						// into a fresh slice is a call, not a selector.
						selector, isSelector := result.(*ast.SelectorExpr)
						if !isSelector {
							continue
						}
						base, isIdent := selector.X.(*ast.Ident)
						if !isIdent || base.Name != receiver {
							continue
						}
						typ := pkg.TypesInfo.TypeOf(selector)
						if typ == nil {
							continue
						}
						switch typ.Underlying().(type) {
						case *types.Slice, *types.Map:
							aliased = append(aliased, strings.TrimPrefix(
								pkg.Fset.Position(result.Pos()).String(), root+"/")+
								" | "+function.Name.Name)
						}
					}
					return true
				})
			}
		}
	}

	if len(aliased) > 0 {
		sort.Strings(aliased)
		t.Fatalf("these methods hand a caller the receiver's own container:\n%s",
			strings.Join(slicesCompact(aliased), "\n"))
	}
}

func slicesCompact(values []string) []string {
	out := values[:0]
	var previous string
	for index, value := range values {
		if index == 0 || value != previous {
			out = append(out, value)
		}
		previous = value
	}
	return out
}

// TestRecoveredPanicsKeepTheirStack proves every contained panic in this module
// either travels on — re-raised for an outer owner — or captures the stack
// where it is absorbed.
//
// A recovered panic is the one failure whose cause exists nowhere but its own
// frame: the value alone names neither the operation nor the line. Two in
// delivery absorbed one without a stack, and a health probe discarded the value
// too, so a panicking probe answered readiness with "probe panic" and left no
// way to learn what panicked.
func TestRecoveredPanicsKeepTheirStack(t *testing.T) {
	root := moduleRoot(t)
	loaded := loadModule(t)

	// keepers name every function that captures a stack. A deferred recover may
	// hand its value to one of these: debug.Stack still sees the panicking frames
	// from anything the deferred call reaches. Re-raising does not delegate — it
	// has to be visible in the frame that recovered.
	keepers := map[string]bool{}
	// Every function's outgoing calls, so delegation can be followed past one hop.
	callGraph := map[string][]string{}
	type site struct {
		name     string
		position string
		callees  []string
		keeps    bool
	}
	var sites []site

	for _, pkg := range loaded {
		if pkg.Fset == nil || pkg.TypesInfo == nil {
			continue
		}
		for _, file := range pkg.Syntax {
			filename := pkg.Fset.Position(file.Pos()).Filename
			if !strings.HasPrefix(filename, root+"/") || strings.HasSuffix(filename, "_test.go") {
				continue
			}
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				declared, isFunc := pkg.TypesInfo.Defs[fn.Name].(*types.Func)
				if !isFunc {
					continue
				}
				self := questionPackagePath(declared.Pkg().Path()) + "." + fn.Name.Name
				recovers, keeps, stacks := 0, 0, 0
				var callees []string
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					call, isCall := n.(*ast.CallExpr)
					if !isCall {
						return true
					}
					switch callee := call.Fun.(type) {
					case *ast.Ident:
						switch callee.Name {
						case "recover":
							recovers++
						case "panic":
							keeps++
						default:
							if target, isTarget := pkg.TypesInfo.Uses[callee].(*types.Func); isTarget &&
								target.Pkg() != nil {
								callees = append(callees,
									questionPackagePath(target.Pkg().Path())+"."+target.Name())
							}
						}
					case *ast.SelectorExpr:
						if callee.Sel.Name == "Stack" {
							keeps++
							stacks++
							return true
						}
						if target, isTarget := pkg.TypesInfo.Uses[callee.Sel].(*types.Func); isTarget &&
							target.Pkg() != nil {
							callees = append(callees,
								questionPackagePath(target.Pkg().Path())+"."+target.Name())
						}
					}
					return true
				})
				callGraph[self] = callees
				if stacks > 0 {
					keepers[self] = true
				}
				if recovers > 0 {
					sites = append(sites, site{
						name: fn.Name.Name,
						position: strings.TrimPrefix(
							pkg.Fset.Position(fn.Pos()).String(), root+"/"),
						callees: callees,
						keeps:   keeps > 0,
					})
				}
			}
		}
	}

	// Delegation is transitive: a recover may hand its value to a helper that
	// hands it to the one taking the stack. Stopping at one hop would fail a
	// correct site the moment someone extracted a shared reporter, and a guard
	// that misfires is one somebody deletes.
	for grown := true; grown; {
		grown = false
		for name, callees := range callGraph {
			if keepers[name] {
				continue
			}
			for _, callee := range callees {
				if keepers[callee] {
					keepers[name] = true
					grown = true
					break
				}
			}
		}
	}

	var bare []string
	seen := map[string]bool{}
	for _, found := range sites {
		if found.keeps {
			continue
		}
		delegated := false
		for _, callee := range found.callees {
			if keepers[callee] {
				delegated = true
				break
			}
		}
		if delegated {
			continue
		}
		line := found.position + "  " + found.name
		if !seen[line] {
			seen[line] = true
			bare = append(bare, line)
		}
	}

	if len(bare) > 0 {
		sort.Strings(bare)
		t.Fatalf("these absorb a panic without re-raising it or keeping its stack:\n%s",
			strings.Join(bare, "\n"))
	}
}
