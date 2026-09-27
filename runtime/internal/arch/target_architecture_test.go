package arch

import (
	"fmt"
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
	if fromRing == ringUnknown {
		return nil, fmt.Errorf("unclassified production package: %s", path)
	}
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	var violations []dependencyViolation
	for _, imported := range file.Imports {
		importPath := strings.Trim(imported.Path.Value, `"`)
		relativeImport, internal := strings.CutPrefix(importPath, runtimeModulePath+"/")
		if importPath == runtimeModulePath {
			relativeImport, internal = ".", true
		}
		if !internal {
			continue
		}
		toRing := layerOf(relativeImport)
		if forbidden(fromRing, toRing) {
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
		{name: "adapter", dependency: "internal/adapter/run/execution", wantViolations: 1},
		{name: "bootstrap", dependency: "internal/bootstrap", wantViolations: 1},
		{name: "unknown target", dependency: "internal/newfeature", wantViolations: 1},
		{name: "public protocol", dependency: "protocol"},
		{name: "pure value", dependency: "internal/identity"},
		{name: "technical mechanism", dependency: "internal/keylock"},
		{name: "generator", dependency: "internal/contractcatalog", wantViolations: 1},
		{name: "test support", dependency: "internal/testsupport", wantViolations: 1},
		{name: "fixture inside a ring", dependency: "internal/domain/testdata/fixture", wantViolations: 1},
		{name: "deployment", dependency: "localruntime", wantViolations: 1},
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

func TestDependencyRuleRejectsUnclassifiedProductionSources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "feature.go")
	if err := os.WriteFile(path, []byte("package newfeature\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := dependencyViolationsInFile(path, layerOf("internal/newfeature")); err == nil {
		t.Fatal("production package without imports escaped classification")
	}
}

func TestDependencyRuleClassifiesSharedAndPublicBoundaries(t *testing.T) {
	for _, test := range []struct {
		from, to string
		allowed  bool
	}{
		{from: ".", to: "internal/bootstrap", allowed: true},
		{from: "internal/domain/newaggregate", to: "internal/identity", allowed: true},
		{from: "internal/domain/newaggregate", to: "protocol"},
		{from: "internal/domain/newaggregate", to: "internal/completion"},
		{from: "internal/application/agent/newusecase", to: "internal/completion", allowed: true},
		{from: "internal/identity", to: "internal/domain/session"},
		{from: "internal/completion", to: "internal/application/agent/runs"},
		{from: "protocol", to: "internal/contractshape", allowed: true},
		{from: "protocol", to: "internal/application/agent/runs"},
		{from: "protocol", to: "internal/capture"},
		{from: "localruntime", to: "internal/bootstrap"},
		{from: "cmd/contractgen", to: "internal/contractcatalog", allowed: true},
		{from: "internal/adapter/newtranslation", to: "internal/contractcatalog"},
		{from: ".", to: "internal/testsupport"},
		{from: "internal/testsupport", to: "internal/application/agent/runs", allowed: true},
	} {
		t.Run(test.from+" -> "+test.to, func(t *testing.T) {
			from, to := layerOf(test.from), layerOf(test.to)
			if from == ringUnknown || to == ringUnknown {
				t.Fatalf("intended boundary is unclassified: %s -> %s", from, to)
			}
			if allowed := !forbidden(from, to); allowed != test.allowed {
				t.Fatalf("allowed = %v, want %v", allowed, test.allowed)
			}
		})
	}
}

func TestDependencyRuleRejectsImportingTheModuleRootFromDelivery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.go")
	source := "package delivery\nimport _ \"" + runtimeModulePath + "\"\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	violations, err := dependencyViolationsInFile(path, ringDelivery)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 1 || violations[0].toRing != ringComposition {
		t.Fatalf("module-root binding escaped the composition rule: %+v", violations)
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
