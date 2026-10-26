// Package arch enforces the public SDK's standalone ownership and value boundaries.
package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sdkModulePath = "github.com/Tangerg/flame/packages/go-sdk"

// moduleRoot resolves this module from its manifest; it never guesses a checkout layout.
func moduleRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		data, err := os.ReadFile(filepath.Join(directory, "go.mod"))
		if err == nil && strings.Contains(string(data), "module "+sdkModulePath+"\n") {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("SDK module root not found")
		}
		directory = parent
	}
}

// TestStandaloneImports includes tests and commands: neither may borrow a host implementation.
func TestStandaloneImports(t *testing.T) {
	root := moduleRoot(t)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == "testdata" {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range file.Imports {
			value := strings.Trim(imported.Path.Value, `"`)
			if (value == "github.com/Tangerg/flame" || strings.HasPrefix(value, "github.com/Tangerg/flame/")) && value != sdkModulePath && !strings.HasPrefix(value, sdkModulePath+"/") {
				relative, _ := filepath.Rel(root, path)
				t.Errorf("%s imports host/repository package %s; the SDK must be independently buildable", relative, value)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSharedValuesStayFreeOfExternalCapabilities(t *testing.T) {
	root := moduleRoot(t)
	values := map[string]bool{
		"extension": true, "exactint": true, "identity": true, "instant": true,
		"jsonvalue": true, "model": true, "pagination": true,
	}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		if values[filepath.ToSlash(relative)] {
			forbidExternalImports(t, path, frameworkImports)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

var frameworkImports = []string{
	"os",
	"io/fs",
	"net",
	"net/http",
	"database/sql",
	"modernc.org/sqlite",
	"github.com/go-git",
	"github.com/mark3labs",
	"github.com/sourcegraph",
	"github.com/Tangerg/scope/core/history",
	"github.com/Tangerg/scope/models/catalog",
	"gopkg.in/yaml.v3",
}

func forbidExternalImports(t *testing.T, dir string, banned []string) {
	t.Helper()
	root := moduleRoot(t)
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			ip := strings.Trim(imp.Path.Value, `"`)
			// net/url and net/netip are deterministic syntax/value parsers, not network
			// capability. All packages capable of network I/O remain excluded.
			if ip == "net/url" || ip == "net/netip" {
				continue
			}
			for _, bad := range banned {
				if ip == bad || strings.HasPrefix(ip, bad+"/") {
					rel, _ := filepath.Rel(root, path)
					t.Errorf("%s must not import %q", rel, ip)
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk %s: %v", dir, walkErr)
	}
}

func assertOnlyJSONNumberIsUsed(t *testing.T, relative, path string, imported *ast.ImportSpec) {
	t.Helper()
	qualifier := "json"
	if imported.Name != nil {
		qualifier = imported.Name.Name
	}
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", relative, err)
	}
	ast.Inspect(file, func(node ast.Node) bool {
		selector, isSelector := node.(*ast.SelectorExpr)
		if !isSelector {
			return true
		}
		receiver, isIdentifier := selector.X.(*ast.Ident)
		if !isIdentifier || receiver.Name != qualifier || selector.Sel.Name == "Number" {
			return true
		}
		t.Errorf("%s uses %s.%s; the exception covers json.Number alone",
			relative, qualifier, selector.Sel.Name)
		return true
	})
}

func TestPublicTypesUseTheirOwningDeclaration(t *testing.T) {
	root := moduleRoot(t)
	internal := root
	err := filepath.WalkDir(internal, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(internal, path)
		if err != nil {
			return err
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, spec := range general.Specs {
				typeSpec := spec.(*ast.TypeSpec)
				if !typeSpec.Assign.IsValid() {
					continue
				}
				t.Errorf("%s declares transparent alias %s; use the owning type directly or define a semantic boundary type", rel, typeSpec.Name.Name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan transparent aliases: %v", err)
	}
}

func TestSDKUsesOneJSONVocabulary(t *testing.T) {
	root := moduleRoot(t)
	exact := map[string]string{
		"jsonvalue": "names json.Number as the exact-number carrier",
	}
	found := make(map[string]bool, len(exact))
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range parsed.Imports {
			if strings.Trim(imported.Path.Value, `"`) != "encoding/json" {
				continue
			}
			relative, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			if !strings.HasSuffix(path, "_test.go") {
				pkg := filepath.Dir(relative)
				if _, allowed := exact[pkg]; !allowed {
					t.Errorf("%s imports encoding/json; SDK decodes with encoding/json/v2", relative)
					continue
				}
				found[pkg] = true
			}
			assertOnlyJSONNumberIsUsed(t, relative, path, imported)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk SDK: %v", walkErr)
	}
	for relative, reason := range exact {
		if !found[relative] {
			t.Errorf("%s no longer needs encoding/json (%s); drop it from the exception list", relative, reason)
		}
	}
}

func TestSharedCapabilitiesStayPure(t *testing.T) {
	root := moduleRoot(t)
	for _, name := range []string{
		"capture", "exactint", "jsonvalue", "extension", "identity", "model", "pagination",
	} {
		forbidExternalImports(t, filepath.Join(root, name), []string{
			"github.com/Tangerg/flame/runtime/internal/domain",
			"github.com/Tangerg/flame/runtime/internal/application",
			"github.com/Tangerg/flame/runtime/internal/adapter",
			"github.com/Tangerg/flame/runtime/internal/infra",
			"github.com/Tangerg/flame/runtime/internal/delivery",
			"github.com/Tangerg/flame/runtime/internal/bootstrap",
		})
	}
}

func TestProtocolStaysWireOnly(t *testing.T) {
	root := moduleRoot(t)
	// These packages remain outward capabilities after moving into the SDK;
	// their public paths must not escape the former private-import boundary.
	banned := []string{"github.com/Tangerg/flame/runtime"}
	for _, name := range []string{
		"client", "plugin", "sqlite", "contractgen", "cmd", "filesystem", "sandbox",
		"httpresponse", "model/catalog", "cancellation", "capture", "pagination",
	} {
		banned = append(banned, sdkModulePath+"/"+name)
	}
	for _, name := range []string{"protocol", "jsonvalue"} {
		forbidExternalImports(t, filepath.Join(root, name), append(banned, frameworkImports...))
	}
}
