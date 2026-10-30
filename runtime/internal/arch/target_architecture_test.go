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

const (
	runtimeModulePath      = "github.com/Tangerg/flame/runtime"
	sdkModulePath          = "github.com/Tangerg/flame/packages/go-sdk"
	qimuModulePath         = "github.com/Tangerg/flame/qimu"
	localRuntimeModulePath = "github.com/Tangerg/flame/packages/local-runtime"
)

// repositoryLayer classifies real module import paths. An unrecognized repository
// package fails closed instead of escaping the Runtime dependency rule.
func repositoryLayer(path string) (string, bool) {
	if path == runtimeModulePath {
		return ringComposition, true
	}
	if relative, ok := strings.CutPrefix(path, runtimeModulePath+"/"); ok {
		return layerOf(relative), true
	}
	if relative, ok := strings.CutPrefix(path, sdkModulePath+"/"); ok {
		return sdkLayer(relative), true
	}
	if path == qimuModulePath {
		return ringMechanism, true
	}
	if relative, ok := strings.CutPrefix(path, qimuModulePath+"/"); ok {
		return qimuLayer(relative), true
	}
	if path == localRuntimeModulePath {
		return ringDeployment, true
	}
	if path == "github.com/Tangerg/flame" || strings.HasPrefix(path, "github.com/Tangerg/flame/") {
		return ringUnknown, true
	}
	return "", false
}

func sdkLayer(path string) string {
	switch path {
	case "exactint", "identity", "instant", "extension", "model", "hook", "httporigin":
		return ringValue
	case "pagination", "capture":
		return ringMechanism
	case "protocol":
		return ringProtocol
	case "plugin", "plugin/memory", "plugin/codeintel", "plugin/skilllearning", "plugin/modelprovider", "plugin/hookprovider", "plugin/sqlite", "filesystem", "model/catalog", "sandbox", "httpresponse", "procgroup", "client":
		return ringInfra
	case "cmd/flame-sdk":
		return ringGenerator
	default:
		return ringUnknown
	}
}

