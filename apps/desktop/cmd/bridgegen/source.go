package main

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/Tangerg/flame/apps/desktop/internal/bridge"
	"github.com/Tangerg/flame/packages/go-sdk/wiregen"
)

type contract struct {
	definitions wiregen.Definitions
	methods     []method
	events      []event
}

type method struct {
	name   string
	fqn    string
	params []parameter
	result *wiregen.Schema
}

type parameter struct {
	name  string
	shape *wiregen.Schema
}

type event struct {
	name    string
	value   string
	payload string
}

type schemaGraph struct {
	pkg         *types.Package
	definitions wiregen.Definitions
	enums       map[string][]string
}

// Source directives mark the declarations the bridge publishes.
const (
	enumDirective  = "flame:enum"
	eventDirective = "flame:event"
)

func newGraph(pkg *types.Package, files []*ast.File) (*schemaGraph, []event, error) {
	graph := &schemaGraph{pkg: pkg, definitions: make(wiregen.Definitions), enums: make(map[string][]string)}
	var events []event
	for _, file := range files {
		for _, declaration := range file.Decls {
			decl, ok := declaration.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, specification := range decl.Specs {
				switch spec := specification.(type) {
				case *ast.TypeSpec:
					if directive(spec.Doc, decl.Doc, enumDirective) != enumDirective {
						continue
					}
					values, err := enumValues(pkg, spec.Name.Name)
					if err != nil {
						return nil, nil, err
					}
					graph.enums[spec.Name.Name] = values
				case *ast.ValueSpec:
					declared, found, err := eventDeclaration(pkg, decl, spec)
					if err != nil {
						return nil, nil, err
					}
					if found {
						events = append(events, declared)
					}
				}
			}
		}
	}
	slices.SortFunc(events, func(a, b event) int { return strings.Compare(a.name, b.name) })
	return graph, events, nil
}

// enumValues lists a published string enum's typed constants in source order,
// each exactly once.
func enumValues(pkg *types.Package, name string) ([]string, error) {
	object := pkg.Scope().Lookup(name)
	if object == nil || object.Type().Underlying() != types.Typ[types.String] {
		return nil, fmt.Errorf("enum %s is not a named string", name)
	}
	var constants []*types.Const
	for _, member := range pkg.Scope().Names() {
		value, ok := pkg.Scope().Lookup(member).(*types.Const)
		if ok && types.Identical(value.Type(), object.Type()) {
			constants = append(constants, value)
		}
	}
	if len(constants) == 0 {
		return nil, fmt.Errorf("enum %s has no typed constants", name)
	}
	slices.SortFunc(constants, func(a, b *types.Const) int { return int(a.Pos() - b.Pos()) })
	var values []string
	for _, value := range constants {
		text := constant.StringVal(value.Val())
		if slices.Contains(values, text) {
			return nil, fmt.Errorf("enum %s repeats %q", name, text)
		}
		values = append(values, text)
	}
	return values, nil
}

// eventDeclaration reads one published event: a single nonempty string
// constant annotated with its payload type.
func eventDeclaration(pkg *types.Package, decl *ast.GenDecl, spec *ast.ValueSpec) (event, bool, error) {
	annotation := directive(spec.Doc, decl.Doc, eventDirective)
	if annotation == "" {
		return event{}, false, nil
	}
	parts := strings.Fields(annotation)
	if decl.Tok != token.CONST || len(spec.Names) != 1 || len(parts) != 2 {
		return event{}, false, fmt.Errorf("event requires one constant and one payload type")
	}
	name := spec.Names[0].Name
	value, ok := pkg.Scope().Lookup(name).(*types.Const)
	if !ok || value.Val().Kind() != constant.String || constant.StringVal(value.Val()) == "" {
		return event{}, false, fmt.Errorf("event %s has no nonempty string name", name)
	}
	return event{name: name, value: constant.StringVal(value.Val()), payload: parts[1]}, true, nil
}

func directive(local, group *ast.CommentGroup, prefix string) string {
	for _, comments := range []*ast.CommentGroup{local, group} {
		if comments == nil {
			continue
		}
		for _, comment := range comments.List {
			text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
			if text == prefix || strings.HasPrefix(text, prefix+" ") {
				return text
			}
		}
	}
	return ""
}

func readMethods(directory, receiver string, graph *schemaGraph) ([]method, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	var methods []method
	seen := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(directory, entry.Name()), nil, 0)
		if err != nil {
			return nil, err
		}
		declared, err := fileMethods(file, receiver, graph, seen)
		if err != nil {
			return nil, err
		}
		methods = append(methods, declared...)
	}
	if len(methods) == 0 {
		return nil, fmt.Errorf("receiver %s exposes no methods", receiver)
	}
	slices.SortFunc(methods, func(a, b method) int { return strings.Compare(a.name, b.name) })
	return methods, nil
}

