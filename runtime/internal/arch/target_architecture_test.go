package arch

import (
	"fmt"
	"go/ast"
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
	frameModulePath        = "github.com/Tangerg/frame"
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
	if path == frameModulePath {
		return ringMechanism, true
	}
	if relative, ok := strings.CutPrefix(path, frameModulePath+"/"); ok {
		return frameLayer(relative), true
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
	case "skill", "plugin/hookprovider", "runoutcome", "goalworkflow", "planworkflow", "mcpconnector", "exactint", "identity", "instant", "extension", "model", "hook", "httporigin", "question", "builtin", "plugin/process", "plugin/contract":
		return ringValue
	case "pagination", "capture":
		return ringMechanism
	case "protocol":
		return ringProtocol
	case "plugin/goalworkflow/mcp", "plugin/planworkflow/mcp", "plugin/mcpconnector/mcp", "plugin", "plugin/memory", "plugin/codeintel", "plugin/skilllearning", "plugin/modelprovider", "plugin/hookprovider/mcp", "plugin/sqlite", "plugin/mcp", "filesystem", "model/catalog", "sandbox", "httpresponse", "procgroup", "client":
		return ringInfra
	case "cmd/flame-sdk", "wiregen":
		return ringGenerator
	default:
		return ringUnknown
	}
}

func frameLayer(path string) string {
	switch path {
	case "value", "portable":
		return ringValue
	case "browser", "process/mcp", "sqlite", "process/runner", "distribution", "process":
		return ringInfra
	case "portable/contractgen", "cmd/frame-runner":
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
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
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
		if forbidden(fromRing, toRing) && !frameValueImport(file, imported) {
			violations = append(violations, dependencyViolation{importPath: importPath, toRing: toRing})
		}
	}
	return violations, nil
}

// Frame publishes pure invocation and browser grant values beside executable
// transports. Inner rings may use these values; starting sessions, admitting
// use, and controlling lifetimes remains confined to external adapters.
func frameValueImport(file *ast.File, imported *ast.ImportSpec) bool {
	var name string
	var values map[string]bool
	switch strings.Trim(imported.Path.Value, `"`) {
	case frameModulePath + "/process/mcp":
		name = "mcp"
		values = map[string]bool{"Invocation": true, "Origin": true, "Refusal": true, "ErrInvocationConflict": true}
	case frameModulePath + "/browser":
		name = "browser"
		values = map[string]bool{"Authority": true, "Realm": true, "UseGrant": true}
	default:
		return false
	}
	if imported.Name != nil {
		name = imported.Name.Name
	}
	if name == "_" || name == "." {
		return false
	}
	used, pure := false, true
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		alias, ok := selector.X.(*ast.Ident)
		if ok && alias.Name == name {
			used = true
			pure = pure && values[selector.Sel.Name]
		}
		return true
	})
	return used && pure
}

func TestFrameMCPValueBoundaryRejectsExecutableSymbols(t *testing.T) {
	for _, test := range []struct {
		name, source string
		violations   int
	}{
		{name: "original invocation", source: "var _ mcp.Invocation"},
		{name: "owner refusal", source: "var _ mcp.Refusal"},
		{name: "conflicting identity", source: "var _ = mcp.ErrInvocationConflict"},
		{name: "client transport", source: "var _ = mcp.NewCallbackReceiver", violations: 1},
		{name: "mixed value and transport", source: "var _ mcp.Invocation\nvar _ = mcp.NewCallbackReceiver", violations: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "contract.go")
			content := "package domain\nimport mcp \"" + frameModulePath + "/process/mcp\"\n" + test.source
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			violations, err := dependencyViolationsInFile(path, ringDomain)
			if err != nil || len(violations) != test.violations {
				t.Fatalf("contract boundary = %v, %v; want %d violations", violations, err, test.violations)
			}
		})
	}
}

