// apiconsumers reports actual Go product callsites for the API coverage gate.
package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		fail(fmt.Errorf("usage: apiconsumers repository-root"))
	}
	root, err := filepath.Abs(os.Args[1])
	if err != nil {
		fail(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(collect(root)); err != nil {
		fail(err)
	}
}

func collect(root string) map[string][]string {
	files := token.NewFileSet()
	parse := func(directory string) []*ast.File {
		entries, err := os.ReadDir(directory)
		if err != nil {
			fail(err)
		}
		var parsed []*ast.File
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			file, err := parser.ParseFile(files, filepath.Join(directory, entry.Name()), nil, 0)
			if err != nil {
				fail(err)
			}
			parsed = append(parsed, file)
		}
		return parsed
	}
	names := map[string]string{}
	for _, file := range parse(filepath.Join(root, "runtime/internal/delivery")) {
		ast.Inspect(file, func(node ast.Node) bool {
			spec, ok := node.(*ast.ValueSpec)
			if !ok || len(spec.Names) != 1 || len(spec.Values) != 1 {
				return true
			}
			kind, ok := spec.Type.(*ast.Ident)
			if !ok || kind.Name != "Name" {
				return true
			}
			value, ok := spec.Values[0].(*ast.BasicLit)
			if !ok || value.Kind != token.STRING {
				return true
			}
			name, err := strconv.Unquote(value.Value)
			if err != nil {
				fail(err)
			}
			names[spec.Names[0].Name] = name
			return true
		})
	}
	methods := map[string]string{}
	for _, file := range parse(filepath.Join(root, "runtime")) {
		for _, declaration := range file.Decls {
			method, ok := declaration.(*ast.FuncDecl)
			if !ok || method.Recv == nil || method.Body == nil {
				continue
			}
			receiver, ok := method.Recv.List[0].Type.(*ast.StarExpr)
			if !ok {
				continue
			}
			kind, ok := receiver.X.(*ast.Ident)
			if !ok || kind.Name != "binding" {
				continue
			}
			ast.Inspect(method.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				for _, argument := range call.Args {
					selector, ok := argument.(*ast.SelectorExpr)
					if !ok {
						continue
					}
					owner, ok := selector.X.(*ast.Ident)
					if !ok || owner.Name != "delivery" {
						continue
					}
					if name, exists := names[selector.Sel.Name]; exists {
						methods[method.Name.Name] = name
					}
				}
				return true
			})
		}
	}
	cli := parse(filepath.Join(root, "cli/internal/adapter/runtimebinding"))
	interfaces := map[string]map[string]bool{}
	ports := map[string]map[string]bool{}
	for _, file := range cli {
		ast.Inspect(file, func(node ast.Node) bool {
			spec, ok := node.(*ast.TypeSpec)
			if !ok {
				return true
			}
			if kind, ok := spec.Type.(*ast.InterfaceType); ok {
				members := map[string]bool{}
				for _, field := range kind.Methods.List {
					for _, name := range field.Names {
						if _, ok := field.Type.(*ast.FuncType); ok {
							members[name.Name] = true
						}
					}
				}
				interfaces[spec.Name.Name] = members
			}
			return true
		})
	}
	for _, file := range cli {
		ast.Inspect(file, func(node ast.Node) bool {
			spec, ok := node.(*ast.TypeSpec)
			if !ok || spec.Name.Name != "Connection" {
				return true
			}
			kind, ok := spec.Type.(*ast.StructType)
			if !ok {
				return true
			}
			for _, field := range kind.Fields.List {
				typeName, ok := field.Type.(*ast.Ident)
				if !ok {
					continue
				}
				for _, name := range field.Names {
					if members, ok := interfaces[typeName.Name]; ok {
						ports[name.Name] = members
					}
				}
			}
			return true
		})
	}
	consumers := map[string][]string{}
	for _, file := range cli {
		for _, declaration := range file.Decls {
			method, ok := declaration.(*ast.FuncDecl)
			if !ok || method.Recv == nil || method.Body == nil {
				continue
			}
			receiver, ok := method.Recv.List[0].Type.(*ast.StarExpr)
			if !ok {
				continue
			}
			kind, ok := receiver.X.(*ast.Ident)
			if !ok || kind.Name != "Connection" {
				continue
			}
			receiverName := method.Recv.List[0].Names[0].Name
			ast.Inspect(method.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				operation, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				port, ok := operation.X.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				owner, ok := port.X.(*ast.Ident)
				if !ok || owner.Name != receiverName {
					return true
				}
				if !ports[port.Sel.Name][operation.Sel.Name] {
					return true
				}
				name, ok := methods[operation.Sel.Name]
				if !ok {
					return true
				}
				position := files.Position(call.Pos())
				path, err := filepath.Rel(root, position.Filename)
				if err != nil {
					fail(err)
				}
				consumers[name] = append(consumers[name], fmt.Sprintf("%s:%d", filepath.ToSlash(path), position.Line))
				return true
			})
		}
	}
	for _, entry := range []string{"runtime/internal/plugins/schedules", "runtime/internal/plugins/memory"} {
		directory := filepath.Join(root, entry)
		if _, err := os.Stat(directory); os.IsNotExist(err) {
			continue
		}
		sources := parse(directory)
		receivers := map[string]map[string]bool{}
		for _, file := range sources {
			imports := map[string]bool{}
			for _, value := range file.Imports {
				path, err := strconv.Unquote(value.Path.Value)
				if err != nil {
					fail(err)
				}
				if path != "github.com/Tangerg/flame/runtime" {
					continue
				}
				alias := "runtime"
				if value.Name != nil {
					alias = value.Name.Name
				}
				imports[alias] = true
			}
			ast.Inspect(file, func(node ast.Node) bool {
				spec, ok := node.(*ast.TypeSpec)
				if !ok {
					return true
				}
				value, ok := spec.Type.(*ast.StructType)
				if !ok {
					return true
				}
				fields := map[string]bool{}
				for _, field := range value.Fields.List {
					pointer, ok := field.Type.(*ast.StarExpr)
					if !ok {
						continue
					}
					name, ok := pointer.X.(*ast.SelectorExpr)
					if !ok {
						continue
					}
					owner, ok := name.X.(*ast.Ident)
					if !ok || !imports[owner.Name] || (name.Sel.Name != "Client" && name.Sel.Name != "Runtime") {
						continue
					}
					for _, fieldName := range field.Names {
						fields[fieldName.Name] = true
					}
				}
				receivers[spec.Name.Name] = fields
				return true
			})
		}
		for _, file := range sources {
			for _, declaration := range file.Decls {
				method, ok := declaration.(*ast.FuncDecl)
				if !ok || method.Recv == nil || method.Body == nil {
					continue
				}
				receiver := method.Recv.List[0]
				kind := receiver.Type
				if pointer, ok := kind.(*ast.StarExpr); ok {
					kind = pointer.X
				}
				owner, ok := kind.(*ast.Ident)
				if !ok {
					continue
				}
				fields := receivers[owner.Name]
				if len(fields) == 0 {
					continue
				}
				ast.Inspect(method.Body, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok {
						return true
					}
					operation, ok := call.Fun.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					field, ok := operation.X.(*ast.SelectorExpr)
					if !ok || !fields[field.Sel.Name] {
						return true
					}
					target, ok := field.X.(*ast.Ident)
					if !ok || target.Name != receiver.Names[0].Name {
						return true
					}
					name, ok := methods[operation.Sel.Name]
					if !ok {
						return true
					}
					position := files.Position(call.Pos())
					path, err := filepath.Rel(root, position.Filename)
					if err != nil {
						fail(err)
					}
					consumers[name] = append(consumers[name], fmt.Sprintf("%s:%d", filepath.ToSlash(path), position.Line))
					return true
				})
			}
		}
	}

	return consumers
}

func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
