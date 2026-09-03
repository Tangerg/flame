package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
	"github.com/Tangerg/scope/tools/textread"
)

// FileBrowser adapts local filesystem browsing and content search to the workspace
// application ports.
type FileBrowser struct{}

func (FileBrowser) List(ctx context.Context, root string, options workspaceapp.FileListOptions) ([]workspaceapp.FileEntry, error) {
	entries, err := ListFiles(ctx, root, options)
	if err != nil {
		switch {
		case errors.Is(err, ErrListingTooLarge):
			return nil, workspaceapp.ErrFileListTooLarge
		case errors.Is(err, ErrInvalidGlob):
			return nil, workspaceapp.ErrInvalidFileGlob
		case errors.Is(err, errInvalidListPath):
			return nil, workspaceapp.ErrInvalidFileListPath
		default:
			return nil, err
		}
	}
	return entries, nil
}

func (FileBrowser) Read(ctx context.Context, root string, input workspaceapp.FileReadPlan) (_ workspaceapp.FileReadResult, err error) {
	if cause := context.Cause(ctx); cause != nil {
		return workspaceapp.FileReadResult{}, cause
	}
	budget, err := workspaceReadBudget(input.MaxBytes)
	if err != nil {
		return workspaceapp.FileReadResult{}, err
	}
	path := input.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return workspaceapp.FileReadResult{}, fmt.Errorf("workspace: resolve read root: %w", err)
	}
	rootHandle, err := os.OpenRoot(canonicalRoot)
	if err != nil {
		return workspaceapp.FileReadResult{}, fmt.Errorf("workspace: open read root: %w", err)
	}
	defer func() {
		err = errors.Join(err, rootHandle.Close())
	}()
	relative, err := rootRelativeFilePath(root, canonicalRoot, path)
	if err != nil {
		return workspaceapp.FileReadResult{}, err
	}
	file, opened, err := fileinput.OpenAt(
		rootHandle,
		filepath.FromSlash(relative),
		workspaceapp.MaxFileReadSourceBytes,
	)
	if err != nil {
		switch {
		case errors.Is(err, fileinput.ErrNotRegular):
			return workspaceapp.FileReadResult{}, fmt.Errorf("%w: %s", workspaceapp.ErrUnsupportedFile, input.Path)
		case errors.Is(err, fileinput.ErrTooLarge):
			return workspaceapp.FileReadResult{}, fmt.Errorf("%w: %s", workspaceapp.ErrFileReadTooLarge, input.Path)
		default:
			return workspaceapp.FileReadResult{}, err
		}
	}
	defer func() {
		err = errors.Join(err, file.Close())
	}()

	start, lines := 0, 0
	if input.StartLine > 0 {
		start = input.StartLine - 1
		if input.EndLine >= input.StartLine {
			lines = input.EndLine - input.StartLine + 1
		}
	}
	result, err := textread.Scan(ctx, file, textread.Options{
		InputBytes: workspaceapp.MaxFileReadSourceBytes, LineBytes: workspaceapp.MaxFileReadLineBytes,
		OutputBytes: budget, StartLine: start, MaxLines: lines, PartialLine: true,
	})
	if err != nil {
		switch {
		case errors.Is(err, textread.ErrInputTooLarge):
			return workspaceapp.FileReadResult{}, fmt.Errorf("%w: %s grew while reading", workspaceapp.ErrFileReadTooLarge, input.Path)
		case errors.Is(err, textread.ErrLineTooLarge):
			return workspaceapp.FileReadResult{}, fmt.Errorf(
				"%w: %s line %d exceeds the 8 MiB limit",
				workspaceapp.ErrFileReadTooLarge, input.Path, textread.LineNumber(err),
			)
		case errors.Is(err, textread.ErrInvalidText):
			return workspaceapp.FileReadResult{}, fmt.Errorf("%w: %s", workspaceapp.ErrUnsupportedFile, input.Path)
		default:
			return workspaceapp.FileReadResult{}, fmt.Errorf("workspace: scan %s: %w", input.Path, err)
		}
	}
	if err := fileinput.VerifyAtVersion(file, opened, rootHandle, filepath.FromSlash(relative)); err != nil {
		if errors.Is(err, fileinput.ErrChanged) {
			return workspaceapp.FileReadResult{}, fmt.Errorf("workspace: %s changed while it was being read", input.Path)
		}
		return workspaceapp.FileReadResult{}, fmt.Errorf("workspace: verify %s after reading: %w", input.Path, err)
	}
	if input.StartLine > result.TotalLines {
		return workspaceapp.FileReadResult{}, workspaceapp.ErrInvalidFileRange
	}
	return workspaceapp.FileReadResult{
		Content: result.Content, TotalLines: result.TotalLines, StartLine: result.StartLine,
		EndLine: result.EndLine, Truncated: result.Truncated, OutputTruncated: result.OutputTruncated,
	}, nil
}

func workspaceReadBudget(requested int) (int, error) {
	switch {
	case requested < 0:
		return 0, workspaceapp.ErrInvalidFileRange
	case requested == 0:
		return workspaceapp.DefaultFileReadBytes, nil
	default:
		return min(requested, workspaceapp.MaxFileReadBytes), nil
	}
}
