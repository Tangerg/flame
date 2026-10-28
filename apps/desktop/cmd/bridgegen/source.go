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
	"github.com/Tangerg/flame/qimu/contractgen"
)

type contract struct {
	definitions contractgen.Definitions
	methods     []method
	events      []event
}

type method struct {
	name   string
	fqn    string
	params []parameter
	result *contractgen.Schema
}

type parameter struct {
	name  string
	shape *contractgen.Schema
}

type event struct {
	name    string
	value   string
	payload string
}

type schemaGraph struct {
	pkg         *types.Package
	definitions contractgen.Definitions
	enums       map[string][]string
}

func newGraph(pkg *types.Package, files []*ast.File) (*schemaGraph, []event, error) {
	graph := &schemaGraph{pkg: pkg, definitions: make(contractgen.Definitions), enums: make(map[string][]string)}
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
					if directive(spec.Doc, decl.Doc, "flame:enum") != "flame:enum" {
						continue
					}
					object := pkg.Scope().Lookup(spec.Name.Name)
					if object == nil || object.Type().Underlying() != types.Typ[types.String] {
						return nil, nil, fmt.Errorf("enum %s is not a named string", spec.Name.Name)
					}
					var values []*types.Const
					for _, name := range pkg.Scope().Names() {
						value, ok := pkg.Scope().Lookup(name).(*types.Const)
						if ok && types.Identical(value.Type(), object.Type()) {
							values = append(values, value)
						}
					}
					slices.SortFunc(values, func(a, b *types.Const) int { return int(a.Pos() - b.Pos()) })
					for _, value := range values {
						text := constant.StringVal(value.Val())
						if slices.Contains(graph.enums[spec.Name.Name], text) {
							return nil, nil, fmt.Errorf("enum %s repeats %q", spec.Name.Name, text)
						}
						graph.enums[spec.Name.Name] = append(graph.enums[spec.Name.Name], text)
					}
					if len(values) == 0 {
						return nil, nil, fmt.Errorf("enum %s has no typed constants", spec.Name.Name)
					}
				case *ast.ValueSpec:
					annotation := directive(spec.Doc, decl.Doc, "flame:event")
					if annotation == "" {
						continue
					}
					parts := strings.Fields(annotation)
					if decl.Tok != token.CONST || len(spec.Names) != 1 || len(parts) != 2 {
						return nil, nil, fmt.Errorf("event requires one constant and one payload type")
					}
					name := spec.Names[0].Name
					value, ok := pkg.Scope().Lookup(name).(*types.Const)
					if !ok || value.Val().Kind() != constant.String || constant.StringVal(value.Val()) == "" {
						return nil, nil, fmt.Errorf("event %s has no nonempty string name", name)
					}
					events = append(events, event{name: name, value: constant.StringVal(value.Val()), payload: parts[1]})
				}
			}
		}
	}
	slices.SortFunc(events, func(a, b event) int { return strings.Compare(a.name, b.name) })
	return graph, events, nil
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
		for _, declaration := range file.Decls {
			decl, ok := declaration.(*ast.FuncDecl)
			if !ok || decl.Recv == nil || len(decl.Recv.List) != 1 || !decl.Name.IsExported() {
				continue
			}
			owner := decl.Recv.List[0].Type
			if pointer, ok := owner.(*ast.StarExpr); ok {
				owner = pointer.X
			}
			name, ok := owner.(*ast.Ident)
			if !ok || name.Name != receiver {
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
			method := method{name: decl.Name.Name, fqn: file.Name.Name + "." + receiver + "." + decl.Name.Name}
			if decl.Type.TypeParams != nil {
				return nil, fmt.Errorf("method %s cannot have type parameters", method.fqn)
			}
			for _, field := range decl.Type.Params.List {
				shape, err := graph.expression(field.Type, imports)
				if err != nil {
					return nil, fmt.Errorf("%s parameter: %w", method.fqn, err)
				}
				if len(field.Names) == 0 {
					return nil, fmt.Errorf("%s has an unnamed parameter", method.fqn)
				}
				for _, name := range field.Names {
					if name.Name == "_" {
						return nil, fmt.Errorf("%s has a discarded wire parameter", method.fqn)
					}
					method.params = append(method.params, parameter{name: name.Name, shape: shape})
				}
			}
			var results []ast.Expr
			if decl.Type.Results != nil {
				for _, result := range decl.Type.Results.List {
					for range max(1, len(result.Names)) {
						results = append(results, result.Type)
					}
				}
			}
			if len(results) > 0 && isIdentifier(results[len(results)-1], "error") {
				results = results[:len(results)-1]
			}
			if len(results) > 1 {
				return nil, fmt.Errorf("%s has multiple business results", method.fqn)
			}
			if len(results) == 1 {
				method.result, err = graph.expression(results[0], imports)
				if err != nil {
					return nil, fmt.Errorf("%s result: %w", method.fqn, err)
				}
			}
			methods = append(methods, method)
		}
	}
	if len(methods) == 0 {
		return nil, fmt.Errorf("receiver %s exposes no methods", receiver)
	}
	slices.SortFunc(methods, func(a, b method) int { return strings.Compare(a.name, b.name) })
	return methods, nil
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

