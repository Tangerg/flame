package bootstrap

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestWorkspaceGitDiffPreservesFileIdentities(t *testing.T) {
	source, git := taskWorktreeTestRepository(t)
	const (
		tracked   = "client notes.txt"
		untracked = "new draft.txt"
		modeOnly  = "scripts b/start.sh"
		binary    = "assets and b/blob.bin"
		before    = "previous name.txt"
		after     = "new name.txt"
	)
	write := func(path, contents string) {
		t.Helper()
		file := filepath.Join(source, path)
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(tracked, "old notes\n")
	write(modeOnly, "#!/bin/sh\n")
	write(binary, "\x00old asset\n")
	write(before, "renamed without content changes\n")
	git("add", ".")
	git("commit", "--quiet", "-m", "original files")
	git("config", "core.filemode", "false")
	git("update-index", "--chmod=+x", "--", modeOnly)
	git("mv", "--", before, after)
	write(tracked, "updated notes\n")
	write(binary, "\x00updated asset\n")
	write(untracked, "new draft\n")

	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath = source
	r := poMCPOpen(t, cfg)
	diff := r.must(delivery.WorkspaceDiffGet, protocol.GetDiffRequest{
		Workspace: protocol.WorkspaceRef{Path: source}, Format: protocol.DiffFormatVisual,
	}, "").(*protocol.Diff)
	want := []struct {
		path, previous string
		status         protocol.FileStatus
		binary         bool
	}{
		{path: tracked, status: protocol.FileStatusModified},
		{path: untracked, status: protocol.FileStatusUntracked},
		{path: modeOnly, status: protocol.FileStatusModified},
		{path: binary, status: protocol.FileStatusModified, binary: true},
		{path: after, previous: before, status: protocol.FileStatusRenamed},
	}
	if diff.Visual == nil || len(diff.Visual.Manifest) != len(want) {
		t.Fatalf("workspace diff has the wrong set of files: %+v", diff.Visual)
	}
	for _, expected := range want {
		if !slices.ContainsFunc(diff.Visual.Manifest, func(change protocol.WorkspaceFileChange) bool {
			return change.Path == expected.path && change.PreviousPath == expected.previous && change.Status == expected.status && change.Binary == expected.binary
		}) {
			t.Errorf("workspace diff lacks %+v: %+v", expected, diff.Visual.Manifest)
		}
	}
}
