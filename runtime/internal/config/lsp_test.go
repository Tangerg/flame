package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLSPServers_FromYAML(t *testing.T) {
	const yaml = `
provider: anthropic
lsp:
  servers:
    - name: gopls
      command: gopls
      languageId: go
      extensions: [".go"]
      rootMarkers: ["go.mod"]
    - name: pyright
      command: pyright-langserver
      args: ["--stdio"]
      languageId: python
      extensions: [".py"]
      rootMarkers: ["pyproject.toml", "setup.py"]
`
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "config.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLAME_PROVIDER", "")
	settings, err := Load([]string{directory})
	servers := settings.LSPServers
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 {
		t.Fatalf("got %d servers, want 2", len(servers))
	}
	py := servers[1]
	if py.Name != "pyright" || py.Command != "pyright-langserver" || py.LanguageID != "python" {
		t.Errorf("pyright spec = %+v, want name/command/languageId populated", py)
	}
	if len(py.Args) != 1 || py.Args[0] != "--stdio" {
		t.Errorf("pyright args = %v, want [--stdio]", py.Args)
	}
	if len(py.Extensions) != 1 || py.Extensions[0] != ".py" {
		t.Errorf("pyright extensions = %v, want [.py]", py.Extensions)
	}
	if len(py.RootMarkers) != 2 {
		t.Errorf("pyright rootMarkers = %v, want 2", py.RootMarkers)
	}
}

func TestLoadLSPServers_Absent(t *testing.T) {
	t.Setenv("FLAME_PROVIDER", "anthropic")
	settings, err := Load([]string{t.TempDir()})
	servers := settings.LSPServers
	if err != nil {
		t.Fatal(err)
	}
	if servers != nil {
		t.Errorf("got %v, want nil (fall back to engine defaults)", servers)
	}
}
