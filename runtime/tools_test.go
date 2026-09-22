package runtime

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

func TestDirectToolBindingPreservesScopeInputRejection(t *testing.T) {
	for _, name := range []string{
		"FLAME_MODEL", "FLAME_APIKEY", "FLAME_BASEURL", "ANTHROPIC_API_KEY",
		"FLAME_MCP_SERVERS", "FLAME_A2A_AGENTS", "FLAME_A2A_RPC_ORIGINS",
	} {
		t.Setenv(name, "")
	}
	t.Setenv("FLAME_PROVIDER", "anthropic")
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("flame\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rt, err := Open(t.Context(), Config{
		DataDirectory: t.TempDir(), DefaultWorkspacePath: root,
		UserHomePath: t.TempDir(), ConfigDirectories: []string{t.TempDir()},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := rt.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, arguments := range []map[string]any{
		{"Path": path},
		{"path": path, "start_line": 0},
		{"path": path, "max_lines": nil},
	} {
		result, err := rt.InvokeTool(t.Context(), protocol.InvokeToolRequest{
			Name: "read", Arguments: arguments,
		}, CommandOptions{})
		if result != nil || !errors.Is(err, protocol.ErrInvalidParams) {
			t.Fatalf("InvokeTool(%v) = (%v, %v), want invalid params", arguments, result, err)
		}
		problem, ok := errors.AsType[protocol.ProblemError](err)
		if !ok || problem.Problem().Type != protocol.ErrInvalidParams.Error() {
			t.Fatalf("input rejection lost its protocol problem: %v", err)
		}
	}
	result, err := rt.InvokeTool(t.Context(), protocol.InvokeToolRequest{
		Name: "read", Arguments: map[string]any{"path": path},
	}, CommandOptions{})
	if err != nil {
		t.Fatalf("valid read after input rejection: %v", err)
	}
	read, ok := result.(map[string]any)
	if !ok || read["content"] != "flame\n" {
		t.Fatalf("read result = %#v", result)
	}
}