// fileMethods reads the receiver's exported methods from one file; the Wails
// startup hook is lifecycle, not bridge surface.
func fileMethods(file *ast.File, receiver string, graph *schemaGraph, seen map[string]bool) ([]method, error) {
	imports, err := fileImports(file)
	if err != nil {
		return nil, err
	}
	var methods []method
	for _, declaration := range file.Decls {
		decl, ok := declaration.(*ast.FuncDecl)
		if !ok || !exportedMethodOf(decl, receiver) {
			continue
		}
		if decl.Name.Name == "ServiceStartup" {
			if !startupHook(decl.Type, imports) {
				return nil, fmt.Errorf("%s.ServiceStartup does not match the Wails lifecycle hook", receiver)
			}
			continue
		}
		if seen[decl.Name.Name] {
			return nil, fmt.Errorf("service repeats method %s", decl.Name.Name)
		}
		seen[decl.Name.Name] = true
		method, err := graph.method(decl, file.Name.Name+"."+receiver+"."+decl.Name.Name, imports)
		if err != nil {
			return nil, err
		}
		methods = append(methods, method)
	}
	return methods, nil
}

// fileImports maps each local import name to its path.
func fileImports(file *ast.File) (map[string]string, error) {
	imports := make(map[string]string)
	for _, imported := range file.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			return nil, err
		}
		name := filepath.Base(path)
		if imported.Name != nil {
			name = imported.Name.Name
		}
		imports[name] = path
	}
	return imports, nil
}

func exportedMethodOf(decl *ast.FuncDecl, receiver string) bool {
	if decl.Recv == nil || len(decl.Recv.List) != 1 || !decl.Name.IsExported() {
		return false
	}
	owner := decl.Recv.List[0].Type
	if pointer, ok := owner.(*ast.StarExpr); ok {
		owner = pointer.X
	}
	name, ok := owner.(*ast.Ident)
	return ok && name.Name == receiver
}

// method translates one service method. Every parameter crosses the bridge by
// name, and a trailing error is the transport failure, not a business result.
func (g *schemaGraph) method(decl *ast.FuncDecl, fqn string, imports map[string]string) (method, error) {
	result := method{name: decl.Name.Name, fqn: fqn}
	if decl.Type.TypeParams != nil {
		return method{}, fmt.Errorf("method %s cannot have type parameters", fqn)
	}
	for _, field := range decl.Type.Params.List {
		shape, err := g.expression(field.Type, imports)
		if err != nil {
			return method{}, fmt.Errorf("%s parameter: %w", fqn, err)
		}
		if len(field.Names) == 0 {
			return method{}, fmt.Errorf("%s has an unnamed parameter", fqn)
		}
		for _, name := range field.Names {
			if name.Name == "_" {
				return method{}, fmt.Errorf("%s has a discarded wire parameter", fqn)
			}
			result.params = append(result.params, parameter{name: name.Name, shape: shape})
		}
	}
	var results []ast.Expr
	if decl.Type.Results != nil {
		for _, field := range decl.Type.Results.List {
			for range max(1, len(field.Names)) {
				results = append(results, field.Type)
			}
		}
	}
	if len(results) > 0 && isIdentifier(results[len(results)-1], "error") {
		results = results[:len(results)-1]
	}
	switch len(results) {
	case 0:
		return result, nil
	case 1:
		shape, err := g.expression(results[0], imports)
		if err != nil {
			return method{}, fmt.Errorf("%s result: %w", fqn, err)
		}
		result.result = shape
		return result, nil
	default:
		return method{}, fmt.Errorf("%s has multiple business results", fqn)
	}
}

func startupHook(signature *ast.FuncType, imports map[string]string) bool {
	return signature.Params != nil && len(signature.Params.List) == 2 &&
		isImported(signature.Params.List[0].Type, imports, "context", "Context") &&
		isImported(signature.Params.List[1].Type, imports, "github.com/wailsapp/wails/v3/pkg/application", "ServiceOptions") &&
		signature.Results != nil && len(signature.Results.List) == 1 && isIdentifier(signature.Results.List[0].Type, "error")
}

func isIdentifier(expression ast.Expr, name string) bool {
	identifier, ok := expression.(*ast.Ident)
	return ok && identifier.Name == name
}

func isImported(expression ast.Expr, imports map[string]string, path, name string) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	qualifier, ok := selector.X.(*ast.Ident)
	return ok && imports[qualifier.Name] == path && selector.Sel.Name == name
}

func (g *schemaGraph) expression(expression ast.Expr, imports map[string]string) (*wiregen.Schema, error) {
	switch node := expression.(type) {
	case *ast.Ident:
		switch node.Name {
		case "string":
			return &wiregen.Schema{Type: wiregen.SchemaTypeString}, nil
		case "bool":
			return &wiregen.Schema{Type: wiregen.SchemaTypeBoolean}, nil
		case "int":
			return &wiregen.Schema{Type: wiregen.SchemaTypeInteger}, nil
		case "float64":
			return &wiregen.Schema{Type: wiregen.SchemaTypeNumber}, nil
		}
	case *ast.SelectorExpr:
		qualifier, ok := node.X.(*ast.Ident)
		if ok && imports[qualifier.Name] == g.pkg.Path() {
			return g.named(node.Sel.Name)
		}
	}
	return nil, fmt.Errorf("unsupported wire type %T; use a pure bridge DTO or a supported JSON primitive", expression)
}

