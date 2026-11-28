package bootstrap

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func TestWorkspaceFileListsUseTheSelectedGitRepository(t *testing.T) {
	source, git := taskWorktreeTestRepository(t)
	git("commit", "--quiet", "--allow-empty", "-m", "outer checkout")
	const name = "nested"
	nested := filepath.Join(source, name)
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	git("-C", nested, "init", "--quiet", "--template=", "--initial-branch=main")
	const owned, ignored, rules = "notes.txt", "local.txt", ".gitignore"
	for _, file := range []struct{ name, content string }{
		{owned, "nested repository notes\n"},
		{ignored, "ignored locally\n"},
		{rules, ignored + "\n"},
	} {
		if err := os.WriteFile(filepath.Join(nested, file.name), []byte(file.content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	git("-C", nested, "add", "--", owned, rules)
	git("-C", nested, "commit", "--quiet", "-m", "nested checkout")
	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath = source
	r := poMCPOpen(t, cfg)
	ctx := protocolLifecycleContext(t.Context())
	want := []string{name + "/" + rules, name + "/" + owned}

	verify := func(location string) {
		t.Helper()
		for _, recursive := range []bool{false, true} {
			page, err := r.api.ListWorkspaceFiles(ctx, protocol.ListFilesRequest{
				Workspace: protocol.WorkspaceRef{Path: source}, Path: name, Recursive: recursive,
			})
			if err != nil {
				t.Errorf("list %s, recursive=%t: %v", location, recursive, err)
				continue
			}
			paths := make([]string, 0, len(page.Data))
			for _, entry := range page.Data {
				if entry.Type != protocol.FileEntryFile {
					t.Errorf("list %s, recursive=%t returned a non-file: %+v", location, recursive, entry)
				}
				paths = append(paths, entry.Path)
			}
			slices.Sort(paths)
			if !slices.Equal(paths, want) {
				t.Errorf("list %s, recursive=%t returned %q; want %q", location, recursive, paths, want)
			}
		}
	}
	verify("untracked nested repository")
	git("add", "--", name)
	verify("tracked gitlink")
}
