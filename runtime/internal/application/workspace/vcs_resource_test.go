package workspace

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

type resourceGitReader struct {
	changes   []FileChange
	files     []FileDiff
	patch     string
	changeMax int
	diffFiles int
	diffRows  int
	diffBytes int
}

func (r *resourceGitReader) Changes(_ context.Context, _ string, maxChanges int) ([]FileChange, error) {
	r.changeMax = maxChanges
	return slices.Clone(r.changes), nil
}

func (r *resourceGitReader) StructuredDiff(_ context.Context, _, _ string, _ bool, maxFiles, maxRows, maxBytes int) (StructuredDiffResult, error) {
	r.diffFiles, r.diffRows, r.diffBytes = maxFiles, maxRows, maxBytes
	files := slices.Clone(r.files)
	for index := range files {
		files[index].Rows = slices.Clone(files[index].Rows)
	}
	return StructuredDiffResult{Files: files}, nil
}

func (r *resourceGitReader) RawDiff(context.Context, string, string, bool, int) (string, error) {
	return r.patch, nil
}

func TestNewVCSRequiresCompleteDependencies(t *testing.T) {
	scope := newScope(t, "", "", testPaths{})
	for _, test := range []struct {
		name  string
		scope *Scope
		git   GitReader
	}{
		{name: "scope", git: &resourceGitReader{}},
		{name: "reader", scope: scope},
		{name: "typed nil reader", scope: scope, git: (*resourceGitReader)(nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if vcs, err := NewVCS(test.scope, test.git); err == nil || vcs != nil {
				t.Fatalf("NewVCS = (%v, %v), want incomplete construction rejected", vcs, err)
			}
		})
	}
}

func newVCS(t *testing.T, scope *Scope, git GitReader) *VCS {
	t.Helper()
	vcs, err := NewVCS(scope, git)
	if err != nil {
		t.Fatal(err)
	}
	return vcs
}

func TestVCSRejectsUnboundedCompleteChangeCatalog(t *testing.T) {
	changes := make([]FileChange, MaxWorkspaceChanges+1)
	for index := range changes {
		changes[index] = FileChange{Path: "changed", Status: FileStatusModified}
	}
	vcs := newVCS(t, newScope(t, "", "", testPaths{}), &resourceGitReader{changes: changes})

	if _, err := vcs.Changes(t.Context(), "/repo"); !errors.Is(err, ErrVCSResultTooLarge) {
		t.Fatalf("Changes error = %v, want ErrVCSResultTooLarge", err)
	}
}

