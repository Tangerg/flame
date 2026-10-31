// Consumer discovery records actual product callsites for the API coverage gate.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

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

// consumerScan records, per Runtime method, the CLI binding and plugin backend
// call sites that invoke it.
type consumerScan struct {
	root      string
	methods   map[string]string
	files     *token.FileSet
	consumers map[string][]string
}

func collect(root string, methods map[string]string) map[string][]string {
	scan := &consumerScan{root: root, methods: methods, files: token.NewFileSet(), consumers: map[string][]string{}}
	cli := scan.parse(filepath.Join(root, "apps/cli/internal/adapter/runtimebinding"))
	scan.connectionCalls(cli, connectionPorts(cli))
	backends, err := filepath.Glob(filepath.Join(root, "plugins", "*", "backend"))
	if err != nil {
		fail(err)
	}
	for _, directory := range backends {
		if _, err := os.Stat(directory); os.IsNotExist(err) {
			continue
		}
		scan.backendCalls(scan.parse(directory))
	}
	return scan.consumers
}

func (s *consumerScan) parse(directory string) []*ast.File {
	var parsed []*ast.File
	for _, entry := range readGoDirectory(directory) {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(s.files, filepath.Join(directory, entry.Name()), nil, 0)
		if err != nil {
			fail(err)
		}
		parsed = append(parsed, file)
	}
	return parsed
}

// record adds call as a consumer of the Runtime method named operation, if any.
func (s *consumerScan) record(operation string, call *ast.CallExpr) {
	name, ok := s.methods[operation]
	if !ok {
		return
	}
	position := s.files.Position(call.Pos())
	path, err := filepath.Rel(s.root, position.Filename)
	if err != nil {
		fail(err)
	}
	s.consumers[name] = append(s.consumers[name], fmt.Sprintf("%s:%d", filepath.ToSlash(path), position.Line))
}

// connectionPorts maps each Connection field typed by a binding interface to
// that interface's method set.
func connectionPorts(cli []*ast.File) map[string]map[string]bool {
	interfaces := map[string]map[string]bool{}
	for _, file := range cli {
		ast.Inspect(file, func(node ast.Node) bool {
			spec, ok := node.(*ast.TypeSpec)
			if !ok {
				return true
			}
			if kind, ok := spec.Type.(*ast.InterfaceType); ok {
				interfaces[spec.Name.Name] = interfaceMethods(kind)
			}
			return true
		})
	}
	ports := map[string]map[string]bool{}
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
	return ports
}

func interfaceMethods(kind *ast.InterfaceType) map[string]bool {
	members := map[string]bool{}
	for _, field := range kind.Methods.List {
		if _, ok := field.Type.(*ast.FuncType); !ok {
			continue
		}
		for _, name := range field.Names {
			members[name.Name] = true
		}
	}
	return members
}

// connectionCalls records each `receiver.port.Method(...)` call a Connection
// method makes through a binding port.
func (s *consumerScan) connectionCalls(cli []*ast.File, ports map[string]map[string]bool) {
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
			if kind, ok := receiver.X.(*ast.Ident); !ok || kind.Name != "Connection" {
				continue
			}
			receiverName := method.Recv.List[0].Names[0].Name
			s.fieldCalls(method.Body, receiverName, func(field, operation string) bool {
				return ports[field][operation]
			})
		}
	}
}

// fieldCalls records calls shaped `receiver.field.operation(...)` that accepts.
func (s *consumerScan) fieldCalls(body *ast.BlockStmt, receiver string, accepts func(field, operation string) bool) {
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		operation, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		field, ok := operation.X.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		owner, ok := field.X.(*ast.Ident)
		if !ok || owner.Name != receiver || !accepts(field.Sel.Name, operation.Sel.Name) {
			return true
		}
		s.record(operation.Sel.Name, call)
		return true
	})
}

// backendCalls records plugin backend calls through struct fields holding a
// Runtime or Runtime client.
func (s *consumerScan) backendCalls(sources []*ast.File) {
	receivers := map[string]map[string]bool{}
	for _, file := range sources {
		imports := runtimeImports(file)
		ast.Inspect(file, func(node ast.Node) bool {
			spec, ok := node.(*ast.TypeSpec)
			if !ok {
				return true
			}
			if value, ok := spec.Type.(*ast.StructType); ok {
				receivers[spec.Name.Name] = runtimeFields(value, imports)
			}
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
			s.fieldCalls(method.Body, receiver.Names[0].Name, func(field, _ string) bool { return fields[field] })
		}
	}
}

// runtimeImports returns the local names file gives the Runtime and its client.
func runtimeImports(file *ast.File) map[string]bool {
	imports := map[string]bool{}
	for _, value := range file.Imports {
		path, err := strconv.Unquote(value.Path.Value)
		if err != nil {
			fail(err)
		}
		alias := ""
		switch path {
		case "github.com/Tangerg/flame/runtime":
			alias = "runtime"
		case "github.com/Tangerg/flame/packages/go-sdk/client":
			alias = "runtimeclient"
		default:
			continue
		}
		if value.Name != nil {
			alias = value.Name.Name
		}
		imports[alias] = true
	}
	return imports
}

// runtimeFields names the struct's fields that hold a *Client or *Runtime from
// one of imports.
func runtimeFields(value *ast.StructType, imports map[string]bool) map[string]bool {
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
	return fields
}

func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
