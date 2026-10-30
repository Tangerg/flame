package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestRuntimeHookTrustAndRevocationSurviveRestart(t *testing.T) {
	t.Setenv("FLAME_PROVIDER", "anthropic")
	t.Setenv("ANTHROPIC_API_KEY", "")

	config := Config{
		DataDirectory: testsupport.TempDirectory(t), DefaultWorkspacePath: testsupport.TempDirectory(t),
		UserHomePath: testsupport.TempDirectory(t), ConfigDirectories: []string{testsupport.TempDirectory(t)},
	}
	if err := os.Mkdir(filepath.Join(config.DefaultWorkspacePath, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{config.UserHomePath, config.DefaultWorkspacePath} {
		if err := os.Mkdir(filepath.Join(root, ".flame"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, ".flame", "hooks.json"), []byte(`{"hooks":[{"event":"SessionStart","inject":"context"}]}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	current, err := Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = current.Close() })
	inspect := func(trusted bool) string {
		t.Helper()
		result, err := current.ListHooks(t.Context(), protocol.ListHooksRequest{
			Workspace: protocol.WorkspaceRef{Path: config.DefaultWorkspacePath},
		}, protocol.CallOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if result.ProjectRoot == "" || result.ProjectTrusted != trusted || len(result.Hooks) != 2 {
			t.Fatalf("ListHooks = %+v, want global and project hooks with trust %v", result, trusted)
		}
		for _, hook := range result.Hooks {
			if want := hook.Scope == protocol.HookScopeGlobal || trusted; hook.Active != want {
				t.Fatalf("hook = %+v, want active %v", hook, want)
			}
		}
		return result.ProjectRoot
	}
	root := inspect(false)
	for _, change := range []struct {
		trusted bool
		key     string
	}{
		{trusted: true, key: "trust-project-hooks"},
		{trusted: false, key: "revoke-project-hooks"},
	} {
		if err := current.SetHookTrust(t.Context(), protocol.SetHookTrustRequest{
			ProjectRoot: root, Trusted: change.trusted,
		}, protocol.CommandOptions{IdempotencyKey: change.key}); err != nil {
			t.Fatal(err)
		}
		inspect(change.trusted)
		if err := current.Close(); err != nil {
			t.Fatal(err)
		}
		current, err = Open(t.Context(), config)
		if err != nil {
			t.Fatal(err)
		}
		inspect(change.trusted)
	}
}
