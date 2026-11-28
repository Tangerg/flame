package bootstrap

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestWorkspaceChangesUseTheSameGitComparisonAsDiff(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		withCommit bool
	}{
		{name: "recreated tracked path", withCommit: true},
		{name: "initial staged path"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			source, git := taskWorktreeTestRepository(t)
			const material = "material.txt"
			path := filepath.Join(source, material)
			if err := os.WriteFile(path, []byte("first line\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			git("add", "--", material)
			if scenario.withCommit {
				git("commit", "--quiet", "-m", "initial material")
				git("rm", "--cached", "--", material)
				if err := os.WriteFile(path, []byte("new material\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(source, "asset.bin"), []byte("\x00asset"), 0o600); err != nil {
				t.Fatal(err)
			}

			cfg := poMCPConfig(t)
			cfg.defaultWorkspacePath = source
			r := poMCPOpen(t, cfg)
			workspace := protocol.WorkspaceRef{Path: source}
			visual := r.must(delivery.WorkspaceDiffGet, protocol.GetDiffRequest{
				Workspace: workspace, Format: protocol.DiffFormatVisual,
			}, "").(*protocol.Diff).Visual
			if visual == nil || visual.Truncated || len(visual.Manifest) != 2 {
				t.Fatalf("worktree comparison = %+v; want both files", visual)
			}
			listed := r.must(delivery.WorkspaceChangesList, protocol.WorkspaceQuery{Workspace: workspace}, "").(*protocol.Page[protocol.WorkspaceFileChange])
			if !reflect.DeepEqual(listed.Data, visual.Manifest) {
				t.Fatalf("change list = %+v; want Git comparison %+v", listed.Data, visual.Manifest)
			}
		})
	}
}

func TestWorkspaceGitComparisonKeepsLinkedSplitIndexUntouched(t *testing.T) {
	source, git := taskWorktreeTestRepository(t)
	const tracked, draft = "tracked.txt", "draft.txt"
	write := func(dir, name, contents string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(source, tracked, "original\n")
	git("add", "--", tracked)
	git("commit", "--quiet", "-m", "original material")
	checkout := filepath.Join(t.TempDir(), "checkout")
	git("worktree", "add", "--quiet", "--detach", checkout, "HEAD")
	git("-C", checkout, "update-index", "--split-index")
	write(checkout, tracked, "updated material\n")
	git("-C", checkout, "add", "--", tracked)
	write(checkout, draft, "new draft\n")

	index := git("-C", checkout, "rev-parse", "--path-format=absolute", "--shared-index-path")
	if index == "" {
		t.Fatal("Git did not create a shared index for the linked checkout")
	}
	previous := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(index, previous, previous); err != nil {
		t.Fatal(err)
	}

	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath = checkout
	r := poMCPOpen(t, cfg)
	visual := r.must(delivery.WorkspaceDiffGet, protocol.GetDiffRequest{
		Workspace: protocol.WorkspaceRef{Path: checkout}, Format: protocol.DiffFormatVisual,
	}, "").(*protocol.Diff).Visual
	if visual == nil || len(visual.Manifest) != 2 || visual.Manifest[0].Path != draft || visual.Manifest[1].Path != tracked {
		t.Errorf("worktree comparison = %+v; want the draft and the staged edit", visual)
	}
	info, err := os.Stat(index)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(previous) {
		t.Errorf("source shared index modification time = %s; want %s", info.ModTime(), previous)
	}
}

func TestWorkspaceGitDiffPreservesFileIdentities(t *testing.T) {
	source, git := taskWorktreeTestRepository(t)
	const (
		tracked        = "client notes.txt"
		untracked      = "new draft.txt"
		modeOnly       = "scripts b/start.sh"
		binaryModeOnly = "assets and b/native.bin"
		binary         = "assets and b/blob.bin"
		before         = "previous name.txt"
		after          = "new name.txt"
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
	write(binaryModeOnly, "\x00unchanged executable\n")
	write(binary, "\x00old asset\n")
	write(before, "renamed without content changes\n")
	git("add", ".")
	git("commit", "--quiet", "-m", "original files")
	git("config", "core.filemode", "false")
	git("update-index", "--chmod=+x", "--", modeOnly, binaryModeOnly)
	git("mv", "--", before, after)
	write(tracked, "updated notes\n")
	write(binary, "\x00updated asset\n")
	write(untracked, "new draft\n")

	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath = source
	r := poMCPOpen(t, cfg)
	workspace := protocol.WorkspaceRef{Path: source}
	diff := r.must(delivery.WorkspaceDiffGet, protocol.GetDiffRequest{
		Workspace: workspace, Format: protocol.DiffFormatVisual,
	}, "").(*protocol.Diff)
	want := []struct {
		path, previous string
		status         protocol.FileStatus
		binary         bool
	}{
		{path: tracked, status: protocol.FileStatusModified},
		{path: untracked, status: protocol.FileStatusUntracked},
		{path: modeOnly, status: protocol.FileStatusModified},
		{path: binaryModeOnly, status: protocol.FileStatusModified, binary: true},
		{path: binary, status: protocol.FileStatusModified, binary: true},
		{path: after, previous: before, status: protocol.FileStatusRenamed},
	}
	if diff.Visual == nil || len(diff.Visual.Manifest) != len(want) {
		t.Fatalf("workspace diff has the wrong set of files: %+v", diff.Visual)
	}
	listed := r.must(delivery.WorkspaceChangesList, protocol.WorkspaceQuery{Workspace: workspace}, "").(*protocol.Page[protocol.WorkspaceFileChange])
	if !reflect.DeepEqual(listed.Data, diff.Visual.Manifest) {
		t.Errorf("change list = %+v; want Git comparison %+v", listed.Data, diff.Visual.Manifest)
	}
	for _, expected := range want {
		if !slices.ContainsFunc(diff.Visual.Manifest, func(change protocol.WorkspaceFileChange) bool {
			return change.Path == expected.path && change.PreviousPath == expected.previous && change.Status == expected.status && change.Binary == expected.binary
		}) {
			t.Errorf("workspace diff lacks %+v: %+v", expected, diff.Visual.Manifest)
		}
	}
}

func TestWorkspaceGitDiffProjectsTypeChangesAsOneFile(t *testing.T) {
	source, git := taskWorktreeTestRepository(t)
	const name, target = "material.txt", "target.txt"
	for _, file := range []string{name, target} {
		if err := os.WriteFile(filepath.Join(source, file), []byte("original\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	git("config", "core.symlinks", "true")
	git("add", "--", name, target)
	git("commit", "--quiet", "-m", "regular files")
	file := filepath.Join(source, name)
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, file); err != nil {
		t.Fatal(err)
	}
	if got := git("diff", "--name-status", "HEAD"); got != "T\t"+name {
		t.Fatalf("Git changes = %q; want one type change for %q", got, name)
	}

	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath = source
	r := poMCPOpen(t, cfg)
	request := protocol.GetDiffRequest{Workspace: protocol.WorkspaceRef{Path: source}, Format: protocol.DiffFormatRows}
	listed := r.must(delivery.WorkspaceChangesList, protocol.WorkspaceQuery{Workspace: request.Workspace}, "").(*protocol.Page[protocol.WorkspaceFileChange])
	if len(listed.Data) != 1 || listed.Data[0].Path != name || listed.Data[0].Status != protocol.FileStatusModified ||
		listed.Data[0].Added == nil || *listed.Data[0].Added != 1 || listed.Data[0].Removed == nil || *listed.Data[0].Removed != 1 {
		t.Errorf("change list should contain one type change with both line counts: %+v", listed.Data)
	}
	read := r.must(delivery.WorkspaceDiffGet, request, "").(*protocol.Diff)
	if read.Rows == nil || read.Rows.Truncated || len(read.Rows.Files) != 1 {
		t.Errorf("type change should be one complete file: %+v", read.Rows)
	} else {
		change := read.Rows.Files[0]
		if change.Path != name || change.Status != protocol.FileStatusModified || change.Added == nil || *change.Added != 1 || change.Removed == nil || *change.Removed != 1 {
			t.Errorf("type change lost evidence from one of its fragments: %+v", change)
		}
	}

	request.Format = protocol.DiffFormatVisual
	request.Limit = new(2)
	bounded := r.must(delivery.WorkspaceDiffGet, request, "").(*protocol.Diff)
	if bounded.Visual == nil || !bounded.Visual.Truncated || len(bounded.Visual.Manifest) != 0 || bounded.Visual.Patch != "" {
		t.Errorf("row limit exposed a partial file type change: %+v", bounded.Visual)
	}
}