func (g *schemaGraph) named(name string) (*wiregen.Schema, error) {
	object, ok := g.pkg.Scope().Lookup(name).(*types.TypeName)
	if !ok || !object.Exported() {
		return nil, fmt.Errorf("bridge has no exported wire type %s", name)
	}
	return g.walk(object.Type())
}

func (g *schemaGraph) walk(value types.Type) (*wiregen.Schema, error) {
	switch value := value.(type) {
	case *types.Named:
		return g.walkNamed(value)
	case *types.Basic:
		if schema, carried := basicSchema(value.Kind()); carried {
			return &wiregen.Schema{Type: schema}, nil
		}
	case *types.Struct:
		return g.walkStruct(value)
	}
	return nil, fmt.Errorf("unsupported wire type %s", value)
}

// basicSchema maps the only Go scalars the bridge carries.
func basicSchema(kind types.BasicKind) (wiregen.SchemaType, bool) {
	switch kind {
	case types.String:
		return wiregen.SchemaTypeString, true
	case types.Bool:
		return wiregen.SchemaTypeBoolean, true
	case types.Int:
		return wiregen.SchemaTypeInteger, true
	case types.Float64:
		return wiregen.SchemaTypeNumber, true
	default:
		return "", false
	}
}

// walkNamed defines a named bridge type once and refers to it everywhere.
func (g *schemaGraph) walkNamed(named *types.Named) (*wiregen.Schema, error) {
	if named.Obj().Pkg() != g.pkg || !named.Obj().Exported() {
		return nil, fmt.Errorf("wire type %s is outside the pure bridge", named)
	}
	name := named.Obj().Name()
	if _, found := g.definitions[name]; !found {
		g.definitions[name] = &wiregen.Schema{}
		definition, err := g.walk(named.Underlying())
		if err != nil {
			return nil, err
		}
		definition.Enum = slices.Clone(g.enums[name])
		g.definitions[name] = definition
	}
	return &wiregen.Schema{Ref: wiregen.RefPrefix + name}, nil
}

func (g *schemaGraph) walkStruct(value *types.Struct) (*wiregen.Schema, error) {
	node := &wiregen.Schema{Type: wiregen.SchemaTypeObject, Properties: make(map[string]any)}
	for index := range value.NumFields() {
		field := value.Field(index)
		if !field.Exported() {
			continue
		}
		if field.Embedded() {
			return nil, fmt.Errorf("embedded wire field %s is unsupported", field.Name())
		}
		name, optional, err := jsonField(value.Tag(index))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", field.Name(), err)
		}
		if name == "-" {
			continue
		}
		if _, duplicate := node.Properties[name]; duplicate {
			return nil, fmt.Errorf("duplicate JSON field %s", name)
		}
		shape, err := g.walk(field.Type())
		if err != nil {
			return nil, fmt.Errorf("%s: %w", field.Name(), err)
		}
		node.Properties[name] = shape
		if !optional {
			node.Required = append(node.Required, name)
		}
	}
	return node, nil
}

func jsonField(tag string) (string, bool, error) {
	parts := strings.Split(reflect.StructTag(tag).Get("json"), ",")
	if parts[0] == "" {
		return "", false, fmt.Errorf("wire field requires an explicit JSON name")
	}
	optional := false
	for _, option := range parts[1:] {
		if option != "omitempty" {
			return "", false, fmt.Errorf("unsupported JSON option %q", option)
		}
		optional = true
	}
	return parts[0], optional, nil
}

func (g *schemaGraph) refine(rules []bridge.Rule) error {
	for _, rule := range rules {
		node, found := g.definitions[rule.Type]
		if !found {
			return fmt.Errorf("rule references unexposed type %s", rule.Type)
		}
		if rule.Field != "" {
			object := g.pkg.Scope().Lookup(rule.Type)
			shape, ok := object.Type().Underlying().(*types.Struct)
			if !ok {
				return fmt.Errorf("rule references field on non-object %s", rule.Type)
			}
			matched := false
			for index := range shape.NumFields() {
				if shape.Field(index).Name() != rule.Field {
					continue
				}
				name, _, err := jsonField(shape.Tag(index))
				if err != nil {
					return err
				}
				node, matched = node.Properties[name].(*wiregen.Schema)
				break
			}
			if !matched {
				return fmt.Errorf("rule references absent wire field %s.%s", rule.Type, rule.Field)
			}
		}
		switch rule.Kind {
		case bridge.Nonempty:
			if node.Type != wiregen.SchemaTypeString {
				return fmt.Errorf("nonempty rule requires a string")
			}
			node.MinLength = new(1)
		case bridge.Nonnegative:
			if node.Type != wiregen.SchemaTypeNumber {
				return fmt.Errorf("nonnegative rule requires a number")
			}
			node.Minimum = new(int64(0))
		case bridge.URL:
			if node.Type != wiregen.SchemaTypeString {
				return fmt.Errorf("URL rule requires a string")
			}
			node.Format = "url"
		default:
			return fmt.Errorf("unsupported wire rule %q", rule.Kind)
		}
	}
	return nil
}
