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

const (
	modulePath  = "github.com/Tangerg/flame/cli"
	runtimePath = "github.com/Tangerg/flame/runtime"
	oolongPath  = "github.com/Tangerg/oolong"
	cobraPath   = "github.com/spf13/cobra"
	viperPath   = "github.com/spf13/viper"
)

type ring uint8

const (
	ringUnknown ring = iota
	ringMechanism
	ringDomain
	ringApplication
	ringAdapter
	ringDelivery
	ringTestSupport
	ringComposition
)

func TestDependenciesPointInward(t *testing.T) {
	root := moduleRoot(t)
	walkProduction(t, root, func(relative, path string) {
		from := ringOf(relative)
		if from == ringUnknown {
			t.Errorf("%s belongs to no architecture ring", relative)
			return
		}
		for _, imported := range imports(t, path) {
			internal, ok := strings.CutPrefix(imported, modulePath+"/")
			if !ok {
				continue
			}
			to := ringOf(internal)
			if to == ringUnknown {
				t.Errorf("%s imports unclassified package %s", relative, internal)
				continue
			}
			if !mayDependOn(from, to) {
				t.Errorf("%s imports %s: dependencies must point inward", relative, internal)
			}
			if deliveryPeers(relative, internal) {
				t.Errorf("%s imports peer delivery adapter %s; compose them in main", relative, internal)
			}
		}
	})
}

func TestExternalFrameworksStopAtTheirAdapters(t *testing.T) {
	root := moduleRoot(t)
	walkProduction(t, root, func(relative, path string) {
		for _, imported := range imports(t, path) {
			switch {
			case importsPath(imported, runtimePath) && imported != runtimePath+"/protocol":
				if !strings.HasPrefix(relative, "internal/adapter/runtimebinding/") {
					t.Errorf("%s imports the concrete Runtime binding outside runtimebinding", relative)
				}
			case importsPath(imported, oolongPath):
				if !strings.HasPrefix(relative, "internal/delivery/terminal/") {
					t.Errorf("%s imports Oolong outside terminal delivery", relative)
				}
			case importsPath(imported, cobraPath), importsPath(imported, viperPath):
				if !strings.HasPrefix(relative, "internal/delivery/cmd/") {
					t.Errorf("%s imports command framework outside cmd delivery", relative)
				}
			}
		}
	})
}

func TestPackagePathsDoNotRepeatOwners(t *testing.T) {
	root := filepath.Join(moduleRoot(t), "internal")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && entry.Name() == "testdata" {
			return filepath.SkipDir
		}
		if !entry.IsDir() || path == root || !hasDirectProductionGo(path) {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		leaf := parts[len(parts)-1]
		for _, owner := range parts[:len(parts)-1] {
			if len(leaf) > len(owner) && (strings.HasPrefix(leaf, owner) || strings.HasSuffix(leaf, owner)) {
				t.Errorf("%s repeats enclosing owner %s", relative, owner)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan package names: %v", err)
	}
}

func TestNamespacesHaveMoreThanOneChild(t *testing.T) {
	root := filepath.Join(moduleRoot(t), "internal")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() || path == root || entry.Name() == "testdata" {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if hasDirectProductionGo(path) {
			return nil
		}
		children, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		packages := 0
		for _, child := range children {
			if child.IsDir() && child.Name() != "testdata" && hasProductionGo(filepath.Join(path, child.Name())) {
				packages++
			}
		}
		if packages == 1 {
			t.Errorf("%s is a single-child namespace; collapse it into its child or owner", filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan namespaces: %v", err)
	}
}

func ringOf(relative string) ring {
	relative = filepath.ToSlash(relative)
	switch {
	case relative == "." || !strings.HasPrefix(relative, "internal/"):
		return ringComposition
	case packageWithin(relative, "internal/arch"):
		return ringComposition
	case packageWithin(relative, "internal/exactint"):
		return ringMechanism
	case packageWithin(relative, "internal/domain"):
		return ringDomain
	case packageWithin(relative, "internal/application"):
		return ringApplication
	case packageWithin(relative, "internal/adapter"):
		return ringAdapter
	case packageWithin(relative, "internal/delivery"):
		return ringDelivery
	case packageWithin(relative, "internal/runtimefixture"):
		return ringTestSupport
	default:
		return ringUnknown
	}
}

func packageWithin(path, root string) bool {
	return path == root || strings.HasPrefix(path, root+"/")
}

func mayDependOn(from, to ring) bool {
	if from == to {
		return true
	}
	if to == ringTestSupport {
		return false
	}
	switch from {
	case ringMechanism:
		return false
	case ringDomain:
		return to == ringMechanism
	case ringApplication:
		return to == ringDomain || to == ringMechanism
	case ringAdapter:
		return to == ringApplication || to == ringDomain || to == ringMechanism
	case ringDelivery:
		return to == ringAdapter || to == ringApplication || to == ringDomain || to == ringMechanism
	case ringTestSupport, ringComposition:
		return true
	default:
		return false
	}
}

func deliveryPeers(from, to string) bool {
	fromCmd := strings.HasPrefix(from, "internal/delivery/cmd/")
	fromTerminal := strings.HasPrefix(from, "internal/delivery/terminal/")
	toCmd := strings.HasPrefix(to, "internal/delivery/cmd/")
	toTerminal := strings.HasPrefix(to, "internal/delivery/terminal/")
	return (fromCmd && toTerminal) || (fromTerminal && toCmd)
}

func importsPath(imported, root string) bool {
	return imported == root || strings.HasPrefix(imported, root+"/")
}

func walkProduction(t *testing.T, root string, visit func(relative, path string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "vendor" || entry.Name() == ".git" || entry.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		visit(filepath.ToSlash(relative)+"/", path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk production sources: %v", err)
	}
}

func imports(t *testing.T, path string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	result := make([]string, 0, len(file.Imports))
	for _, imported := range file.Imports {
		result = append(result, strings.Trim(imported.Path.Value, `"`))
	}
	return result
}

func hasDirectProductionGo(directory string) bool {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".go" && !strings.HasSuffix(entry.Name(), "_test.go") {
			return true
		}
	}
	return false
}

func hasProductionGo(directory string) bool {
	found := false
	_ = filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || found {
			return filepath.SkipAll
		}
		if entry.IsDir() && entry.Name() == "testdata" {
			return filepath.SkipDir
		}
		if !entry.IsDir() && filepath.Ext(path) == ".go" && !strings.HasSuffix(path, "_test.go") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("go.mod not found")
		}
		directory = parent
	}
}
