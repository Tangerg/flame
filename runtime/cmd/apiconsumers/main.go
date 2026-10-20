// apiconsumers reports actual Go product callsites for the API coverage gate.
package main

import (
	json "encoding/json/v2"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func main() {
	if len(os.Args) != 2 {
		fail(fmt.Errorf("usage: apiconsumers repository-root"))
	}
	root, err := filepath.Abs(os.Args[1])
	if err != nil {
		fail(err)
	}
	methods := map[string]string{}
	for _, meta := range delivery.Contract().Metas() {
		methods[meta.GoName] = meta.Name.String()
	}
	if err := json.MarshalWrite(os.Stdout, collect(root, methods)); err != nil {
		fail(err)
	}
}

// readGoDirectory returns the directory entries in bounded batches instead of
// materializing the whole directory at once.
func readGoDirectory(directory string) []os.DirEntry {
	dir, err := os.Open(directory)
	if err != nil {
		fail(err)
	}
	defer func() { _ = dir.Close() }()
	var entries []os.DirEntry
	for {
		batch, err := dir.ReadDir(128)
		entries = append(entries, batch...)
		if err == io.EOF {
			return entries
		}
		if err != nil {
			fail(err)
		}
	}
}

func collect(root string, methods map[string]string) map[string][]string {
	files := token.NewFileSet()
	parse := func(directory string) []*ast.File {
		entries := readGoDirectory(directory)
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
	cli := parse(filepath.Join(root, "apps/cli/internal/adapter/runtimebinding"))
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
	backends, err := filepath.Glob(filepath.Join(root, "plugins", "*", "backend"))
	if err != nil {
		fail(err)
	}
	for _, directory := range backends {
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
				if path != "github.com/Tangerg/flame/runtime" && path != "github.com/Tangerg/flame/packages/runtime-client/go" {
					continue
				}
				alias := "runtime"
				if path == "github.com/Tangerg/flame/packages/runtime-client/go" {
					alias = "runtimeclient"
				}
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
