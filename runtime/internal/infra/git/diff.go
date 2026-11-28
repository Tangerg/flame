package git

import (
	"context"
	"errors"
	"fmt"
)

var ErrNoBase = errors.New("git: cannot resolve base branch")

type Mode string

const (
	Worktree Mode = "worktree"
	Base     Mode = "base"
)

type RowType string

const (
	RowHunk    RowType = "hunk"
	RowContext RowType = "context"
	RowAdded   RowType = "added"
	RowDeleted RowType = "deleted"
)

type Row struct {
	Type      RowType
	Text      string
	LeftLine  int
	RightLine int
	Code      string
	NoNewline bool
}

type DiffFile struct {
	FileChange
	Rows []Row
}

type BaselineType string

const (
	BaselineHead      BaselineType = "head"
	BaselineMergeBase BaselineType = "mergeBase"
	BaselineEmptyTree BaselineType = "emptyTree"
)

type Baseline struct {
	Type   BaselineType
	Commit string
}

type DiffResult struct {
	Baseline  Baseline
	Files     []DiffFile
	Patch     string
	Truncated bool
}

type Projection string

const (
	Rows   Projection = "rows"
	Visual Projection = "visual"
)

type RawDiffResult struct {
	Baseline Baseline
	Patch    string
}

type DiffBudget struct {
	Files int
	Rows  int
	Bytes int
}

func Diff(ctx context.Context, dir, relPath string, mode Mode, projection Projection, budget DiffBudget) (DiffResult, error) {
	if projection != Rows && projection != Visual {
		return DiffResult{}, fmt.Errorf("git: unsupported diff projection %q", projection)
	}
	if budget.Files <= 0 || budget.Rows <= 0 {
		return DiffResult{}, fmt.Errorf("%w: diff projection requires positive limits", ErrResultTooLarge)
	}
	comparison, baseline, err := resolveComparison(ctx, dir, relPath, mode)
	if err != nil {
		return DiffResult{}, err
	}
	output, err := comparison.capture(ctx, dir, budget.Bytes, append(changeSummaryFlags(), "--patch")...)
	if err != nil {
		return DiffResult{}, err
	}
	summary, err := output.summarize(budget.Files)
	if err != nil {
		return DiffResult{}, err
	}
	parsed, err := parseUnifiedDiff(summary, budget.Rows, projection == Rows)
	if err != nil {
		return DiffResult{}, err
	}
	result := DiffResult{Baseline: baseline, Files: parsed.files, Truncated: parsed.truncated}
	if projection == Visual {
		result.Patch = string(summary.patch[:parsed.end])
	}
	return result, nil
}

func RawDiff(ctx context.Context, dir, relPath string, mode Mode, maxBytes int) (RawDiffResult, error) {
	comparison, baseline, err := resolveComparison(ctx, dir, relPath, mode)
	if err != nil {
		return RawDiffResult{}, err
	}
	output, err := comparison.capture(ctx, dir, maxBytes)
	if err != nil {
		return RawDiffResult{}, err
	}
	return RawDiffResult{Baseline: baseline, Patch: string(output.data)}, nil
}

func resolveComparison(ctx context.Context, dir, relPath string, mode Mode) (worktreeComparison, Baseline, error) {
	baseline, revision, err := diffBaseline(ctx, dir, mode)
	if err != nil {
		return worktreeComparison{}, Baseline{}, err
	}
	return worktreeComparison{
		revision: revision, path: relPath, includeUntracked: mode == Worktree,
	}, baseline, nil
}
