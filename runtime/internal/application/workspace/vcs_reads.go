package workspace

import (
	"cmp"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"

	"github.com/samber/lo"
)

const (
	maxWorkspaceChanges   = 10_000
	maxWorkspaceDiffFiles = 5_000
	maxWorkspaceDiffRows  = 5_000
	maxWorkspaceDiffBytes = 64 << 20
)

// VCS owns root-scoped Git status and diff operations.
type VCS struct {
	scope *Scope
	git   gitReader
}

func NewVCS(scope *Scope, git gitReader) (*VCS, error) {
	if scope == nil {
		return nil, errors.New("workspace: vcs scope is required")
	}
	if lo.IsNil(git) {
		return nil, errors.New("workspace: git reader is required")
	}
	return &VCS{scope: scope, git: git}, nil
}

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
	NoNewline bool
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

type DiffBaselineType string

const (
	diffBaselineHead      DiffBaselineType = "head"
	diffBaselineMergeBase DiffBaselineType = "mergeBase"
	diffBaselineEmptyTree DiffBaselineType = "emptyTree"
)

// DiffBaseline identifies the immutable Git object used by the comparison.
type DiffBaseline struct {
	Type   DiffBaselineType
	Commit string
}

func (b DiffBaseline) Validate() error {
	switch b.Type {
	case diffBaselineEmptyTree:
		if b.Commit == "" {
			return nil
		}
	case diffBaselineHead, diffBaselineMergeBase:
		if len(b.Commit) == hex.EncodedLen(sha1.Size) || len(b.Commit) == hex.EncodedLen(sha256.Size) {
			if _, err := hex.DecodeString(b.Commit); err == nil {
				return nil
			}
		}
	}
	return errors.New("workspace: invalid diff baseline")
}

type StructuredDiffResult struct {
	Baseline  DiffBaseline
	Files     []FileDiff
	Patch     string
	Truncated bool
}

type RawDiffResult struct {
	Baseline DiffBaseline
	Patch    string
}

// gitReader is the application-owned port for working-tree status and diff
// reads. Its error contract uses this package's VCS sentinels. Results transfer
// ownership to the caller, including the rows within structured diffs.
// StructuredDiff returns only whole files, at most maxFiles of them holding at
// most maxRows rows altogether, and reports Truncated when it stopped early.
type gitReader interface {
	Changes(ctx context.Context, root string, maxChanges int) ([]FileChange, error)
	StructuredDiff(ctx context.Context, root, path string, base bool, format DiffFormat, maxFiles, maxRows, maxBytes int) (StructuredDiffResult, error)
	RawDiff(ctx context.Context, root, path string, base bool, maxBytes int) (RawDiffResult, error)
}

type DiffFormat string

const (
	DiffFormatRows   DiffFormat = "rows"
	DiffFormatRaw    DiffFormat = "raw"
	DiffFormatVisual DiffFormat = "visual"
)

type DiffBudget struct {
	Files int
	Rows  int
	Bytes int
}

// DiffInput selects a working-tree or merge-base diff and its read projection.
type DiffInput struct {
	CWD      string
	Path     string
	Base     bool
	Format   DiffFormat
	RowLimit DiffRowLimit
}

// DiffRowLimit is the caller's structured-diff row budget. Zero is the named
// default; the constructor admits only a positive row count. Complete raw
// reads reject an explicit budget instead of silently ignoring it.
type DiffRowLimit struct{ rows int }

// DefaultDiffRowLimit asks the structured read to use its owned maximum.
func DefaultDiffRowLimit() DiffRowLimit { return DiffRowLimit{} }

// NewDiffRowLimit constructs an explicit positive structured-diff row budget.
func NewDiffRowLimit(rows int) (DiffRowLimit, error) {
	if rows <= 0 {
		return DiffRowLimit{}, ErrPageLimit
	}
	return DiffRowLimit{rows: rows}, nil
}

// Rows resolves this request against the Application-owned maximum.
func (l DiffRowLimit) Rows() int {
	if l.rows == 0 {
		return maxWorkspaceDiffRows
	}
	return min(l.rows, maxWorkspaceDiffRows)
}

