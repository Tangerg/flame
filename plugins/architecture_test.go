package plugins_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginPoliciesStayIndependentOfInfrastructure(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "node_modules" || entry.Name() == "vendor" || entry.Name() == "dist" || entry.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		location := "/" + filepath.ToSlash(path)
		inner := strings.Contains(location, "/internal/domain/") || strings.Contains(location, "/internal/application/")
		fixture := strings.Contains(location, "/internal/testsupport/")
		for _, imported := range file.Imports {
			dependency := strings.Trim(imported.Path.Value, `"`)
			if dependency == "github.com/Tangerg/flame/runtime" || strings.HasPrefix(dependency, "github.com/Tangerg/flame/runtime/") {
				t.Errorf("%s imports Runtime host implementation %s", path, dependency)
			}
			if !fixture && strings.Contains(dependency, "/internal/testsupport") {
				t.Errorf("%s imports a test fixture in production", path)
			}
			if inner && outwardDependency(dependency) {
				t.Errorf("%s imports infrastructure %s into plugin policy", path, dependency)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func outwardDependency(path string) bool {
	for _, prefix := range []string{
		"database/sql", "net/http", "os", "github.com/Tangerg/go-sdk/mcp",
		"github.com/Tangerg/scope/agent", "github.com/Tangerg/scope/core/chatclient",
		"github.com/Tangerg/flame/packages/go-sdk/plugin", "github.com/Tangerg/flame/packages/go-sdk/storage", "github.com/Tangerg/flame/qimu/sqlite", "github.com/Tangerg/flame/packages/go-sdk/filesystem",
		"github.com/Tangerg/flame/packages/go-sdk/sandbox", "github.com/Tangerg/flame/packages/go-sdk/httpresponse",
	} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	for _, segment := range []string{"/internal/sqlite", "/internal/contract"} {
		if strings.Contains(path, segment) {
			return true
		}
	}
	return false
}

func TestPluginPolicyBoundaryRejectsConcreteAdapters(t *testing.T) {
	for _, dependency := range []string{"net/http", "database/sql", "github.com/Tangerg/flame/plugins/memory/internal/sqlite", "github.com/Tangerg/flame/packages/go-sdk/plugin"} {
		if !outwardDependency(dependency) {
			t.Fatalf("policy boundary admitted %s", dependency)
		}
	}
	if outwardDependency("github.com/Tangerg/flame/packages/go-sdk/model") {
		t.Fatal("policy boundary refused the public immutable model choice")
	}
}

// A policy leaf must state the owner it holds, not repeat the ring directory that
// classifies it. The import path already carries internal/domain or internal/application;
// a package named after that ring (knowledge.Document vs domain.Document) hides the owner.
func TestPluginPolicyPackagesAreNamedAfterTheirAggregate(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "node_modules" || entry.Name() == "vendor" || entry.Name() == "dist" || entry.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		location := "/" + filepath.ToSlash(path)
		for _, ring := range []string{"domain", "application"} {
			if !strings.Contains(location, "/internal/"+ring+"/") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.PackageClauseOnly)
			if err != nil {
				return err
			}
			if strings.TrimSuffix(file.Name.Name, "_test") == ring {
				t.Errorf("%s declares package %q after the %s ring; name it after the aggregate it owns", path, file.Name.Name, ring)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
