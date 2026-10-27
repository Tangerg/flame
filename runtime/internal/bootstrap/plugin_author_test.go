package bootstrap

import (
	json "encoding/json/v2"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/pluginpackage"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

const fixturePluginCommand = "./bin/mcp-fixture"

func preparePluginFixture(executable string) error {
	_, file, _, _ := goruntime.Caller(0)
	command := exec.Command(filepath.Join(goruntime.GOROOT(), "bin", "go"), "build", "-trimpath", "-o", filepath.Join(filepath.Dir(executable), "mcp-fixture"), "./testdata/mcp-fixture")
	command.Dir = filepath.Join(filepath.Dir(file), "../..")
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	return command.Run()
}

func testMCPServer(endpoint string) map[string]any {
	return map[string]any{"type": "stdio", "command": fixturePluginCommand, "args": []string{"proxy", endpoint}}
}

func testMCPServerJSON(t *testing.T, endpoint string) string {
	t.Helper()
	data, err := json.Marshal(testMCPServer(endpoint))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// Test remotes own their real Qimu lifecycle. The packaged fixture only
// transports bytes; it cannot manufacture setup or activation acknowledgments.
func publishTestBackend(t *testing.T, server *sdk.Server) {
	t.Helper()
	backend, err := qimumcp.NewBackend(server)
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.Publish(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })
}

func authorPluginPackage(t *testing.T, source string) {
	t.Helper()
	declaration, err := os.ReadFile(filepath.Join(source, "mcp.json"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if strings.Contains(string(declaration), fixturePluginCommand) {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		binary, err := os.ReadFile(filepath.Join(filepath.Dir(executable), "mcp-fixture"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(source, "bin"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, "bin", "mcp-fixture"), binary, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pluginpackage.BuildRelease(t.Context(), source, pluginpackage.BuildTarget{OS: goruntime.GOOS, Arch: goruntime.GOARCH}, io.Discard); err != nil {
		t.Fatalf("author plugin release: %v", err)
	}
}
