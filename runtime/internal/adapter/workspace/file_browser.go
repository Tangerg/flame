package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/qimu/cancellation"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/scope/tools/textread"
)

// FileBrowser adapts local filesystem browsing and content search to the workspace
// application ports.
type FileBrowser struct{}

func (FileBrowser) List(ctx context.Context, root string, options workspaceapp.FileListOptions) ([]workspaceapp.FileEntry, error) {
	entries, err := listFiles(ctx, root, options)
	if err != nil {
		switch {
		case errors.Is(err, errListingTooLarge):
			return nil, errors.Join(workspaceapp.ErrFileListTooLarge, err)
		case errors.Is(err, errInvalidGlob):
			return nil, errors.Join(workspaceapp.ErrInvalidFileGlob, err)
		case errors.Is(err, errInvalidListPath):
			return nil, errors.Join(workspaceapp.ErrInvalidFileListPath, err)
		default:
			return nil, err
		}
	}
	return entries, nil
}

func (FileBrowser) Read(ctx context.Context, root string, input workspaceapp.FileReadPlan) (_ workspaceapp.FileReadResult, err error) {
	if cause := cancellation.Err(ctx); cause != nil {
		return workspaceapp.FileReadResult{}, cause
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
	file, opened, err := filesystem.OpenFileAt(
		rootHandle,
		filepath.FromSlash(relative),
		workspaceapp.MaxFileReadSourceBytes,
	)
	if err != nil {
		switch {
		case errors.Is(err, filesystem.ErrNotRegular):
			return workspaceapp.FileReadResult{}, fmt.Errorf("%w: %s: %w", workspaceapp.ErrUnsupportedFile, input.Path, err)
		case errors.Is(err, filesystem.ErrFileTooLarge):
			return workspaceapp.FileReadResult{}, fmt.Errorf("%w: %s: %w", workspaceapp.ErrFileReadTooLarge, input.Path, err)
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
	source := &fileScanInput{reader: file}
	result, err := textread.Scan(ctx, source, textread.Options{
		InputBytes: workspaceapp.MaxFileReadSourceBytes, LineBytes: workspaceapp.MaxFileReadLineBytes,
		OutputBytes: input.MaxBytes, StartLine: start, MaxLines: lines, PartialLine: true,
	})
	err = errors.Join(err, source.readErr, cancellation.Err(ctx))
	if err != nil {
		switch {
		case errors.Is(err, textread.ErrInputTooLarge):
			return workspaceapp.FileReadResult{}, fmt.Errorf("%w: %s grew while reading: %w", workspaceapp.ErrFileReadTooLarge, input.Path, err)
		case errors.Is(err, textread.ErrLineTooLarge):
			return workspaceapp.FileReadResult{}, fmt.Errorf(
				"%w: %s line %d exceeds the %d MiB limit: %w",
				workspaceapp.ErrFileReadTooLarge, input.Path, textread.LineNumber(err),
				workspaceapp.MaxFileReadLineBytes>>20, err,
			)
		case errors.Is(err, textread.ErrInvalidText):
			return workspaceapp.FileReadResult{}, fmt.Errorf("%w: %s: %w", workspaceapp.ErrUnsupportedFile, input.Path, err)
		default:
			return workspaceapp.FileReadResult{}, fmt.Errorf("workspace: scan %s: %w", input.Path, err)
		}
	}
	if err := filesystem.VerifyAtVersion(file, opened, rootHandle, filepath.FromSlash(relative)); err != nil {
		return workspaceapp.FileReadResult{}, fmt.Errorf("workspace: verify %s after reading: %w", input.Path, err)
	}
	if input.StartLine > result.TotalLines {
		return workspaceapp.FileReadResult{}, workspaceapp.ErrInvalidFileRange
	}
	return workspaceapp.FileReadResult{
		Content: result.Content, TotalLines: result.TotalLines, StartLine: result.StartLine,
		EndLine: result.EndLine, Truncated: result.Truncated,
	}, nil
}