func TestFrameBrowserValueBoundaryRejectsControlAuthority(t *testing.T) {
	for _, test := range []struct {
		source     string
		violations int
	}{
		{source: "var _ browser.Realm"},
		{source: "var _ browser.UseGrant"},
		{source: "var _ = browser.NewBridge", violations: 1},
		{source: "var _ browser.Realm\nvar _ = browser.NewBridge", violations: 1},
	} {
		path := filepath.Join(t.TempDir(), "contract.go")
		source := "package application\nimport browser \"" + frameModulePath + "/browser\"\n" + test.source
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
		violations, err := dependencyViolationsInFile(path, ringApplication)
		if err != nil || len(violations) != test.violations {
			t.Fatalf("browser authority boundary = %v, %v; want %d violations", violations, err, test.violations)
		}
	}
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
		{name: "frame admission", dependency: frameModulePath},
		{name: "frame contract", dependency: frameModulePath + "/portable"},
		{name: "frame mcp driver", dependency: frameModulePath + "/process/mcp", wantViolations: 1},
		{name: "frame unknown", dependency: frameModulePath + "/unclassified", wantViolations: 1},
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
		{from: sdkModulePath + "/plugin/sqlite", to: frameModulePath + "/sqlite", allowed: true},
		{from: runtimeModulePath + "/internal/domain/newaggregate", to: frameModulePath + "/portable", allowed: true},
		{from: runtimeModulePath + "/internal/domain/newaggregate", to: frameModulePath},
		{from: runtimeModulePath + "/internal/application/agent/newusecase", to: frameModulePath, allowed: true},
		{from: runtimeModulePath + "/internal/application/agent/newusecase", to: frameModulePath + "/process/mcp"},
		{from: frameModulePath + "/portable", to: frameModulePath},
		{from: frameModulePath + "/portable", to: frameModulePath + "/value", allowed: true},
		{from: frameModulePath, to: frameModulePath + "/process/mcp"},
		{from: frameModulePath + "/process/mcp", to: frameModulePath, allowed: true},
		{from: runtimeModulePath + "/internal/domain/newaggregate", to: sdkModulePath + "/model", allowed: true},
		{from: runtimeModulePath + "/internal/domain/newaggregate", to: sdkModulePath + "/question", allowed: true},
		{from: runtimeModulePath + "/internal/domain/newaggregate", to: sdkModulePath + "/builtin", allowed: true},
		{from: sdkModulePath + "/question", to: runtimeModulePath + "/internal/application/agent/runs"},
		{from: sdkModulePath + "/builtin", to: runtimeModulePath + "/internal/adapter/toolset"},
		{from: sdkModulePath + "/plugin/sqlite", to: sdkModulePath + "/extension", allowed: true},
		{from: sdkModulePath + "/extension", to: runtimeModulePath + "/internal/application/agent/runs"},
		{from: sdkModulePath + "/model", to: sdkModulePath + "/model/catalog"},
		{from: sdkModulePath + "/model/catalog", to: sdkModulePath + "/model", allowed: true},
		{from: frameModulePath + "/sqlite", to: sdkModulePath + "/capture", allowed: true},
		{from: frameModulePath + "/sqlite", to: runtimeModulePath + "/internal/application/agent/runs"},
		{from: runtimeModulePath + "/internal/domain/newaggregate", to: frameModulePath + "/sqlite"},
		{from: runtimeModulePath + "/internal/domain/newaggregate", to: sdkModulePath + "/identity", allowed: true},
		{from: runtimeModulePath + "/internal/domain/newaggregate", to: sdkModulePath + "/protocol"},
		{from: runtimeModulePath + "/internal/domain/newaggregate", to: sdkModulePath + "/extension", allowed: true},
		{from: runtimeModulePath + "/internal/application/agent/newusecase", to: sdkModulePath + "/extension", allowed: true},
		{from: runtimeModulePath + "/internal/domain/newaggregate", to: sdkModulePath + "/plugin"},
		{from: runtimeModulePath + "/internal/application/agent/newusecase", to: sdkModulePath + "/plugin"},
		{from: sdkModulePath + "/extension", to: sdkModulePath + "/plugin"},
		{from: sdkModulePath + "/extension", to: frameModulePath + "/value", allowed: true},
		{from: runtimeModulePath + "/internal/domain/newaggregate", to: runtimeModulePath + "/internal/completion"},
		{from: runtimeModulePath + "/internal/application/agent/newusecase", to: runtimeModulePath + "/internal/completion", allowed: true},
		{from: sdkModulePath + "/identity", to: runtimeModulePath + "/internal/domain/session"},
		{from: runtimeModulePath + "/internal/completion", to: runtimeModulePath + "/internal/application/agent/runs"},
		{from: sdkModulePath + "/protocol", to: frameModulePath + "/value", allowed: true},
		{from: sdkModulePath + "/protocol", to: frameModulePath + "/browser"},
		{from: runtimeModulePath + "/internal/domain/newaggregate", to: frameModulePath + "/browser"},
		{from: runtimeModulePath + "/internal/application/agent/newusecase", to: frameModulePath + "/browser"},
		{from: sdkModulePath + "/protocol", to: runtimeModulePath + "/internal/application/agent/runs"},
		{from: sdkModulePath + "/protocol", to: sdkModulePath + "/capture"},
		{from: localRuntimeModulePath, to: runtimeModulePath + "/internal/bootstrap"},
		{from: runtimeModulePath + "/cmd/contractgen", to: runtimeModulePath + "/cmd/contractgen", allowed: true},
		{from: runtimeModulePath + "/cmd/contractgen", to: sdkModulePath + "/wiregen", allowed: true},
		{from: runtimeModulePath + "/internal/application/agent/newusecase", to: sdkModulePath + "/wiregen"},
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
