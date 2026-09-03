package workspace

import (
	"cmp"
	"context"
	"fmt"
	"slices"
)

const (
	MaxWorkspaceChanges   = 10_000
	MaxWorkspaceDiffFiles = 5_000
	MaxWorkspaceDiffRows  = 5_000
	MaxWorkspaceDiffBytes = 64 << 20
)

// VCS owns root-scoped Git status and diff operations.
type VCS struct {
	scope *Scope
	git   GitReader
}

func NewVCS(scope *Scope, git GitReader) *VCS { return &VCS{scope: scope, git: git} }

// FileStatus is the application vocabulary for a working-tree change.
type FileStatus string

const (
	FileStatusAdded     FileStatus = "added"
	FileStatusModified  FileStatus = "modified"
	FileStatusDeleted   FileStatus = "deleted"
	FileStatusRenamed   FileStatus = "renamed"
	FileStatusUntracked FileStatus = "untracked"
)

// FileChange is one working-tree change.
type FileChange struct {
	Path         string
	Status       FileStatus
	PreviousPath string
	Binary       bool
	Added        int
	Removed      int
}

// DiffRowType is the application vocabulary for a parsed unified-diff row.
type DiffRowType string

const (
	DiffRowHunk    DiffRowType = "hunk"
	DiffRowContext DiffRowType = "context"
	DiffRowAdded   DiffRowType = "added"
	DiffRowDeleted DiffRowType = "deleted"
)

// DiffRow is one structured diff row.
type DiffRow struct {
	Type      DiffRowType
	Text      string
	LeftLine  int
	RightLine int
	Code      string
}

// FileDiff is one file's structured diff.
type FileDiff struct {
	Path         string
	Status       FileStatus
	PreviousPath string
	Binary       bool
	Added        int
	Removed      int
	Rows         []DiffRow
}

// StructuredDiffResult carries an honestly bounded whole-file projection.
type StructuredDiffResult struct {
	Files     []FileDiff
	Truncated bool
}

// GitReader is the application-owned port for working-tree status and diff
// reads. Its error contract uses this package's VCS sentinels.
type GitReader interface {
	Changes(ctx context.Context, root string, maxChanges int) ([]FileChange, error)
	StructuredDiff(ctx context.Context, root, path string, base bool, maxFiles, maxRows, maxBytes int) (StructuredDiffResult, error)
	RawDiff(ctx context.Context, root, path string, base bool, maxBytes int) (string, error)
}

// DiffInput selects a working-tree or merge-base diff, optionally as raw text.
type DiffInput struct {
	CWD      string
	Path     string
	Base     bool
	Raw      bool
	RowLimit DiffRowLimit
}

// DiffRowLimit is the caller's structured-diff row budget. Its zero value is
// the named default intent; an explicit value can only be constructed as a
// positive row count. Raw diffs do not consume this value at all.
type DiffRowLimit struct {
	explicit bool
	rows     int
}

// DefaultDiffRowLimit asks the structured read to use its owned maximum.
func DefaultDiffRowLimit() DiffRowLimit { return DiffRowLimit{} }

// NewDiffRowLimit constructs an explicit positive structured-diff row budget.
func NewDiffRowLimit(rows int) (DiffRowLimit, error) {
	if rows <= 0 {
		return DiffRowLimit{}, ErrPageLimit
	}
	return DiffRowLimit{explicit: true, rows: rows}, nil
}

// Rows resolves this request against the Application-owned maximum.
func (l DiffRowLimit) Rows() (int, error) {
	if !l.explicit {
		if l.rows != 0 {
			return 0, ErrPageLimit
		}
		return MaxWorkspaceDiffRows, nil
	}
	if l.rows <= 0 {
		return 0, ErrPageLimit
	}
	return min(l.rows, MaxWorkspaceDiffRows), nil
}

// Diff is a structured or raw workspace diff.
type Diff struct {
	Patch     string
	Files     []FileDiff
	Truncated bool
}

