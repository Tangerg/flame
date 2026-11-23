package git

import (
	"context"
	"errors"
	"fmt"
	"os"
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
	patch, untracked, baseline, err := completeDiff(ctx, dir, relPath, mode, budget.Bytes)
	if err != nil {
		return DiffResult{}, err
	}
	parsed, err := parseUnifiedDiff(patch, budget.Files, budget.Rows, projection == Rows)
	if err != nil {
		return DiffResult{}, err
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

func completeDiff(ctx context.Context, dir, relPath string, mode Mode, maxBytes int) ([]byte, map[string]struct{}, Baseline, error) {
	if maxBytes <= 0 {
		return nil, nil, Baseline{}, fmt.Errorf("%w: diff requires a positive byte limit", ErrResultTooLarge)
	}
	patch, untrackedPaths, baseline, err := diffSources(ctx, dir, relPath, mode)
	if err != nil {
		return nil, nil, Baseline{}, err
	}
	if len(patch) > maxBytes {
		return nil, nil, Baseline{}, fmt.Errorf("%w: aggregate diff exceeds %d bytes", ErrResultTooLarge, maxBytes)
	}
	untracked := make(map[string]struct{}, len(untrackedPaths))
	for _, path := range untrackedPaths {
		untracked[path] = struct{}{}
		// no-index diff of /dev/null vs the file; Git uses exit code 1 to report
		// the expected fact that the two paths differ.
		out, err := runAllowingExitCode(
			ctx,
			dir,
			1,
			"diff", "--no-index", "--no-ext-diff", "--no-textconv", "--no-color", "--", os.DevNull, path,
		)
		if err != nil {
			return nil, nil, Baseline{}, err
		}
		patch, err = appendDiffPatch(patch, out, maxBytes)
		if err != nil {
			return nil, nil, Baseline{}, err
		}
	}
	return patch, untracked, baseline, nil
}

func appendDiffPatch(existing, addition []byte, maxBytes int) ([]byte, error) {
	if len(addition) == 0 {
		return existing, nil
	}
	separator := 0
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		separator = 1
	}
	if len(existing) > maxBytes-separator || len(addition) > maxBytes-len(existing)-separator {
		return nil, fmt.Errorf("%w: aggregate diff exceeds %d bytes", ErrResultTooLarge, maxBytes)
	}
	if separator != 0 {
		existing = append(existing, '\n')
	}
	return append(existing, addition...), nil
}
