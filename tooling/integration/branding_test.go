package integration

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var retiredProductBrandTokens = [][]byte{
	[]byte("LY" + "RA"),
	[]byte("Ly" + "ra"),
	[]byte("ly" + "ra"),
}

const (
	brandScanChunkBytes       = 64 << 10
	maxRetiredBrandTokenBytes = len("ly" + "ra")
)

// TestAppHasNoRetiredProductBrand keeps the breaking product-identity cutover
// complete across every shipped Flame module. Generated dependencies and build
// outputs are not repository product sources.
func TestAppHasNoRetiredProductBrand(t *testing.T) {
	appRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	sources, err := maintainedProductFiles(appRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, relative := range sources {
		path := filepath.Join(appRoot, relative)
		for _, token := range retiredProductBrandTokens {
			if bytes.Contains([]byte(relative), token) {
				t.Errorf("retired product brand remains in path %s", relative)
			}
		}
		if !productBrandTextFile(filepath.Base(relative)) {
			continue
		}
		retiredToken, err := retiredBrandInFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if retiredToken != "" {
			t.Errorf("retired product brand %q remains in %s", retiredToken, relative)
		}
	}
}

// Git owns the maintained-source boundary, including ignored embed staging and
// build outputs. New output locations must not create another exclusion policy.
func maintainedProductFiles(root string) ([]string, error) {
	command := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	command.Dir = root
	data, err := command.Output()
	if err != nil {
		return nil, err
	}
	paths := strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
	return slices.DeleteFunc(paths, func(path string) bool { return path == "" }), nil
}

func retiredBrandInFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	var window [brandScanChunkBytes + maxRetiredBrandTokenBytes - 1]byte
	overlap := 0
	for {
		n, readErr := file.Read(window[overlap:])
		chunk := window[:overlap+n]
		for _, token := range retiredProductBrandTokens {
			if bytes.Contains(chunk, token) {
				return string(token), nil
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return "", nil
			}
			return "", readErr
		}
		overlap = min(len(chunk), maxRetiredBrandTokenBytes-1)
		copy(window[:overlap], chunk[len(chunk)-overlap:])
	}
}

func TestProductBrandScanFollowsTrackedAndUntrackedSource(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		command := exec.CommandContext(t.Context(), "git", args...)
		command.Dir = root
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	write := func(name, data string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "--quiet")
	write(".gitignore", "out/\napps/desktop/assets/web/\nnode_modules/\n**/config.yaml\n")
	write("assets/brand/README.md", "Flame")
	git("add", "--", ".gitignore", "assets/brand/README.md")
	write("apps/web/src/new.ts", "export const product = 'Flame'")
	write("apps/desktop/build/config.yml", "name: Flame")
	for _, name := range []string{"out/generated.js", "apps/desktop/assets/web/vendor.js", "apps/web/node_modules/vendor/index.js", "runtime/config/config.yaml"} {
		write(name, "private or generated")
	}
	actual, err := maintainedProductFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(actual)
	expected := []string{".gitignore", "apps/desktop/build/config.yml", "apps/web/src/new.ts", "assets/brand/README.md"}
	if !slices.Equal(actual, expected) {
		t.Fatalf("maintained product sources = %v, want %v", actual, expected)
	}
}

func productBrandTextFile(name string) bool {
	if name == "Makefile" || strings.HasPrefix(name, ".git") {
		return true
	}
	switch filepath.Ext(name) {
	case ".cjs", ".css", ".go", ".html", ".js", ".json", ".jsx", ".md", ".mjs",
		".plist", ".ts", ".tsx", ".txt", ".xml", ".yaml", ".yml":
		return true
	default:
		return false
	}
}
