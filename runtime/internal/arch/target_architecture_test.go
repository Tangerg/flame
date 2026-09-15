package arch

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const runtimeModulePath = "github.com/Tangerg/flame/runtime"

// TestTargetDependencyRuleRejectsOutwardFixture proves the production DAG gate
// has a failing counterexample. The fixture is parsed, not compiled, so it can
// remain an intentionally invalid Delivery-to-Adapter edge in the repository.
func TestTargetDependencyRuleRejectsOutwardFixture(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "internal", "arch", "testdata", "architecture", "delivery_imports_adapter.go")
	violations, err := dependencyViolationsInFile(path, ringDelivery)
	if err != nil {
		t.Fatalf("check invalid dependency fixture: %v", err)
	}
	if len(violations) != 1 || violations[0].toRing != ringAdapter {
		t.Fatal("invalid Delivery-to-Adapter fixture was accepted by the target dependency rule")
	}
}

type dependencyViolation struct {
	importPath string
	toRing     string
}

func dependencyViolationsInFile(path, fromRing string) ([]dependencyViolation, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	var violations []dependencyViolation
	for _, imported := range file.Imports {
		importPath := strings.Trim(imported.Path.Value, `"`)
		relativeImport, internal := strings.CutPrefix(importPath, runtimeModulePath+"/")
		if !internal {
			continue
		}
		toRing := layerOf(relativeImport)
		if toRing != "" && forbidden(fromRing, toRing) {
			violations = append(violations, dependencyViolation{importPath: relativeImport, toRing: toRing})
		}
	}
	return violations, nil
}

// TestTargetHasNoCompatibilityPackages prevents breaking migrations from
// accumulating a second package graph behind legacy/compat/versioned paths.
// Wire version fields remain valid; this rule is only about source directories.
func TestTargetHasNoCompatibilityPackages(t *testing.T) {
	root := moduleRoot(t)
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		name := strings.ToLower(entry.Name())
		if name == "legacy" || name == "compat" || isVersionDirectory(name) {
			relativePath, _ := filepath.Rel(root, path)
			t.Errorf("compatibility package is forbidden during the breaking migration: %s", relativePath)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan compatibility packages: %v", err)
	}
}

func TestDeliveryDependencyRuleAllowsIndependentMechanisms(t *testing.T) {
	for _, test := range []struct {
		name           string
		dependency     string
		wantViolations int
	}{
		{name: "inward", dependency: "internal/application/agent/runs"},
		{name: "adapter", dependency: "internal/adapter/agentexec", wantViolations: 1},
		{name: "bootstrap", dependency: "internal/bootstrap", wantViolations: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "internal", "delivery", "newmechanism")
			if err := os.MkdirAll(directory, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, "mechanism.go")
			source := "package newmechanism\nimport _ \"" + runtimeModulePath + "/" + test.dependency + "\"\n"
			if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			violations, err := dependencyViolationsInFile(path, layerOf("internal/delivery/newmechanism/mechanism.go"))
			if err != nil {
				t.Fatal(err)
			}
			if len(violations) != test.wantViolations {
				t.Fatalf("dependency violations = %v, want %d", violations, test.wantViolations)
			}
		})
	}
}

func isVersionDirectory(name string) bool {
	if len(name) < 2 || name[0] != 'v' {
		return false
	}
	for _, character := range name[1:] {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