// Diff is a structured or raw workspace diff.
type Diff struct {
	Format    DiffFormat
	Baseline  DiffBaseline
	Patch     string
	Files     []FileDiff
	Truncated bool
	Budget    DiffBudget
}

// Changes reads the root's VCS status.
func (v *VCS) Changes(ctx context.Context, cwd string) ([]FileChange, error) {
	root, err := v.scope.ResolveRoot(cwd)
	if err != nil {
		return nil, err
	}
	changes, err := v.git.Changes(ctx, root, maxWorkspaceChanges)
	if err != nil {
		return nil, err
	}
	if len(changes) > maxWorkspaceChanges {
		return nil, fmt.Errorf("%w: more than %d workspace changes", ErrVCSResultTooLarge, maxWorkspaceChanges)
	}
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

// Diff reads a workspace VCS diff, keeping path confinement in the application
// use case. This use case chooses the file, row and byte limits; the reader
// owns cutting at a whole-file boundary within them, because only it can stop
// before materializing what does not fit.
func (v *VCS) Diff(ctx context.Context, input DiffInput) (Diff, error) {
	format, err := input.format()
	if err != nil {
		return Diff{}, err
	}
	root, err := v.scope.ResolveRoot(input.CWD)
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
	if format == DiffFormatRaw {
		return v.rawDiff(ctx, root, path, input.Base)
	}
	return v.structuredDiff(ctx, root, path, input, format)
}

// format is the requested diff format, rows by default; a raw patch takes no
// row budget.
func (d DiffInput) format() (DiffFormat, error) {
	format := d.Format
	if format == "" {
		format = DiffFormatRows
	}
	switch format {
	case DiffFormatRaw:
		if d.RowLimit.rows != 0 {
			return "", errors.New("workspace: raw diff does not accept a row budget")
		}
	case DiffFormatRows, DiffFormatVisual:
	default:
		return "", fmt.Errorf("workspace: unsupported diff format %q", format)
	}
	return format, nil
}

func (v *VCS) rawDiff(ctx context.Context, root, path string, base bool) (Diff, error) {
	result, err := v.git.RawDiff(ctx, root, path, base, maxWorkspaceDiffBytes)
	if err != nil {
		return Diff{}, err
	}
	if len(result.Patch) > maxWorkspaceDiffBytes {
		return Diff{}, fmt.Errorf("%w: raw diff exceeds %d bytes", ErrVCSResultTooLarge, maxWorkspaceDiffBytes)
	}
	if err := result.Baseline.Validate(); err != nil {
		return Diff{}, err
	}
	return Diff{Format: DiffFormatRaw, Baseline: result.Baseline, Patch: result.Patch}, nil
}

func (v *VCS) structuredDiff(ctx context.Context, root, path string, input DiffInput, format DiffFormat) (Diff, error) {
	rowLimit := input.RowLimit.Rows()
	result, err := v.git.StructuredDiff(ctx, root, path, input.Base, format, maxWorkspaceDiffFiles, rowLimit, maxWorkspaceDiffBytes)
	if err != nil {
		return Diff{}, err
	}
	if err := result.Baseline.Validate(); err != nil {
		return Diff{}, err
	}
	if len(result.Patch) > maxWorkspaceDiffBytes {
		return Diff{}, fmt.Errorf("%w: visual diff exceeds %d bytes", ErrVCSResultTooLarge, maxWorkspaceDiffBytes)
	}
	paths := make(map[string]struct{}, len(result.Files))
	for _, file := range result.Files {
		if _, duplicate := paths[file.Path]; duplicate {
			return Diff{}, fmt.Errorf("workspace: VCS diff repeated path %q", file.Path)
		}
		paths[file.Path] = struct{}{}
	}
	return Diff{
		Format: format, Baseline: result.Baseline, Files: result.Files,
		Patch: result.Patch, Truncated: result.Truncated,
		Budget: DiffBudget{Files: maxWorkspaceDiffFiles, Rows: rowLimit, Bytes: maxWorkspaceDiffBytes},
	}, nil
}
