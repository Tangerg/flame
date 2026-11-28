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
	Path         string
	Status       Status
	PreviousPath string
	Added        int
	Removed      int
	Binary       bool
	Rows         []Row
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
	patch, untrackedPaths, baseline, err := completeDiff(ctx, dir, relPath, mode, budget.Bytes)
	if err != nil {
		return DiffResult{}, err
	}
	parsed, err := parseUnifiedDiff(patch, budget.Files, budget.Rows, projection == Rows)
	if err != nil {
		return DiffResult{}, err
	}
	untracked := make(map[string]struct{}, len(untrackedPaths))
	for _, path := range untrackedPaths {
		untracked[path] = struct{}{}
	}
	for index := range parsed.files {
		if _, ok := untracked[parsed.files[index].Path]; ok {
			parsed.files[index].Status = StatusUntracked
		}
	}
	result := DiffResult{Baseline: baseline, Files: parsed.files, Truncated: parsed.truncated}
	if projection == Visual {
		result.Patch = string(patch[:parsed.end])
	}
	return result, nil
}

func RawDiff(ctx context.Context, dir, relPath string, mode Mode, maxBytes int) (RawDiffResult, error) {
	patch, _, baseline, err := completeDiff(ctx, dir, relPath, mode, maxBytes)
	if err != nil {
		return RawDiffResult{}, err
	}
	return RawDiffResult{Baseline: baseline, Patch: string(patch)}, nil
}

func completeDiff(ctx context.Context, dir, relPath string, mode Mode, maxBytes int) ([]byte, []string, Baseline, error) {
	baseline, revision, err := diffBaseline(ctx, dir, mode)
	if err != nil {
		return nil, nil, Baseline{}, err
	}
	patch, untracked, err := readWorktreePatch(ctx, dir, patchComparison{
		revision: revision, path: relPath, includeUntracked: mode == Worktree,
	}, maxBytes)
	if err != nil {
		return nil, nil, Baseline{}, err
	}
	return patch, untracked, baseline, nil
}
