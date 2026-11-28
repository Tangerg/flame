package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestTaskWorktreeHandoffComparesRecreatedPathsOnce(t *testing.T) {
	source, git := taskWorktreeTestRepository(t)
	const recreated, other = "a [1].txt", "z.txt"
	for _, name := range []string{recreated, other} {
		if err := os.WriteFile(filepath.Join(source, name), []byte("original "+name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	git("add", ".")
	git("commit", "--quiet", "-m", "original material")
	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath = source
	r := poMCPOpen(t, cfg)
	task := r.must(delivery.TaskWorktreesCreate, protocol.CreateTaskWorktreeRequest{
		Source: protocol.WorkspaceRef{Path: source}, Base: "HEAD",
	}, "create-task-for-recreated-path").(*protocol.TaskWorktree)
	git("-C", task.Workspace.Path, "--literal-pathspecs", "rm", "--cached", "--", recreated)
	for _, name := range []string{recreated, other} {
		if err := os.WriteFile(filepath.Join(task.Workspace.Path, name), []byte("updated "+name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	index := git("-C", task.Workspace.Path, "rev-parse", "--path-format=absolute", "--git-path", "index")
	before, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	objects := git("count-objects", "-v")

	read := r.endpoint.Invoke(protocolLifecycleContext(t.Context()), delivery.WorkspaceDiffGet, protocol.GetDiffRequest{
		Workspace: task.Workspace, Format: protocol.DiffFormatVisual,
	}, protocol.CallMetadata{})
	if read.Failure != nil {
		t.Errorf("workspace diff refused the recreated path: %v", read.Failure)
	} else {
		visual := read.Value.(*protocol.Diff).Visual
		if visual == nil || len(visual.Manifest) != 2 || !slices.ContainsFunc(visual.Manifest, func(file protocol.WorkspaceFileChange) bool {
			return file.Path == recreated && file.Status == protocol.FileStatusModified
		}) {
			t.Errorf("recreated path is missing from the worktree comparison: %+v", visual)
		}
	}
	preview := r.must(delivery.TaskWorktreesPreview, protocol.TaskWorktreeRequest{ID: task.ID}, "").(*protocol.WorktreeHandoff)
	if marker := "diff --git a/" + recreated + " b/" + recreated + "\n"; strings.Count(preview.Patch, marker) != 1 {
		t.Errorf("handoff did not compare the recreated file once: %q", preview.Patch)
	}
	if after, err := os.ReadFile(index); err != nil || !bytes.Equal(before, after) {
		t.Fatalf("the worktree comparison changed its source index: %v", err)
	}
	if after := git("count-objects", "-v"); after != objects {
		t.Fatalf("the worktree comparison wrote objects to the source repository: %s", after)
	}

	r.must(delivery.TaskWorktreesApply, protocol.ApplyWorktreeHandoffRequest{ID: task.ID, Digest: preview.Digest},
		"apply-recreated-path-handoff")
	for _, name := range []string{recreated, other} {
		if got := git("show", ":"+name); got != "updated "+name {
			t.Errorf("handoff staged %q for %q", got, name)
		}
	}
}

func TestTaskWorktreePreviewReportsPatchCapacity(t *testing.T) {
	source, git := taskWorktreeTestRepository(t)
	git("commit", "--quiet", "--allow-empty", "-m", "initial checkout")
	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath = source
	r := poMCPOpen(t, cfg)
	task := r.must(delivery.TaskWorktreesCreate, protocol.CreateTaskWorktreeRequest{
		Source: protocol.WorkspaceRef{Path: source}, Base: "HEAD",
	}, "create-large-patch-worktree").(*protocol.TaskWorktree)
	asset, err := os.Create(filepath.Join(task.Workspace.Path, "asset.bin"))
	if err != nil {
		t.Fatal(err)
	}
	// Git's binary encoding expands incompressible data past its stdout ceiling.
	const assetBytes = 56 << 20
	_, writeErr := io.CopyN(asset, rand.NewChaCha8([32]byte{}), assetBytes)
	if err := errors.Join(writeErr, asset.Close()); err != nil {
		t.Fatal(err)
	}
	result := r.endpoint.Invoke(protocolLifecycleContext(t.Context()), delivery.TaskWorktreesPreview,
		protocol.TaskWorktreeRequest{ID: task.ID}, protocol.CallMetadata{})
	if result.Failure == nil || !errors.Is(result.Failure, protocol.ErrInvalidParams) {
		t.Fatalf("oversized binary handoff preview = %+v, %v; want invalid params", result.Value, result.Failure)
	}
}

func TestWorkspacePatchRefusesUntrackedRepository(t *testing.T) {
	source, git := taskWorktreeTestRepository(t)
	git("commit", "--quiet", "--allow-empty", "-m", "initial checkout")
	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath = source
	r := poMCPOpen(t, cfg)
	task := r.must(delivery.TaskWorktreesCreate, protocol.CreateTaskWorktreeRequest{
		Source: protocol.WorkspaceRef{Path: source}, Base: "HEAD",
	}, "create-worktree-with-embedded-repository").(*protocol.TaskWorktree)

	const name = "nested"
	nested := filepath.Join(task.Workspace.Path, name)
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	git("-C", nested, "init", "--quiet", "--template=", "--initial-branch=main")
	git("-C", nested, "commit", "--quiet", "--allow-empty", "-m", "nested repository")
	if paths := git("-C", task.Workspace.Path, "ls-files", "--others", "--exclude-standard"); paths != name+"/" {
		t.Fatalf("untracked paths = %q; want one untracked repository directory", paths)
	}

	for _, query := range []struct {
		method delivery.Name
		input  any
	}{
		{delivery.TaskWorktreesPreview, protocol.TaskWorktreeRequest{ID: task.ID}},
		{delivery.WorkspaceDiffGet, protocol.GetDiffRequest{Workspace: task.Workspace, Format: protocol.DiffFormatRaw}},
	} {
		result := r.endpoint.Invoke(protocolLifecycleContext(t.Context()), query.method, query.input, protocol.CallMetadata{})
		if result.Failure == nil || !errors.Is(result.Failure, protocol.ErrInvalidParams) || !strings.Contains(result.Failure.Error(), name+"/") {
			t.Errorf("%s with an untracked repository = %+v, %v; want explicit patch refusal", query.method, result.Value, result.Failure)
		}
	}
	if err := os.Rename(nested, filepath.Join(t.TempDir(), name)); err != nil {
		t.Fatal(err)
	}
	const empty = "empty.txt"
	if err := os.WriteFile(filepath.Join(task.Workspace.Path, empty), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	preview := r.must(delivery.TaskWorktreesPreview, protocol.TaskWorktreeRequest{ID: task.ID}, "").(*protocol.WorktreeHandoff)
	if !strings.Contains(preview.Patch, empty) {
		t.Fatalf("handoff omitted the ordinary empty file: %q", preview.Patch)
	}
}

func TestWorkspacePatchReportsAFileRemovedDuringRead(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Git timing fixture uses a POSIX shell")
	}
	source, git := taskWorktreeTestRepository(t)
	git("commit", "--quiet", "--allow-empty", "-m", "initial checkout")
	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath = source
	r := poMCPOpen(t, cfg)
	const name = "vanishing.txt"
	file := filepath.Join(source, name)
	if err := os.WriteFile(file, []byte("temporary content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	entered, proceed := holdTaskGitBoundary(t, beforeUntrackedInclusion)
	ctx, cancel := context.WithTimeout(protocolLifecycleContext(t.Context()), lifecycleWaitBudget(t))
	var caller sync.WaitGroup
	defer func() {
		cancel()
		if err := os.WriteFile(proceed, nil, 0o600); err != nil {
			t.Error(err)
		}
		caller.Wait()
	}()
	output := make(chan protocol.OperationResult, 1)
	caller.Go(func() {
		output <- r.endpoint.Invoke(ctx, delivery.WorkspaceDiffGet, protocol.GetDiffRequest{
			Workspace: protocol.WorkspaceRef{Path: source}, Format: protocol.DiffFormatRaw,
		}, protocol.CallMetadata{})
	})
	awaitTaskGitBoundary(t, ctx, entered, output)
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(proceed, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	caller.Wait()
	result := <-output
	if result.Failure == nil || !errors.Is(result.Failure, protocol.ErrInvalidParams) || !strings.Contains(result.Failure.Error(), name) {
		t.Fatalf("patch after the selected file disappeared = %+v, %v; want explicit patch refusal", result.Value, result.Failure)
	}
}
