package delivery

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/infra/git"
)

func TestWorkspaceVcsUnavailable(t *testing.T) {
	if !git.Available() {
		t.Skip("git not on PATH")
	}
	s := newWorkspaceHandler(t.TempDir())
	if _, err := s.ListWorkspaceFileChanges(context.Background(), protocol.WorkspaceQuery{}); !errors.Is(err, protocol.ErrVcsUnavailable) {
		t.Errorf("listFileChanges err = %v, want ErrVcsUnavailable", err)
	}
	if _, err := s.GetWorkspaceDiff(context.Background(), protocol.GetDiffRequest{}); !errors.Is(err, protocol.ErrVcsUnavailable) {
		t.Errorf("getDiff err = %v, want ErrVcsUnavailable", err)
	}
}

func TestWorkspaceDiffRejectsMeaninglessOrNonPositiveRowLimits(t *testing.T) {
	s := newWorkspaceHandler(t.TempDir())
	if _, err := s.GetWorkspaceDiff(t.Context(), protocol.GetDiffRequest{
		Format: protocol.DiffFormatRaw,
		Limit:  valuePtr(1),
	}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("raw diff limit err = %v, want invalid_params", err)
	}
	if _, err := s.GetWorkspaceDiff(t.Context(), protocol.GetDiffRequest{
		Limit: valuePtr(0),
	}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("zero row limit err = %v, want invalid_params", err)
	}
}

func TestWorkspaceGitWireMapping(t *testing.T) {
	if !git.Available() {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	env := append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	gitCmd := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir, cmd.Env = dir, env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	gitCmd("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd("add", ".")
	gitCmd("commit", "-m", "init")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n\nB"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "empty.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s := newWorkspaceHandler(dir)
	page, err := s.ListWorkspaceFileChanges(context.Background(), protocol.WorkspaceQuery{})
	if err != nil {
		t.Fatalf("listFileChanges: %v", err)
	}
	if len(page.Data) != 2 || page.Data[0].Status != "modified" || page.Data[0].Added == nil {
		t.Fatalf("changes = %+v, want modified and empty untracked files", page.Data)
	}

	diff, err := s.GetWorkspaceDiff(context.Background(), protocol.GetDiffRequest{})
	if err != nil {
		t.Fatalf("getDiff: %v", err)
	}
	if len(diff.Rows.Files) != 2 || len(diff.Rows.Files[0].Rows) == 0 {
		t.Fatalf("diff = %+v, want modified and empty files", diff.Rows.Files)
	}
	if err := protocol.ValidateWireTree(diff); err != nil {
		t.Fatalf("real Git diff violates the response contract: %v", err)
	}
	blank := false
	for _, row := range diff.Rows.Files[0].Rows {
		if row.Type == protocol.DiffRowContext && row.Code != nil && *row.Code == "" {
			blank = true
		}
	}
	if !blank {
		t.Fatal("real Git diff lost its blank context line")
	}

	last := diff.Rows.Files[0].Rows[len(diff.Rows.Files[0].Rows)-1]
	if last.Type != protocol.DiffRowAdded || !last.NoNewline || *last.Code != "B" {
		t.Fatalf("line termination = %+v", last)
	}
	selected, err := s.GetWorkspaceDiff(t.Context(), protocol.GetDiffRequest{Path: "a.txt"})
	if err != nil || len(selected.Rows.Files) != 1 || !reflect.DeepEqual(selected.Rows.Files[0], diff.Rows.Files[0]) {
		t.Fatalf("single-file projection differs: %+v, %v", selected, err)
	}
	raw, err := s.GetWorkspaceDiff(t.Context(), protocol.GetDiffRequest{Format: protocol.DiffFormatRaw})
	if err != nil || strings.Count(*raw.Patch, "diff --git ") != 2 || !strings.Contains(*raw.Patch, `\ No newline at end of file`) {
		t.Fatalf("raw patch = %+v, %v", raw, err)
	}
	visual, err := s.GetWorkspaceDiff(t.Context(), protocol.GetDiffRequest{Format: protocol.DiffFormatVisual})
	if err != nil {
		t.Fatal(err)
	}
	if err := protocol.ValidateWireTree(visual); err != nil {
		t.Fatal(err)
	}
	if visual.Type != protocol.DiffFormatVisual || visual.Visual.Patch != *raw.Patch || len(visual.Visual.Manifest) != 2 ||
		visual.Visual.Manifest[1].Status != protocol.FileStatusUntracked || visual.Visual.Truncated ||
		visual.Visual.Budget != (protocol.DiffBudget{Files: 5000, Rows: 5000, Bytes: 64 << 20}) {
		t.Fatalf("visual read = %+v", visual.Visual)
	}
	bounded, err := s.GetWorkspaceDiff(t.Context(), protocol.GetDiffRequest{Format: protocol.DiffFormatVisual, Limit: valuePtr(1)})
	if err != nil || !bounded.Visual.Truncated || bounded.Visual.Patch != "" || len(bounded.Visual.Manifest) != 0 || bounded.Visual.Budget.Rows != 1 {
		t.Fatalf("zero-file visual budget = %+v, %v", bounded, err)
	}
	if _, err := s.GetWorkspaceDiff(t.Context(), protocol.GetDiffRequest{Format: "unknown"}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("unknown projection error = %v", err)
	}

	gitCmd("checkout", "-b", "feature")
	baseRef := filepath.Join(dir, ".git", "refs", "heads", "main")
	baseCommit, err := os.ReadFile(baseRef)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(baseRef, []byte(strings.Repeat("1", len(strings.TrimSpace(string(baseCommit))))+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, format := range []protocol.DiffFormat{protocol.DiffFormatRaw, protocol.DiffFormatRows, protocol.DiffFormatVisual} {
		_, err := s.GetWorkspaceDiff(t.Context(), protocol.GetDiffRequest{Mode: protocol.DiffModeBase, Format: format})
		if err == nil || errors.Is(err, protocol.ErrInvalidParams) {
			t.Fatalf("%s broken repository error = %v, want an operational failure", format, err)
		}
	}
}