func (g *schemaGraph) expression(expression ast.Expr, imports map[string]string) (*contractgen.Schema, error) {
	switch node := expression.(type) {
	case *ast.Ident:
		switch node.Name {
		case "string":
			return &contractgen.Schema{Type: contractgen.SchemaTypeString}, nil
		case "bool":
			return &contractgen.Schema{Type: contractgen.SchemaTypeBoolean}, nil
		case "float64":
			return &contractgen.Schema{Type: contractgen.SchemaTypeNumber}, nil
		}
	case *ast.SelectorExpr:
		qualifier, ok := node.X.(*ast.Ident)
		if ok && imports[qualifier.Name] == g.pkg.Path() {
			return g.named(node.Sel.Name)
		}
	}
	return nil, fmt.Errorf("unsupported wire type %T; use a pure bridge DTO or a supported JSON primitive", expression)
}

func (g *schemaGraph) named(name string) (*contractgen.Schema, error) {
	object, ok := g.pkg.Scope().Lookup(name).(*types.TypeName)
	if !ok || !object.Exported() {
		return nil, fmt.Errorf("bridge has no exported wire type %s", name)
	}
	return g.walk(object.Type())
}

func (g *schemaGraph) walk(value types.Type) (*contractgen.Schema, error) {
	if named, ok := value.(*types.Named); ok {
		if named.Obj().Pkg() != g.pkg || !named.Obj().Exported() {
			return nil, fmt.Errorf("wire type %s is outside the pure bridge", named)
		}
		name := named.Obj().Name()
		if _, found := g.definitions[name]; !found {
			g.definitions[name] = &contractgen.Schema{}
			definition, err := g.walk(named.Underlying())
			if err != nil {
				return nil, err
			}
			definition.Enum = slices.Clone(g.enums[name])
			g.definitions[name] = definition
		}
		return &contractgen.Schema{Ref: contractgen.RefPrefix + name}, nil
	}
	switch value := value.(type) {
	case *types.Basic:
		switch value.Kind() {
		case types.String:
			return &contractgen.Schema{Type: contractgen.SchemaTypeString}, nil
		case types.Bool:
			return &contractgen.Schema{Type: contractgen.SchemaTypeBoolean}, nil
		case types.Float64:
			return &contractgen.Schema{Type: contractgen.SchemaTypeNumber}, nil
		}
	case *types.Struct:
		node := &contractgen.Schema{Type: contractgen.SchemaTypeObject, Properties: make(map[string]any)}
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
	return nil, fmt.Errorf("unsupported wire type %s", value)
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
				node, matched = node.Properties[name].(*contractgen.Schema)
				break
			}
			if !matched {
				return fmt.Errorf("rule references absent wire field %s.%s", rule.Type, rule.Field)
			}
		}
		switch rule.Kind {
		case bridge.Nonempty:
			if node.Type != contractgen.SchemaTypeString {
				return fmt.Errorf("nonempty rule requires a string")
			}
			node.MinLength = new(1)
		case bridge.Nonnegative:
			if node.Type != contractgen.SchemaTypeNumber {
				return fmt.Errorf("nonnegative rule requires a number")
			}
			node.Minimum = new(int64(0))
		case bridge.URL:
			if node.Type != contractgen.SchemaTypeString {
				return fmt.Errorf("URL rule requires a string")
			}
			node.Format = "url"
		default:
			return fmt.Errorf("unsupported wire rule %q", rule.Kind)
		}
	}
	return nil
}