func TestVCSChangesOwnsStablePathOrder(t *testing.T) {
	t.Parallel()
	reader := &resourceGitReader{changes: []FileChange{
		{Path: "zeta.go", Status: FileStatusModified},
		{Path: "alpha.go", Status: FileStatusAdded},
	}}
	vcs := newVCS(t, newScope(t, "", "", testPaths{}), reader)

	changes, err := vcs.Changes(t.Context(), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 || changes[0].Path != "alpha.go" || changes[1].Path != "zeta.go" {
		t.Fatalf("Changes = %+v, want path-ascending order", changes)
	}
	if reader.changes[0].Path != "zeta.go" {
		t.Fatal("Changes mutated adapter-owned rows")
	}
	changes[0].Path = "caller edit"
	next, err := vcs.Changes(t.Context(), "/repo")
	if err != nil || len(next) != 2 || next[0].Path != "alpha.go" {
		t.Fatalf("Changes after caller reused result = (%+v, %v)", next, err)
	}
}

func TestVCSRejectsRepeatedChangePath(t *testing.T) {
	vcs := newVCS(t, newScope(t, "", "", testPaths{}), &resourceGitReader{changes: []FileChange{
		{Path: "main.go", Status: FileStatusModified},
		{Path: "main.go", Status: FileStatusModified},
	}})

	_, err := vcs.Changes(t.Context(), "/repo")
	if err == nil || !strings.Contains(err.Error(), `VCS changes repeated path "main.go"`) {
		t.Fatalf("Changes error = %v, want repeated path failure", err)
	}
}

func TestVCSDiffAppliesDefaultBudgetAtTheFirstFileBoundary(t *testing.T) {
	rows := make([]DiffRow, MaxWorkspaceDiffRows+1)
	for index := range rows {
		rows[index] = DiffRow{Type: DiffRowAdded, Code: "line"}
	}
	vcs := newVCS(t, newScope(t, "", "", testPaths{}), &resourceGitReader{
		files: []FileDiff{{Path: "large.txt", Status: FileStatusModified, Rows: rows}},
	})

	diff, err := vcs.Diff(t.Context(), DiffInput{CWD: "/repo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Files) != 0 || !diff.Truncated {
		t.Fatalf("Diff = %d files, truncated=%v; want zero whole files and an honest cut", len(diff.Files), diff.Truncated)
	}
}

func TestVCSDiffAppliesMaterialBudgetAtTheFirstFileBoundary(t *testing.T) {
	vcs := newVCS(t, newScope(t, "", "", testPaths{}), &resourceGitReader{
		files: []FileDiff{{
			Path:   "large.txt",
			Status: FileStatusModified,
			Rows: []DiffRow{{
				Type: DiffRowAdded,
				Code: strings.Repeat("x", MaxWorkspaceDiffBytes+1),
			}},
		}},
	})

	diff, err := vcs.Diff(t.Context(), DiffInput{CWD: "/repo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Files) != 0 || !diff.Truncated {
		t.Fatalf("Diff = %d files, truncated=%v; want zero whole files and an honest material cut", len(diff.Files), diff.Truncated)
	}
}

func TestVCSDiffRejectsRepeatedRetainedPath(t *testing.T) {
	vcs := newVCS(t, newScope(t, "", "", testPaths{}), &resourceGitReader{files: []FileDiff{
		{Path: "main.go", Status: FileStatusModified},
		{Path: "main.go", Status: FileStatusModified},
	}})

	_, err := vcs.Diff(t.Context(), DiffInput{CWD: "/repo"})
	if err == nil || !strings.Contains(err.Error(), `VCS diff repeated path "main.go"`) {
		t.Fatalf("Diff error = %v, want repeated path failure", err)
	}
}

func TestVCSRejectsUnboundedRawDiffFromDirectPort(t *testing.T) {
	vcs := newVCS(t, newScope(t, "", "", testPaths{}), &resourceGitReader{
		patch: strings.Repeat("p", MaxWorkspaceDiffBytes+1),
	})

	if _, err := vcs.Diff(t.Context(), DiffInput{CWD: "/repo", Raw: true}); !errors.Is(err, ErrVCSResultTooLarge) {
		t.Fatalf("Diff error = %v, want ErrVCSResultTooLarge", err)
	}
}

func TestVCSPassesApplicationLimitsToTheGitReader(t *testing.T) {
	reader := &resourceGitReader{}
	vcs := newVCS(t, newScope(t, "", "", testPaths{}), reader)
	if _, err := vcs.Changes(t.Context(), "/repo"); err != nil {
		t.Fatal(err)
	}
	limit, err := NewDiffRowLimit(42)
	if err != nil {
		t.Fatalf("NewDiffRowLimit: %v", err)
	}
	if _, err := vcs.Diff(t.Context(), DiffInput{CWD: "/repo", RowLimit: limit}); err != nil {
		t.Fatal(err)
	}
	if reader.changeMax != MaxWorkspaceChanges || reader.diffFiles != MaxWorkspaceDiffFiles ||
		reader.diffRows != 42 || reader.diffBytes != MaxWorkspaceDiffBytes {
		t.Fatalf(
			"reader limits = changes:%d files:%d rows:%d bytes:%d",
			reader.changeMax,
			reader.diffFiles,
			reader.diffRows,
			reader.diffBytes,
		)
	}
}

func TestDiffRowLimitOwnsDefaultClampAndInvalidState(t *testing.T) {
	if rows, err := DefaultDiffRowLimit().Rows(); err != nil || rows != MaxWorkspaceDiffRows {
		t.Fatalf("default Rows = (%d, %v), want %d", rows, err, MaxWorkspaceDiffRows)
	}
	large, err := NewDiffRowLimit(MaxWorkspaceDiffRows + 1)
	if err != nil {
		t.Fatalf("NewDiffRowLimit: %v", err)
	}
	if rows, resolveErr := large.Rows(); resolveErr != nil || rows != MaxWorkspaceDiffRows {
		t.Fatalf("clamped Rows = (%d, %v), want %d", rows, resolveErr, MaxWorkspaceDiffRows)
	}
	for _, rows := range []int{0, -1} {
		if _, constructErr := NewDiffRowLimit(rows); !errors.Is(constructErr, ErrPageLimit) {
			t.Fatalf("NewDiffRowLimit(%d) = %v, want ErrPageLimit", rows, constructErr)
		}
	}
	if _, err := (DiffRowLimit{explicit: true}).Rows(); !errors.Is(err, ErrPageLimit) {
		t.Fatalf("corrupt DiffRowLimit = %v, want ErrPageLimit", err)
	}
}