func qimuLayer(path string) string {
	switch path {
	case "jsonvalue", "portable":
		return ringValue
	case "cancellation", "control":
		return ringMechanism
	case "mcp", "sqlite", "runner", "distribution", "process", "control/host":
		return ringInfra
	case "contractgen", "cmd/qimu", "cmd/controlgen", "mcp/cmd/protocolgen":
		return ringGenerator
	default:
		return ringUnknown
	}
}

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
		toRing, internal := repositoryLayer(importPath)
		if !internal {
			continue
		}
		if forbidden(fromRing, toRing) {
			violations = append(violations, dependencyViolation{importPath: importPath, toRing: toRing})
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
		{name: "inward", dependency: runtimeModulePath + "/internal/application/agent/runs"},
		{name: "adapter", dependency: runtimeModulePath + "/internal/adapter/run/execution", wantViolations: 1},
		{name: "bootstrap", dependency: runtimeModulePath + "/internal/bootstrap", wantViolations: 1},
		{name: "unknown target", dependency: runtimeModulePath + "/internal/newfeature", wantViolations: 1},
		{name: "public protocol", dependency: sdkModulePath + "/protocol"},
		{name: "public backend transport", dependency: sdkModulePath + "/plugin", wantViolations: 1},
		{name: "qimu admission", dependency: qimuModulePath},
		{name: "qimu contract", dependency: qimuModulePath + "/portable"},
		{name: "qimu mcp driver", dependency: qimuModulePath + "/mcp", wantViolations: 1},
		{name: "qimu unknown", dependency: qimuModulePath + "/unclassified", wantViolations: 1},
		{name: "pure value", dependency: sdkModulePath + "/identity"},
		{name: "technical mechanism", dependency: sdkModulePath + "/pagination"},
		{name: "generator", dependency: runtimeModulePath + "/cmd/contractgen", wantViolations: 1},
		{name: "test support", dependency: runtimeModulePath + "/internal/testsupport", wantViolations: 1},
		{name: "fixture inside a ring", dependency: runtimeModulePath + "/internal/domain/testdata/fixture", wantViolations: 1},
		{name: "deployment", dependency: localRuntimeModulePath, wantViolations: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "internal", "delivery", "newmechanism")
			if err := os.MkdirAll(directory, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, "mechanism.go")
			source := "package newmechanism\nimport _ \"" + test.dependency + "\"\n"
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
		{from: runtimeModulePath, to: runtimeModulePath + "/internal/bootstrap", allowed: true},
		{from: sdkModulePath + "/plugin/sqlite", to: qimuModulePath + "/sqlite", allowed: true},
		{from: runtimeModulePath + "/internal/domain/newaggregate", to: qimuModulePath + "/portable", allowed: true},
		{from: runtimeModulePath + "/internal/domain/newaggregate", to: qimuModulePath},
		{from: runtimeModulePath + "/internal/application/agent/newusecase", to: qimuModulePath, allowed: true},
		{from: runtimeModulePath + "/internal/application/agent/newusecase", to: qimuModulePath + "/mcp"},
		{from: qimuModulePath + "/portable", to: qimuModulePath},
		{from: qimuModulePath + "/portable", to: qimuModulePath + "/jsonvalue", allowed: true},
		{from: qimuModulePath, to: qimuModulePath + "/mcp"},
		{from: qimuModulePath + "/mcp", to: qimuModulePath, allowed: true},
		{from: runtimeModulePath + "/internal/domain/newaggregate", to: sdkModulePath + "/model", allowed: true},
		{from: sdkModulePath + "/plugin/sqlite", to: sdkModulePath + "/extension", allowed: true},
		{from: sdkModulePath + "/extension", to: runtimeModulePath + "/internal/application/agent/runs"},
		{from: sdkModulePath + "/model", to: sdkModulePath + "/model/catalog"},
		{from: sdkModulePath + "/model/catalog", to: sdkModulePath + "/model", allowed: true},
		{from: qimuModulePath + "/sqlite", to: qimuModulePath + "/cancellation", allowed: true},
		{from: qimuModulePath + "/sqlite", to: runtimeModulePath + "/internal/application/agent/runs"},
		{from: runtimeModulePath + "/internal/domain/newaggregate", to: qimuModulePath + "/sqlite"},
		{from: runtimeModulePath + "/internal/domain/newaggregate", to: sdkModulePath + "/identity", allowed: true},
		{from: runtimeModulePath + "/internal/domain/newaggregate", to: sdkModulePath + "/protocol"},
		{from: runtimeModulePath + "/internal/domain/newaggregate", to: sdkModulePath + "/extension", allowed: true},
		{from: runtimeModulePath + "/internal/application/agent/newusecase", to: sdkModulePath + "/extension", allowed: true},
		{from: runtimeModulePath + "/internal/domain/newaggregate", to: sdkModulePath + "/plugin"},
		{from: runtimeModulePath + "/internal/application/agent/newusecase", to: sdkModulePath + "/plugin"},
		{from: sdkModulePath + "/extension", to: sdkModulePath + "/plugin"},
		{from: sdkModulePath + "/extension", to: qimuModulePath + "/jsonvalue", allowed: true},
		{from: runtimeModulePath + "/internal/domain/newaggregate", to: runtimeModulePath + "/internal/completion"},
		{from: runtimeModulePath + "/internal/application/agent/newusecase", to: runtimeModulePath + "/internal/completion", allowed: true},
		{from: sdkModulePath + "/identity", to: runtimeModulePath + "/internal/domain/session"},
		{from: runtimeModulePath + "/internal/completion", to: runtimeModulePath + "/internal/application/agent/runs"},
		{from: sdkModulePath + "/protocol", to: qimuModulePath + "/jsonvalue", allowed: true},
		{from: sdkModulePath + "/protocol", to: qimuModulePath + "/control"},
		{from: runtimeModulePath + "/internal/domain/newaggregate", to: qimuModulePath + "/control"},
		{from: runtimeModulePath + "/internal/application/agent/newusecase", to: qimuModulePath + "/control/host"},
		{from: sdkModulePath + "/protocol", to: runtimeModulePath + "/internal/application/agent/runs"},
		{from: sdkModulePath + "/protocol", to: sdkModulePath + "/capture"},
		{from: localRuntimeModulePath, to: runtimeModulePath + "/internal/bootstrap"},
		{from: runtimeModulePath + "/cmd/contractgen", to: runtimeModulePath + "/cmd/contractgen", allowed: true},
		{from: runtimeModulePath + "/internal/adapter/newtranslation", to: runtimeModulePath + "/cmd/contractgen"},
		{from: runtimeModulePath, to: runtimeModulePath + "/internal/testsupport"},
		{from: runtimeModulePath + "/internal/testsupport", to: runtimeModulePath + "/internal/application/agent/runs", allowed: true},
	} {
		t.Run(test.from+" -> "+test.to, func(t *testing.T) {
			from, _ := repositoryLayer(test.from)
			to, _ := repositoryLayer(test.to)
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