// Changes reads the root's VCS status.
func (v *VCS) Changes(ctx context.Context, cwd string) ([]FileChange, error) {
	root, err := v.scope.root(cwd)
	if err != nil {
		return nil, err
	}
	if v.git == nil {
		return nil, ErrVCSUnavailable
	}
	changes, err := v.git.Changes(ctx, root, MaxWorkspaceChanges)
	if err != nil {
		return nil, err
	}
	if len(changes) > MaxWorkspaceChanges {
		return nil, fmt.Errorf("%w: more than %d workspace changes", ErrVCSResultTooLarge, MaxWorkspaceChanges)
	}
	changes = slices.Clone(changes)
	paths := make(map[string]struct{}, len(changes))
	for _, change := range changes {
		if _, duplicate := paths[change.Path]; duplicate {
			return nil, fmt.Errorf("workspace: VCS changes repeated path %q", change.Path)
		}
		paths[change.Path] = struct{}{}
	}
	slices.SortFunc(changes, func(a, b FileChange) int { return cmp.Compare(a.Path, b.Path) })
	return changes, nil
}

// Diff reads a workspace VCS diff, keeping path confinement and file-boundary
// truncation in the application use case.
func (v *VCS) Diff(ctx context.Context, input DiffInput) (Diff, error) {
	root, err := v.scope.root(input.CWD)
	if err != nil {
		return Diff{}, err
	}
	path := ""
	if input.Path != "" {
		path, err = v.scope.paths.ResolveInRoot(root, input.Path)
		if err != nil {
			return Diff{}, err
		}
	}
	if v.git == nil {
		return Diff{}, ErrVCSUnavailable
	}
	if input.Raw {
		patch, rawDiffErr := v.git.RawDiff(ctx, root, path, input.Base, MaxWorkspaceDiffBytes)
		if rawDiffErr != nil {
			return Diff{}, rawDiffErr
		}
		if len(patch) > MaxWorkspaceDiffBytes {
			return Diff{}, fmt.Errorf("%w: raw diff exceeds %d bytes", ErrVCSResultTooLarge, MaxWorkspaceDiffBytes)
		}
		return Diff{Patch: patch}, nil
	}
	rowLimit, err := input.RowLimit.Rows()
	if err != nil {
		return Diff{}, err
	}
	result, err := v.git.StructuredDiff(
		ctx,
		root,
		path,
		input.Base,
		MaxWorkspaceDiffFiles,
		rowLimit,
		MaxWorkspaceDiffBytes,
	)
	if err != nil {
		return Diff{}, err
	}
	files, truncated := limitDiffFiles(result.Files, MaxWorkspaceDiffFiles, rowLimit, MaxWorkspaceDiffBytes)
	paths := make(map[string]struct{}, len(files))
	for _, file := range files {
		if _, duplicate := paths[file.Path]; duplicate {
			return Diff{}, fmt.Errorf("workspace: VCS diff repeated path %q", file.Path)
		}
		paths[file.Path] = struct{}{}
	}
	return Diff{Files: files, Truncated: result.Truncated || truncated}, nil
}

func limitDiffFiles(files []FileDiff, maxFiles, maxRows, maxBytes int) ([]FileDiff, bool) {
	rows, material := 0, 0
	for index, file := range files {
		if index >= maxFiles || len(file.Rows) > maxRows-rows {
			return files[:index], true
		}
		fileMaterial, fits := diffFileMaterialBytes(file, maxBytes-material)
		if !fits {
			return files[:index], true
		}
		rows += len(file.Rows)
		material += fileMaterial
	}
	return files, false
}

func diffFileMaterialBytes(file FileDiff, remaining int) (int, bool) {
	if remaining < 0 {
		return 0, false
	}
	used := 0
	add := func(size int) bool {
		if size > remaining-used {
			return false
		}
		used += size
		return true
	}
	if !add(len(file.Path)) || !add(len(file.PreviousPath)) {
		return 0, false
	}
	for _, row := range file.Rows {
		if !add(len(row.Text)) || !add(len(row.Code)) {
			return 0, false
		}
	}
	return used, true
}
