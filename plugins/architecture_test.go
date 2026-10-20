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
		inner := strings.Contains(location, "/backend/domain/") || strings.Contains(location, "/backend/application/")
		fixture := strings.Contains(location, "/internal/testsupport/")
		for _, imported := range file.Imports {
			dependency := strings.Trim(imported.Path.Value, `"`)
			if strings.HasPrefix(dependency, "github.com/Tangerg/flame/runtime/internal/") {
				t.Errorf("%s imports private Runtime implementation %s", path, dependency)
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
		"github.com/Tangerg/flame/runtime/plugin", "github.com/Tangerg/flame/runtime/storage",
		"github.com/Tangerg/flame/runtime/sandbox", "github.com/Tangerg/flame/runtime/transport",
	} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	for _, segment := range []string{"/backend/sqlite", "/backend/transport", "/backend/tools", "/backend/rpc"} {
		if strings.Contains(path, segment) {
			return true
		}
	}
	return false
}

func TestPluginPolicyBoundaryRejectsConcreteAdapters(t *testing.T) {
	for _, dependency := range []string{"net/http", "database/sql", "github.com/Tangerg/flame/plugins/memory/backend/sqlite", "github.com/Tangerg/flame/runtime/plugin"} {
		if !outwardDependency(dependency) {
			t.Fatalf("policy boundary admitted %s", dependency)
		}
	}
	if outwardDependency("github.com/Tangerg/flame/runtime/model") {
		t.Fatal("policy boundary refused the public immutable model choice")
	}
}
