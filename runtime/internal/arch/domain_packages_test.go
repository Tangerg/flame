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

// TestDomainPackagesDeclareAndVerifyTheirBoundaries makes every bounded context
// explicit. A small value package is welcome, but an undocumented or entirely
// untested directory cannot silently become a field-container dumping ground.
func TestDomainPackagesDeclareAndVerifyTheirBoundaries(t *testing.T) {
	domainRoot := filepath.Join(moduleRoot(t), "internal", "domain")
	err := filepath.WalkDir(domainRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() || path == domainRoot {
			return nil
		}
		hasPackage, err := directoryHasDirectProductionGo(path)
		if err != nil {
			return err
		}
		if !hasPackage {
			return nil
		}
		relative, err := filepath.Rel(domainRoot, path)
		if err != nil {
			return err
		}
		t.Run(filepath.ToSlash(relative), func(t *testing.T) {
			assertReviewedDomainPackage(t, path, filepath.Base(path))
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk Domain packages: %v", err)
	}
}

func directoryHasDirectProductionGo(dir string) (bool, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".go") && !strings.HasSuffix(file.Name(), "_test.go") {
			return true, nil
		}
	}
	return false, nil
}

func assertReviewedDomainPackage(t *testing.T, dir, packageName string) {
	t.Helper()
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read package: %v", err)
	}
	hasTest := false
	hasPackageDoc := false
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if !strings.HasSuffix(file.Name(), ".go") {
			continue
		}
		if strings.HasSuffix(file.Name(), "_test.go") {
			hasTest = true
			continue
		}
		parsed, parseErr := parser.ParseFile(
			token.NewFileSet(), filepath.Join(dir, file.Name()), nil, parser.ParseComments,
		)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", file.Name(), parseErr)
		}
		if parsed.Name.Name != packageName {
			t.Errorf("%s declares package %s, want directory name %s", file.Name(), parsed.Name.Name, packageName)
		}
		if parsed.Doc != nil && strings.HasPrefix(parsed.Doc.Text(), "Package "+packageName+" ") {
			hasPackageDoc = true
		}
	}
	if !hasPackageDoc {
		t.Errorf("package %s has no boundary comment beginning with %q", packageName, "Package "+packageName)
	}
	if !hasTest {
		t.Errorf("package %s has no direct Domain test", packageName)
	}
}
